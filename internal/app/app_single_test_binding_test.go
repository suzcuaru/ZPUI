package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"zpui/internal/config"
	"zpui/internal/logger"
	"zpui/internal/zapret"
)

// Биндинг ПКМ-проверки не должен блокироваться сетью, а проверка в горутине
// не должна ждать RefreshAutoSelectCfg (sync.pull, таймаут до 20с): refresh
// идёт параллельно проверке. Регрессия на «первый клик не срабатывает —
// нужно жать дважды» и «модалка висит на Применение... на холодном
// соединении»: медленный сервер не задерживает ни ответ биндинга, ни старт
// проверки; результат приходит через polling.
func TestTestSingleStrategyBindingNotBlocked(t *testing.T) {
	t.Setenv("ZPUI_TEST_NO_SYSTEM", "1")

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()

	oldURL := ServerURL
	ServerURL = slow.URL
	t.Cleanup(func() { ServerURL = oldURL })

	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), filepath.Join(dir, "zapret"))
	logDir, err := os.MkdirTemp("", "zpui-test-logdb")
	if err != nil {
		t.Fatalf("logdb dir: %v", err)
	}
	logMgr, err := logger.New(dir, logDir)
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	t.Cleanup(func() { logMgr.Close() })
	a := &App{cfg: cfg, log: logMgr, zapret: zapret.NewManager(cfg, logMgr)}

	// Гард пустого filename до всякой сети и состояния.
	if resp := a.TestSingleStrategy("", false); resp["error"] == nil {
		t.Fatalf("empty filename must be rejected, got %v", resp)
	}
	if st := a.GetSingleTestResult(); st["running"] != false {
		t.Fatalf("no run yet: %+v", st)
	}

	start := time.Now()
	resp := a.TestSingleStrategy("nonexistent.bat", false)
	elapsed := time.Since(start)
	if resp["status"] != "ok" {
		t.Fatalf("binding must return ok immediately, got %v", resp)
	}
	if elapsed >= 1200*time.Millisecond {
		t.Fatalf("binding blocked %v on autoselect refresh — sync network in binding is back", elapsed)
	}

	// Горутина жива: polling доводит до результата (SetStrategy: файла нет).
	// Строгий дедлайн (< задержка медленного сервера): проверка стартует
	// сразу, не дожидаясь параллельного RefreshAutoSelectCfg — при серийном
	// refresh результат не мог бы прийти раньше 1.5с.
	var st map[string]interface{}
	deadline := start.Add(1200 * time.Millisecond)
	for {
		st = a.GetSingleTestResult()
		if st["running"] == false && st["result"] != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("single test result blocked behind autoselect refresh (%v): %+v", time.Since(start), st)
		}
		time.Sleep(50 * time.Millisecond)
	}
	res := st["result"].(map[string]interface{})
	if res["phase"] != "done" {
		t.Fatalf("expected done phase, got %v", res["phase"])
	}
	if _, hasErr := res["error"]; !hasErr {
		t.Fatalf("nonexistent strategy must carry SetStrategy error: %+v", res)
	}

	// Параллельный sync.pull ещё идёт на медленном сервере — дать ему
	// завершиться до восстановления ServerURL (нет гонки на глобал).
	if time.Since(start) < 1600*time.Millisecond {
		time.Sleep(1600*time.Millisecond - time.Since(start))
	}
}
