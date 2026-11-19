package logger

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"zpui/internal/logdb"
)

var testLogdbDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "logger-logdb")
	if err != nil {
		panic(err)
	}
	testLogdbDir = dir
	if err := logdb.Init(dir); err != nil {
		os.RemoveAll(dir)
		panic(err)
	}
	code := m.Run()
	logdb.Close()
	os.RemoveAll(dir)
	os.Exit(code)
}

func newLogger(t *testing.T) *Logger {
	t.Helper()
	base := filepath.Join(t.TempDir(), "logs")
	l, err := New(base, testLogdbDir)
	if err != nil {
		t.Fatalf("New logger: %v", err)
	}
	t.Cleanup(func() {
		l.SetConsoleOutput(false)
		l.Close()
	})
	return l
}

func TestNewCreatesDirs(t *testing.T) {
	base := filepath.Join(t.TempDir(), "logs")
	l, err := New(base, testLogdbDir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	if fi, err := os.Stat(base); err != nil || !fi.IsDir() {
		t.Errorf("base dir not created: %v", err)
	}
	if fi, err := os.Stat(filepath.Join(base, "boot.log")); err != nil || fi.IsDir() {
		t.Errorf("boot.log not created: %v", err)
	}
}

func TestNewBaseDirMkdirError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(filepath.Join(file, "sub"), testLogdbDir); err == nil {
		t.Fatal("New under a file should fail at baseDir MkdirAll")
	}
}

func TestInitBootLogFailure(t *testing.T) {
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	l := &Logger{
		baseDir:         file,
		debugCategories: make(map[string]bool),
		stopCh:          make(chan struct{}),
		console:         false,
	}
	l.initBootLog()
	if l.bootFile != nil {
		t.Error("initBootLog should leave bootFile nil on failure")
	}
	l.writeBoot("INFO", "should be no-op")
	l.FlushBootLog()
	close(l.stopCh)
}

func TestLevelsWrite(t *testing.T) {
	l := newLogger(t)
	l.SetConsoleOutput(false)

	l.Info("app", "info msg")
	l.Warn("app", "warn msg")
	l.Error("app", "err msg")
	l.ErrorCode("app", "ECODE", "err with code")
	l.ZapretLog("zapret line")
	l.Network("net debug")

	time.Sleep(50 * time.Millisecond)

	total := l.Count(logdb.TableApp, "ALL")
	if total < 4 {
		t.Errorf("TableApp count = %d, want >=4", total)
	}
	zap := l.Count(logdb.TableZapret, "ALL")
	if zap < 1 {
		t.Errorf("TableZapret count = %d, want >=1 (ZapretLog)", zap)
	}
}

func TestDebugDisabled(t *testing.T) {
	l := newLogger(t)
	l.SetConsoleOutput(false)
	if err := l.Clear(logdb.TableApp); err != nil {
		t.Fatalf("clear: %v", err)
	}
	l.Debug("mycat", "should be dropped")
	time.Sleep(30 * time.Millisecond)
	if got := l.Count(logdb.TableApp, "ALL"); got != 0 {
		t.Errorf("Debug with disabled category should not write; count = %d", got)
	}
}

func TestDebugEnabled(t *testing.T) {
	l := newLogger(t)
	l.SetConsoleOutput(false)
	if err := l.Clear(logdb.TableApp); err != nil {
		t.Fatalf("clear: %v", err)
	}
	l.SetDebug("mycat", true)
	if !l.IsDebug("mycat") {
		t.Fatal("IsDebug should be true after SetDebug")
	}
	l.Debug("mycat", "now written")
	time.Sleep(30 * time.Millisecond)
	if got := l.Count(logdb.TableApp, "ALL"); got != 1 {
		t.Errorf("enabled Debug should write once; count = %d", got)
	}
	if cats := l.GetDebugCategories(); !cats["mycat"] {
		t.Errorf("GetDebugCategories missing mycat: %+v", cats)
	}
	l.SetDebug("mycat", false)
	if l.IsDebug("mycat") {
		t.Error("IsDebug should be false after disable")
	}
}

func TestWriteZapretOutput(t *testing.T) {
	l := newLogger(t)
	l.SetConsoleOutput(false)
	l.SetDebug("zapret", true)
	l.WriteZapretOutput("  trimmed line\r\n")
	time.Sleep(30 * time.Millisecond)
}

func TestSetOnError(t *testing.T) {
	l := newLogger(t)
	l.SetConsoleOutput(false)

	got := make(chan string, 2)
	l.SetOnError(func(code, msg string) {
		got <- code + "|" + msg
	})
	l.ErrorCode("app", "OCODE", "trigger")
	select {
	case s := <-got:
		if s != "OCODE|trigger" {
			t.Errorf("onError got %q", s)
		}
	case <-time.After(time.Second):
		t.Fatal("onError callback not invoked")
	}
}

