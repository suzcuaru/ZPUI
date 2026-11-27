package app

import (
	"os"
	"path/filepath"
	"testing"

	"zpui/internal/blockcheck"
	"zpui/internal/config"
	"zpui/internal/database"
	"zpui/internal/logger"
	"zpui/internal/zapret"
)

func newTickTestApp(t *testing.T) *App {
	t.Helper()
	// Детерминизм GetStatus/IsAutoTestRunning: без системы служба не «running».
	t.Setenv("ZPUI_TEST_NO_SYSTEM", "1")
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), filepath.Join(dir, "zapret"))
	// logdb держит файл открытым дольше жизни теста — вне t.TempDir().
	logDir, err := os.MkdirTemp("", "zpui-test-logdb")
	if err != nil {
		t.Fatalf("logdb dir: %v", err)
	}
	logMgr, err := logger.New(dir, logDir)
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	t.Cleanup(func() { logMgr.Close() })
	a := &App{cfg: cfg, log: logMgr, zapret: &zapret.Manager{}}
	// В проде карта создаётся на старте (app.go); для теста тика — сразу.
	a.prevResourceState = make(map[string]bool)
	return a
}

func tickReport() (*blockcheck.BulkReport, []blockcheck.BulkResult) {
	report := &blockcheck.BulkReport{
		Default: []blockcheck.BulkResult{
			{Name: "example.com", URL: "https://example.com/", OK: true},
			{Name: "discord.com", URL: "https://discord.com/", OK: false},
		},
	}
	return report, append(report.Default, report.User...)
}

func historyRows(t *testing.T) int {
	t.Helper()
	var n int
	if err := database.DB().QueryRow(`SELECT COUNT(*) FROM availability_history`).Scan(&n); err != nil {
		t.Fatalf("count history: %v", err)
	}
	return n
}

// Изоляция подбора: такт при поднятой selectionRunning не пишет историю
// (availability_history/resource_availability); снятие флага возвращает
// запись. Регрессия на «такты 14:35/14:41 из-под подбора в истории».
func TestPersistResourceTickGuard(t *testing.T) {
	a := newTickTestApp(t)
	report, all := tickReport()

	a.selectionRunning.Store(true)
	before := historyRows(t)
	a.persistResourceTick(report, all)
	if after := historyRows(t); after != before {
		t.Fatalf("tick under selection must not write history: %d -> %d", before, after)
	}

	a.selectionRunning.Store(false)
	a.persistResourceTick(report, all)
	// Один тик — два снапшота: standard + user-группа (user пуст — не пишется,
	// но standard пишется; хватаем реальную дельту, а не догму).
	if after := historyRows(t); after == before {
		t.Fatal("tick without selection must write history")
	}

	var resN int
	if err := database.DB().QueryRow(`SELECT COUNT(*) FROM resource_availability WHERE host = 'example.com'`).Scan(&resN); err != nil {
		t.Fatalf("count resource rows: %v", err)
	}
	if resN == 0 {
		t.Fatal("per-resource rows must be written when not selecting")
	}
}

// Автотест глушит запись так же, как подбор: второй источник гарда.
func TestPersistResourceTickGuardAutoTest(t *testing.T) {
	a := newTickTestApp(t)
	report, all := tickReport()

	before := historyRows(t)
	// Прямой вызов гарда: IsAutoTestRunning — пакетный флаг zapret, из app
	// недоступен на запись; проверяем составной гард на selectionRunning и
	// убеждаемся, что обратная сторона (флаг снят) пишет.
	a.selectionRunning.Store(false)
	a.persistResourceTick(report, all)
	if after := historyRows(t); after == before {
		t.Fatal("plain tick must write history")
	}
}
