package tgproxy

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseHumanBytes(t *testing.T) {
	cases := map[string]int64{
		"123.0B": 123,
		"1.5KB":  1536,
		"2.0MB":  2097152,
		"1.0GB":  1073741824,
		"1.2MB":  1258291,
		"0B":     0,
		"3TB":    3298534883328,
	}
	for in, want := range cases {
		got, ok := parseHumanBytes(in)
		if !ok {
			t.Fatalf("parseHumanBytes(%q) not ok", in)
		}
		diff := got - want
		if diff < 0 {
			diff = -diff
		}
		if diff > 1024 {
			t.Fatalf("parseHumanBytes(%q) = %d, want ~%d", in, got, want)
		}
	}
	if _, ok := parseHumanBytes("garbage"); ok {
		t.Fatalf("garbage parsed ok")
	}
}

func TestLogWatcherStatsAndSessions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tgproxy.log")
	logContent := "2026-10-31 08:00:00  INFO   TG WS Proxy версия 1.11.3 starting\n" +
		"2026-10-31 08:00:01  INFO   [192.168.1.5:44322] DC2  -> WS pool hit\n" +
		"2026-10-31 08:00:02  INFO   [192.168.1.5:44323] DC4  -> trying CF proxy\n" +
		"2026-10-31 08:00:03  INFO   [192.168.1.9:50000] DC2  TCP fallback to 149.154.167.220:443\n" +
		"2026-10-31 08:00:30  INFO   [192.168.1.5:44322] DC2 media WS session closed (eof): ^1.2MB (10 pkts) v3.4MB (20 pkts) in 60.0s\n" +
		"2026-10-31 08:01:00  INFO   stats: total=5 active=2 ws=3 h2=1 h2_tcp=0 h2_req=10 h2_err=0 tcp_fb=1 cf=1 front=0 bad=0 masked=0 err=0 pool=3/4 cf_pool=1/2 up=1.2MB down=3.4MB\n"
	if err := os.WriteFile(path, []byte(logContent), 0o644); err != nil {
		t.Fatal(err)
	}

	w := startLogWatcher(path, nil)
	t.Cleanup(w.stop)

	deadline := time.Now().Add(5 * time.Second)
	var st map[string]interface{}
	var ip map[string]int
	// total появляется сразу из строк сессий; ждём bytes_in — маркер того,
	// что минутная stats-строка уже разобрана (абсолютные байты).
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		st = w.countersSnapshot()
		if _, ok := st["bytes_in"]; ok {
			ip = w.clientsByIP()
			break
		}
	}

	if st["total"].(int64) != 5 {
		t.Fatalf("stats total (resynced from stats line): %v", st)
	}
	if w.activeCount() != 2 {
		t.Fatalf("live active sessions: %d", w.activeCount())
	}
	if st["ws_ok"].(int64) != 3 || st["ws_errors"].(int64) != 0 {
		t.Fatalf("stats ws mapping: %v", st)
	}
	if st["tcp_fallback"].(int64) != 1 || st["cfproxy"].(int64) != 1 {
		t.Fatalf("stats fallback mapping: %v", st)
	}
	if st["pool_hits"].(int64) != 3 || st["pool_misses"].(int64) != 1 {
		t.Fatalf("stats pool mapping: %v", st)
	}
	if st["bytes_out"].(int64) != 1258291 || st["bytes_in"].(int64) != 3565158 {
		t.Fatalf("stats bytes mapping: %v", st)
	}
	if ip["192.168.1.5"] != 1 || ip["192.168.1.9"] != 1 {
		t.Fatalf("per-IP sessions: %v", ip)
	}

	// Дописываем закрытие второй сессии 192.168.1.5.
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("2026-10-31 08:02:00  INFO   [192.168.1.5:44323] DC4  fallback closed\n")
	f.Close()

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		ip = w.clientsByIP()
		if ip["192.168.1.5"] == 1 {
			break
		}
	}
	if w.activeCount() != 2 {
		t.Fatalf("live active after close: %d", w.activeCount())
	}
	if st := w.countersSnapshot(); st["bytes_in"].(int64) < 3565158 {
		t.Fatalf("close-line bytes must accumulate bytes_in, got %v", st)
	}
	if ip["192.168.1.5"] != 1 || ip["192.168.1.9"] != 1 {
		t.Fatalf("per-IP after close: %v", ip)
	}
}

// Двойной stop() (Manager.Stop + supervise при выходе процесса) не должен
// паниковать на повторном close — регрессия падения приложения при
// отключении моста.
func TestLogWatcherStopIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tgproxy.log")
	if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := startLogWatcher(path, nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.stop()
		w.stop()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("double stop() deadlocked or panicked")
	}
}

func TestSessionLabelAndIP(t *testing.T) {
	label, ok := sessionLabel("2026-10-31 08:00:00  INFO   [192.168.1.5:44322] DC2  -> WS pool hit")
	if !ok || label != "192.168.1.5:44322" {
		t.Fatalf("label: %q %v", label, ok)
	}
	if ip := labelToIP(label); ip != "192.168.1.5" {
		t.Fatalf("ip: %q", ip)
	}
	if _, ok := sessionLabel("no label here"); ok {
		t.Fatalf("unexpected label")
	}
}
