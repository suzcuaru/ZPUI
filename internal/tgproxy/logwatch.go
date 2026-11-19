package tgproxy

import (
	"bytes"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"zpui/internal/logger"
	"zpui/internal/power"
)

// Маркеры строк лога апстрима, по которым восстанавливаются счётчики.
// Формат: "2026-10-31 08:00:00  LEVEL  [ip:port] message" и минутная
// "stats: k=v ..." (proxy/stats.py).
var (
	sessionOpenMarkers = []string{
		"-> WS pool hit",
		"-> trying CF worker",
		"-> trying CF proxy",
		"TCP fallback to",
	}
	sessionCloseMarkers = []string{
		"WS session closed",
		"fallback closed",
		"no fallback available",
	}
)

// logWatcher — инкрементальный хвост лога моста: поддерживает последние
// счётчики stats и активные сессии по IP. Файл открывается на каждый тик
// (без долгого хендла), иначе ротация RotatingFileHandler на Windows ломается.
type logWatcher struct {
	path string
	log  *logger.Logger

	mu       sync.Mutex
	pos      int64
	opens    int64
	counters map[string]interface{}
	perConn  map[string]bool // label "ip:port" -> сессия открыта
	perIP    map[string]int
	stopCh   chan struct{}
	doneCh   chan struct{}
	stopOnce sync.Once
}

func startLogWatcher(path string, log *logger.Logger) *logWatcher {
	w := &logWatcher{
		path:     path,
		log:      log,
		counters: map[string]interface{}{},
		perConn:  map[string]bool{},
		perIP:    map[string]int{},
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
	go w.loop()
	return w
}

func (w *logWatcher) stop() {
	// stop() зовут и из Manager.Stop, и из supervise при выходе процесса —
	// при штатной остановке моста срабатывают оба, повторный close паниковал
	// и ронял всё приложение.
	w.stopOnce.Do(func() { close(w.stopCh) })
	<-w.doneCh
}

func (w *logWatcher) loop() {
	defer close(w.doneCh)
	for {
		select {
		case <-w.stopCh:
			return
		case <-time.After(power.Stretch(2 * time.Second)):
		}
		if power.MonitoringPaused() {
			continue
		}
		w.pump()
	}
}

func (w *logWatcher) pump() {
	f, err := os.Open(w.path)
	if err != nil {
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return
	}

	w.mu.Lock()
	pos := w.pos
	w.mu.Unlock()
	if pos > st.Size() {
		pos = 0 // ротация/усечение — читаем заново
	}
	if pos >= st.Size() {
		return
	}
	if _, err := f.Seek(pos, io.SeekStart); err != nil {
		return
	}

	buf := make([]byte, 128*1024)
	n, _ := io.ReadFull(f, buf)
	if n <= 0 {
		return
	}
	chunk := buf[:n]
	cut := bytes.LastIndexByte(chunk, '\n')
	if cut < 0 {
		return // целой строки пока нет
	}
	w.mu.Lock()
	for _, line := range strings.Split(string(chunk[:cut]), "\n") {
		w.handleLine(strings.TrimRight(line, "\r"))
	}
	w.pos = pos + int64(cut) + 1
	w.mu.Unlock()
}

func (w *logWatcher) countersSnapshot() map[string]interface{} {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make(map[string]interface{}, len(w.counters))
	for k, v := range w.counters {
		out[k] = v
	}
	return out
}

func (w *logWatcher) clientsByIP() map[string]int {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make(map[string]int, len(w.perIP))
	for ip, n := range w.perIP {
		if n > 0 {
			out[ip] = n
		}
	}
	return out
}

// activeCount — живое число активных сессий (моста в целом, без разбора по IP).
func (w *logWatcher) activeCount() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return int64(len(w.perConn))
}

// handleLine разбирает одну строку лога; вызывается под w.mu.
func (w *logWatcher) handleLine(line string) {
	if idx := strings.Index(line, "stats: "); idx >= 0 {
		w.applyStats(line[idx+len("stats: "):])
		return
	}
	opened, closed := false, false
	for _, m := range sessionOpenMarkers {
		if strings.Contains(line, m) {
			opened = true
			break
		}
	}
	for _, m := range sessionCloseMarkers {
		if strings.Contains(line, m) {
			closed = true
			break
		}
	}
	if !opened && !closed {
		return
	}
	label, ok := sessionLabel(line)
	if !ok {
		return
	}
	ip := labelToIP(label)
	if ip == "" {
		return
	}
	if closed {
		if w.perConn[label] {
			delete(w.perConn, label)
			w.perIP[ip]--
		}
		// Байты закрытой WS-сессии: "^1.2MB (10 pkts) v3.4MB (20 pkts)".
		// Суммируем к счётчикам; минутная stats-строка потом выставит точные
		// абсолюты (включая fallback-каналы, которые тут не видны).
		if up, down, ok := parseSessionBytes(line); ok {
			w.counters["bytes_out"] = toI(w.counters["bytes_out"]) + up
			w.counters["bytes_in"] = toI(w.counters["bytes_in"]) + down
		}
		return
	}
	if !w.perConn[label] {
		w.perConn[label] = true
		w.perIP[ip]++
		// total и active обновляем сразу, не дожидаясь минутной stats-строки.
		w.opens++
		if w.opens > toI(w.counters["total"]) {
			w.counters["total"] = w.opens
		}
	}
}

