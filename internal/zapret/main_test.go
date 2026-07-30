package zapret

import (
	"os"
	"path/filepath"
	"testing"

	"zpui/internal/config"
	"zpui/internal/logger"
)

var testLog *logger.Logger

// TestMain создаёт один общий логгер для всех тестов пакета (logdb.Init
// использует глобальное состояние, поэтому инициализируем его один раз,
// а не на каждый тест).
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "zapret-tests")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	testLog, err = logger.New(dir, filepath.Join(dir, "logdb"))
	if err != nil {
		panic(err)
	}

	code := m.Run()
	testLog.Close()
	os.Exit(code)
}

// newTestManager строит Manager напрямую (без NewManager), чтобы не вызывать
// isServiceRunning → "sc query". Zapret-директория создаётся во временной папке.
func newTestManager(t *testing.T) (*Manager, *config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	zapretDir := filepath.Join(dir, "zapret")
	if err := os.MkdirAll(zapretDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "config.json")
	cfg := config.Load(cfgPath, zapretDir)

	m := &Manager{
		cfg:     cfg,
		log:     testLog,
		status:  StatusStopped,
		version: detectZapretVersion(cfg),
	}
	_, m.gameFilterTCP, m.gameFilterUDP = m.LoadGameFilter()
	return m, cfg, zapretDir
}

// resetAutoTestState сбрасывает глобальное состояние автотеста между тестами.
func resetAutoTestState() {
	autoTestMu.Lock()
	autoTestActive = false
	autoTestCancel = nil
	autoTestMu.Unlock()
}
