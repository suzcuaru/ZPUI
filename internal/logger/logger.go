package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"zpui/internal/logdb"
)

const (
	ringMax          = 5000
	errRetentionDays = 30
)

// bootLineRe разбирает строку boot.log вида
// "[ts] [level] [category] message" на level/category/message.
var bootLineRe = regexp.MustCompile(`^\[[^\]]+\] \[([^\]]+)\] \[([^\]]+)\] (.*)$`)

type Logger struct {
	mu              sync.Mutex
	baseDir         string
	ring            []ringEntry
	lastSnap        time.Time
	debugCategories map[string]bool
	stopCh          chan struct{}

	onError func(code, msg string)

	bootFile *os.File
	bootPath string
	bootMu   sync.Mutex

	console bool
}

type ringEntry struct {
	t        time.Time
	level    string
	category string
	code     string
	msg      string
}

type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func New(baseDir string, logDbDir string) (*Logger, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("create logs dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "errors"), 0755); err != nil {
		return nil, fmt.Errorf("create errors dir: %w", err)
	}

	if err := logdb.Init(logDbDir); err != nil {
		return nil, fmt.Errorf("init logdb: %w", err)
	}

	l := &Logger{
		baseDir:         baseDir,
		debugCategories: make(map[string]bool),
		stopCh:          make(chan struct{}),
		console:         true,
	}

	l.initBootLog()

	go l.cleanupLoop()

	return l, nil
}

func (l *Logger) initBootLog() {
	l.bootPath = filepath.Join(l.baseDir, "boot.log")
	// O_TRUNC: каждый запуск начинается с чистого файла. Раньше было
	// O_APPEND — boot.log рос бесконечно, и при закрытии все строки
	// повторно вставлялись в БД как категория «system» с дублирующим
	// префиксом в сообщении.
	f, err := os.OpenFile(l.bootPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open boot.log: %v\n", err)
		return
	}
	l.bootFile = f

	now := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(f, "\n=== BOOT %s ===\n", now)
}

func (l *Logger) FlushBootLog() {
	l.bootMu.Lock()
	defer l.bootMu.Unlock()

	if l.bootFile == nil {
		return
	}
	l.bootFile.Sync()
	l.bootFile.Close()
	l.bootFile = nil

	data, err := os.ReadFile(l.bootPath)
	if err != nil {
		return
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "===") {
			continue
		}
		// Парсим "[ts] [level] [category] message" — восстанавливаем
		// оригинальную категорию и чистое сообщение вместо того, чтобы
		// вставлять всю строку с префиксом в категорию «system».
		if m := bootLineRe.FindStringSubmatch(line); m != nil {
			lvl := m[1]
			if lvl == "" {
				lvl = "INFO"
			}
			logdb.Insert(m[2], lvl, "", m[3])
		} else {
			logdb.Insert("system", "INFO", "", line)
		}
	}
	// Очищаем файл, чтобы при повторном закрытии не вставлять дубликаты.
	os.Truncate(l.bootPath, 0)
}

func (l *Logger) Close() {
	l.FlushBootLog()
	close(l.stopCh)
}

func (l *Logger) writeBoot(level, msg string) {
	l.bootMu.Lock()
	defer l.bootMu.Unlock()
	if l.bootFile == nil {
		return
	}
	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(l.bootFile, "[%s] [%s] %s\n", ts, level, msg)
}

func (l *Logger) Info(category, msg string) {
	l.write("INFO", category, "", msg)
}

func (l *Logger) Error(category, msg string) {
	l.write("ERROR", category, "", msg)
}

func (l *Logger) ErrorCode(category, code, msg string) {
	l.write("ERROR", category, code, msg)
}

func (l *Logger) Warn(category, msg string) {
	l.write("WARN", category, "", msg)
}

func (l *Logger) Debug(category, msg string) {
	l.mu.Lock()
	debug := l.debugCategories[category]
	l.mu.Unlock()
	if !debug {
		return
	}
	l.write("DEBUG", category, "", msg)
}

func (l *Logger) Network(msg string) {
	l.write("DEBUG", "network", "", msg)
}

func (l *Logger) ZapretLog(msg string) {
	l.write("INFO", "zapret", "", msg)
}

func (l *Logger) WriteZapretOutput(line string) {
	l.Debug("zapret", strings.TrimRight(line, "\r\n"))
}

func (l *Logger) SetOnError(fn func(code, msg string)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onError = fn
}

func (l *Logger) SetDebug(category string, enabled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.debugCategories[category] = enabled
}

func (l *Logger) IsDebug(category string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.debugCategories[category]
}

// SetConsoleOutput включает/выключает дублирование логов в stdout.
// Выключается в режимах, где stdout должен быть чистым (например,
// selfupdate --check выводит туда JSON состояния — лог-строки ломают парсинг).
func (l *Logger) SetConsoleOutput(enabled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.console = enabled
}