// parseSessionBytes вытаскивает per-session трафик из строки закрытия
// WS-сессии апстрима.
func parseSessionBytes(line string) (up, down int64, ok bool) {
	caret := strings.Index(line, "^")
	vi := strings.Index(line, " v")
	if caret < 0 || vi < caret {
		return 0, 0, false
	}
	up, okUp := parseHumanBytes(strings.TrimSpace(line[caret+1 : vi]))
	rest := line[vi+2:]
	end := strings.Index(rest, " (")
	if end < 0 {
		return 0, 0, false
	}
	down, okDown := parseHumanBytes(strings.TrimSpace(rest[:end]))
	if !okUp || !okDown {
		return 0, 0, false
	}
	return up, down, true
}

func toI(v interface{}) int64 {
	if n, ok := v.(int64); ok {
		return n
	}
	return 0
}

// sessionLabel достаёт "[...]"-префикс с ip:port.
func sessionLabel(line string) (string, bool) {
	start := strings.Index(line, "[")
	if start < 0 {
		return "", false
	}
	end := strings.Index(line[start:], "]")
	if end < 0 {
		return "", false
	}
	label := line[start+1 : start+end]
	if !strings.Contains(label, ":") {
		return "", false
	}
	return label, true
}

func labelToIP(label string) string {
	idx := strings.LastIndex(label, ":")
	if idx <= 0 {
		return ""
	}
	ip := label[:idx]
	ip = strings.Trim(ip, "[]")
	return ip
}

// applyStats парсит "k=v k=v ..." строки stats апстрима в ключи,
// которые ждёт фронтенд (bytes_in/bytes_out/ws_ok/...).
func (w *logWatcher) applyStats(body string) {
	for _, tok := range strings.Fields(body) {
		eq := strings.Index(tok, "=")
		if eq <= 0 {
			continue
		}
		key, val := tok[:eq], tok[eq+1:]
		switch key {
		case "total":
			if n, err := strconv.ParseInt(val, 10, 64); err == nil {
				if n > w.opens {
					w.opens = n
				}
				w.counters["total"] = w.opens
			}
		case "active":
			// active считаем живьём из трекинга сессий (см. handleLine).
		case "ws", "err", "tcp_fb", "cf", "front",
			"masked", "bad", "h2", "h2_req", "h2_err", "h2_tcp":
			if n, err := strconv.ParseInt(val, 10, 64); err == nil {
				w.setCounter(key, n)
			}
		case "up", "down":
			if b, ok := parseHumanBytes(val); ok {
				if key == "up" {
					w.counters["bytes_out"] = b
				} else {
					w.counters["bytes_in"] = b
				}
			}
		case "pool", "cf_pool":
			sl := strings.SplitN(val, "/", 2)
			if len(sl) != 2 {
				continue
			}
			hits, err1 := strconv.ParseInt(sl[0], 10, 64)
			total, err2 := strconv.ParseInt(sl[1], 10, 64)
			if err1 != nil || err2 != nil {
				continue
			}
			prefix := "pool"
			if key == "cf_pool" {
				prefix = "cf_pool"
			}
			w.counters[prefix+"_hits"] = hits
			w.counters[prefix+"_misses"] = total - hits
		}
	}
}

func (w *logWatcher) setCounter(key string, n int64) {
	switch key {
	case "ws":
		w.counters["ws_ok"] = n
	case "err":
		w.counters["ws_errors"] = n
	case "tcp_fb":
		w.counters["tcp_fallback"] = n
	case "cf":
		w.counters["cfproxy"] = n
	case "h2_req":
		w.counters["h2_try"] = n
	case "h2_err":
		w.counters["h2_errors"] = n
	default:
		w.counters[key] = n
	}
}

// parseHumanBytes разбирает формат human_bytes апстрима: "123.0B", "1.5KB",
// "2.0MB", "1.0GB", "1.0TB".
func parseHumanBytes(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "TB"):
		mult, s = 1<<40, s[:len(s)-2]
	case strings.HasSuffix(s, "GB"):
		mult, s = 1<<30, s[:len(s)-2]
	case strings.HasSuffix(s, "MB"):
		mult, s = 1<<20, s[:len(s)-2]
	case strings.HasSuffix(s, "KB"):
		mult, s = 1<<10, s[:len(s)-2]
	case strings.HasSuffix(s, "B"):
		mult, s = 1, s[:len(s)-1]
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, false
	}
	return int64(f * float64(mult)), true
}
