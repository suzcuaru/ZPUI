package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"zpui/internal/config"
	"zpui/internal/database"
	"zpui/internal/logger"
	"zpui/internal/netmetrics"
)

// Сэмплер netmetrics: замер пишется в БД, окно чтения его отдаёт,
// записи старше часа в окно не попадают.
func TestMeasureAndStoreNetWritesAndReads(t *testing.T) {
	dir := t.TempDir()

	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")
	logDir, err := os.MkdirTemp("", "zpui-test-logdb")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(logDir)
	logMgr, err := logger.New(dir, logDir)
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	defer logMgr.Close()
	a := &App{cfg: cfg, log: logMgr}

	orig := netMeasure
	netMeasure = func() netmetrics.Result {
		return netmetrics.Result{PingMs: 21, SpeedMbps: 42.5, At: time.Now()}
	}
	defer func() { netMeasure = orig }()

	a.measureAndStoreNet()

	res := a.GetYandexNetMetrics(60)
	if isErrorResp(res) {
		t.Fatalf("GetYandexNetMetrics: %v", res["error"])
	}
	if count := res["count"].(int); count < 1 {
		t.Fatalf("count = %d, want >= 1", count)
	}
	hist := res["metrics"].([]database.YandexMetric)
	if hist[0].PingMs != 21 || hist[0].SpeedMbps < 42.4 || hist[0].SpeedMbps > 42.6 {
		t.Fatalf("stored metric mismatch: %+v", hist[0])
	}
	latest, err := database.GetLatestYandexMetric()
	if err != nil || latest == nil || latest.PingMs != 21 {
		t.Fatalf("latest metric: %+v err=%v", latest, err)
	}

	// Старая запись (час назад + запас) в часовое окно не возвращается.
	old := time.Now().Add(-2 * time.Hour)
	if _, err := database.DB().Exec(`UPDATE yandex_metrics SET timestamp = ?`, old); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	res = a.GetYandexNetMetrics(60)
	if count := res["count"].(int); count != 0 {
		t.Fatalf("count after backdate = %d, want 0 (read window is 1h)", count)
	}
}