func (l *Logger) GetDebugCategories() map[string]bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make(map[string]bool)
	for k, v := range l.debugCategories {
		result[k] = v
	}
	return result
}

func (l *Logger) write(level, category, code, msg string) {
	now := time.Now()
	timestamp := now.Format("2006-01-02 15:04:05")

	if l.console {
		consoleLine := fmt.Sprintf("[%s] [%s] [%s] %s\n", timestamp, level, category, msg)
		fmt.Print(consoleLine)
	}

	// boot.log нужен только для записей, сделанных ДО готовности БД логов
	// (иначе каждая запись дублировалась бы: один раз в БД через Insert,
	// второй — в boot.log, который затем повторно вставлялся как «system»).
	if !logdb.Ready() {
		l.writeBoot(level, fmt.Sprintf("[%s] %s", category, msg))
	}

	l.mu.Lock()
	l.ring = append(l.ring, ringEntry{t: now, level: level, category: category, code: code, msg: msg})
	if len(l.ring) > ringMax {
		l.ring = l.ring[len(l.ring)-ringMax:]
	}
	onError := l.onError
	l.mu.Unlock()

	logdb.Insert(category, level, code, msg)

	if level == "ERROR" {
		l.mu.Lock()
		if now.Sub(l.lastSnap) >= 30*time.Second {
			l.lastSnap = now
			l.flushErrorSnapshot(now, category, msg)
		}
		l.mu.Unlock()

		if onError != nil {
			go onError(code, msg)
		}
	}
}

func (l *Logger) flushErrorSnapshot(now time.Time, category, msg string) {
	cutoff := now.Add(-1 * time.Hour)
	var entries []ringEntry
	l.mu.Lock()
	for _, e := range l.ring {
		if e.t.After(cutoff) {
			entries = append(entries, e)
		}
	}
	l.mu.Unlock()

	if len(entries) == 0 {
		return
	}

	errorsDir := filepath.Join(l.baseDir, "errors")
	os.MkdirAll(errorsDir, 0755)

	fname := fmt.Sprintf("error-%s.log", now.Format("2006-01-02_150405"))
	path := filepath.Join(errorsDir, fname)

	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "=== ERROR SNAPSHOT ===\n")
	fmt.Fprintf(f, "Time:  %s\n", now.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(f, "Cat:   %s\n", category)
	fmt.Fprintf(f, "Error: %s\n", msg)
	fmt.Fprintf(f, "Entries (last hour): %d\n", len(entries))
	fmt.Fprintf(f, "\n--- Log timeline ---\n\n")

	for _, e := range entries {
		ts := e.t.Format("2006-01-02 15:04:05")
		marker := ""
		if e.level == "ERROR" {
			marker = " <<<"
		}
		codeStr := ""
		if e.code != "" {
			codeStr = " [" + e.code + "]"
		}
		fmt.Fprintf(f, "[%s] [%s] [%s]%s %s%s\n", ts, e.level, e.category, codeStr, e.msg, marker)
	}
}

func (l *Logger) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-l.stopCh:
			return
		case <-ticker.C:
			l.cleanup()
		}
	}
}

func (l *Logger) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoffDate := time.Now().AddDate(0, 0, -30)

	errorsDir := filepath.Join(l.baseDir, "errors")
	if entries, err := os.ReadDir(errorsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoffDate) {
				os.Remove(filepath.Join(errorsDir, e.Name()))
			}
		}
	}

	logdb.CleanOld(30 * 24 * time.Hour)
}

func (l *Logger) ReadRecent(table logdb.LogTable, level string, limit, offset int) ([]logdb.LogEntry, error) {
	return logdb.Query(table, level, limit, offset)
}

func (l *Logger) ReadByID(table logdb.LogTable, id int64) (*logdb.LogEntry, error) {
	return logdb.QueryByID(table, id)
}

func (l *Logger) GetStats() logdb.LogStats {
	return logdb.GetStats()
}

func (l *Logger) Count(table logdb.LogTable, level string) int64 {
	return logdb.Count(table, level)
}

func (l *Logger) Clear(table logdb.LogTable) error {
	return logdb.Clear(table)
}

func (l *Logger) ClearAll() error {
	return logdb.Clear("all")
}

func (l *Logger) ListTables() []logdb.LogTable {
	return logdb.AllTables
}

func (l *Logger) ClearErrorSnapshots() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ring = nil
	errorsDir := filepath.Join(l.baseDir, "errors")
	os.RemoveAll(errorsDir)
	os.MkdirAll(errorsDir, 0755)
}

func (l *Logger) ListErrorSnapshots() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	var names []string
	errorsDir := filepath.Join(l.baseDir, "errors")
	if entries, err := os.ReadDir(errorsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".log") {
				names = append(names, e.Name())
			}
		}
	}
	sort.Strings(names)
	return names
}

func (l *Logger) ReadErrorSnapshot(name string) (string, error) {
	clean := filepath.Base(name)
	path := filepath.Join(l.baseDir, "errors", clean)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
