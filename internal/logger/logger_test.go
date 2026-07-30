package logger

import (
	"os"
	"path/filepath"
	"strings"
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
	if fi, err := os.Stat(filepath.Join(base, "errors")); err != nil || !fi.IsDir() {
		t.Errorf("errors dir not created: %v", err)
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

func TestNewErrorsDirMkdirError(t *testing.T) {
	base := filepath.Join(t.TempDir(), "base")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "errors"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(base, testLogdbDir); err == nil {
		t.Fatal("New should fail when errors path is a file")
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
	l.Debug("mycat", "should be dropped")
	time.Sleep(30 * time.Millisecond)
	if got := l.Count(logdb.TableApp, "ALL"); got != 0 {
		t.Errorf("Debug with disabled category should not write; count = %d", got)
	}
}

func TestDebugEnabled(t *testing.T) {
	l := newLogger(t)
	l.SetConsoleOutput(false)
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

func TestErrorSnapshotThrottle(t *testing.T) {
	l := newLogger(t)
	l.SetConsoleOutput(false)
	l.Error("app", "first error")
	time.Sleep(100 * time.Millisecond)
	l.Error("app", "second error")
	time.Sleep(100 * time.Millisecond)

	snaps := l.ListErrorSnapshots()
	if len(snaps) == 0 {
		t.Fatal("expected at least one error snapshot after first ERROR")
	}
	first := len(snaps)
	l.Error("app", "third error within throttle")
	time.Sleep(100 * time.Millisecond)
	if got := len(l.ListErrorSnapshots()); got != first {
		t.Errorf("throttled error should not produce new snapshot; before=%d after=%d", first, got)
	}
}

func TestReadAndClearErrorSnapshot(t *testing.T) {
	l := newLogger(t)
	l.SetConsoleOutput(false)
	l.Error("app", "snapshot trigger")
	time.Sleep(150 * time.Millisecond)

	snaps := l.ListErrorSnapshots()
	if len(snaps) == 0 {
		t.Fatal("no snapshots")
	}
	content, err := l.ReadErrorSnapshot(snaps[0])
	if err != nil {
		t.Fatalf("ReadErrorSnapshot: %v", err)
	}
	if !strings.Contains(content, "ERROR SNAPSHOT") {
		t.Errorf("snapshot content unexpected: %q", content)
	}

	if _, err := l.ReadErrorSnapshot("does-not-exist.log"); err == nil {
		t.Error("ReadErrorSnapshot missing file should error")
	}

	l.ClearErrorSnapshots()
	if got := l.ListErrorSnapshots(); len(got) != 0 {
		t.Errorf("after ClearErrorSnapshots got %d", len(got))
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

func TestRingTrim(t *testing.T) {
	l := newLogger(t)
	l.SetConsoleOutput(false)
	for i := 0; i <= ringMax; i++ {
		l.Info("app", "fill")
	}

	l.mu.Lock()
	n := len(l.ring)
	l.mu.Unlock()
	if n != ringMax {
		t.Errorf("ring size = %d, want %d (trimmed)", n, ringMax)
	}
}

func TestCleanupRemovesOldFiles(t *testing.T) {
	l := newLogger(t)
	l.SetConsoleOutput(false)

	errDir := filepath.Join(l.baseDir, "errors")
	old := filepath.Join(errDir, "error-old.log")
	if err := os.WriteFile(old, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().AddDate(0, 0, -60)
	if err := os.Chtimes(old, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(errDir, "error-fresh.log")
	if err := os.WriteFile(fresh, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	l.cleanup()

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old snapshot should be removed: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh snapshot should remain: %v", err)
	}
}

func TestCleanupMissingDir(t *testing.T) {
	l := newLogger(t)
	os.RemoveAll(filepath.Join(l.baseDir, "errors"))
	l.cleanup()
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

func TestReadErrorSnapshotPathBase(t *testing.T) {
	l := newLogger(t)
	l.SetConsoleOutput(false)
	l.Error("app", "snap")
	time.Sleep(150 * time.Millisecond)
	snaps := l.ListErrorSnapshots()
	if len(snaps) == 0 {
		t.Fatal("no snapshots")
	}
	if _, err := l.ReadErrorSnapshot("../" + snaps[0]); err != nil {
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