// ErrorCodeShown — ошибка, уже показанная тостом: запись в журнал есть,
// хук onError (OS-уведомление + очередь сервера) не зовётся.
func TestErrorCodeShownSkipsOnError(t *testing.T) {
	l := newLogger(t)
	l.SetConsoleOutput(false)

	fired := make(chan string, 2)
	l.SetOnError(func(code, msg string) {
		fired <- code
	})

	l.ErrorCodeShown("frontend", "ui_error", "toast error")
	select {
	case code := <-fired:
		t.Errorf("onError fired for ErrorCodeShown with %q, want silent", code)
	case <-time.After(150 * time.Millisecond):
	}

	time.Sleep(50 * time.Millisecond)
	if n := l.Count(logdb.TableApp, "ERROR"); n < 1 {
		t.Errorf("TableApp ERROR count = %d, want >=1 (ErrorCodeShown must be logged)", n)
	}
}

func TestFlushBootLogParsesLines(t *testing.T) {
	l := newLogger(t)
	l.SetConsoleOutput(false)

	l.writeBoot("ERROR", "[zapret] boom from boot")
	l.writeBoot("", "[system] empty level line")

	bootPath := l.bootPath
	f, err := os.OpenFile(bootPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("unmatched plain text line\n")
	f.WriteString("=== BOOT HEADER ===\n")
	f.Close()

	before := l.Count(logdb.TableZapret, "ERROR")
	l.FlushBootLog()
	after := l.Count(logdb.TableZapret, "ERROR")
	if after <= before {
		t.Errorf("FlushBootLog should insert parsed zapret error; before=%d after=%d", before, after)
	}

	sysCount := l.Count(logdb.TableApp, "ALL")
	if sysCount < 1 {
		t.Errorf("FlushBootLog should insert system lines; TableApp=%d", sysCount)
	}

	l.FlushBootLog()
}

func TestFlushBootLogNilFile(t *testing.T) {
	l := &Logger{
		baseDir:         t.TempDir(),
		debugCategories: make(map[string]bool),
		stopCh:          make(chan struct{}),
		console:         false,
	}
	l.FlushBootLog()
	close(l.stopCh)
}

func TestSetConsoleOutput(t *testing.T) {
	l := newLogger(t)
	l.SetConsoleOutput(false)
	l.Info("app", "quiet")
	l.SetConsoleOutput(true)
	l.Info("app", "loud")
}

func TestDelegationMethods(t *testing.T) {
	l := newLogger(t)
	l.SetConsoleOutput(false)
	l.Info("app", "x")
	l.Error("app", "y")
	time.Sleep(50 * time.Millisecond)

	if _, err := l.ReadRecent(logdb.TableApp, "", 10, 0); err != nil {
		t.Errorf("ReadRecent: %v", err)
	}
	stats := l.GetStats()
	if stats.Total < 2 {
		t.Errorf("GetStats Total = %d, want >=2", stats.Total)
	}
	tables := l.ListTables()
	if len(tables) != len(logdb.AllTables) {
		t.Errorf("ListTables len = %d, want %d", len(tables), len(logdb.AllTables))
	}

	entries, _ := l.ReadRecent(logdb.TableApp, "", 10, 0)
	if len(entries) > 0 {
		_, err := l.ReadByID(logdb.TableApp, entries[0].ID)
		if err != nil {
			t.Errorf("ReadByID: %v", err)
		}
	}

	if err := l.Clear(logdb.TableApp); err != nil {
		t.Errorf("Clear: %v", err)
	}
	if got := l.Count(logdb.TableApp, "ALL"); got != 0 {
		t.Errorf("after Clear count = %d", got)
	}
}

func TestClearAll(t *testing.T) {
	l := newLogger(t)
	l.SetConsoleOutput(false)
	l.Info("app", "x")
	time.Sleep(30 * time.Millisecond)
	if err := l.ClearAll(); err != nil {
		t.Fatalf("ClearAll: %v", err)
	}
	for _, table := range l.ListTables() {
		if got := l.Count(table, "ALL"); got != 0 {
			t.Errorf("after ClearAll %s count = %d", table, got)
		}
	}
}

func TestCloseStopsCleanupLoop(t *testing.T) {
	base := filepath.Join(t.TempDir(), "logs")
	l, err := New(base, testLogdbDir)
	if err != nil {
		t.Fatal(err)
	}
	l.SetConsoleOutput(false)
	l.Close()
	select {
	case <-l.stopCh:
	default:
		t.Error("Close should close stopCh")
	}
}
