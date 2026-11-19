package isp

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"zpui/internal/database"
)

// TestMain инициализирует общую БД один раз на весь пакет и гарантированно
// закрывает её до удаления каталога — иначе Windows не сможет удалить test.db,
// удерживаемый открытым соединением.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "isp-test-db")
	if err != nil {
		panic(err)
	}
	if err := database.Init(filepath.Join(dir, "test.db")); err != nil {
		os.RemoveAll(dir)
		panic(err)
	}
	code := m.Run()
	database.Close()
	os.RemoveAll(dir)
	os.Exit(code)
}

// initTestDB — БД уже инициализирована в TestMain; хелпер оставлен для
// читаемости в местах, где тест явно отмечает зависимость от БД.
func initTestDB(t *testing.T) {
	t.Helper()
}

func clearOperatorTables(t *testing.T) {
	t.Helper()
	_, err := database.DB().Exec(`DELETE FROM current_operator; DELETE FROM operator_info;`)
	if err != nil {
		t.Fatalf("clear tables: %v", err)
	}
}

func useFailingClient(t *testing.T) {
	t.Helper()
	orig := client
	client = newFailingClient()
	origWhois := whoisFallback
	whoisFallback = func(string) *Operator { return nil }
	t.Cleanup(func() {
		client = orig
		whoisFallback = origWhois
	})
}

func TestNewMonitor_GetCurrentKey(t *testing.T) {
	m := NewMonitor(func(string, string) {}, time.Second, nil)
	if m == nil {
		t.Fatal("nil monitor")
	}
	if m.GetCurrentKey() != "" {
		t.Errorf("fresh monitor key = %q, want empty", m.GetCurrentKey())
	}
	if m.checkInterval != time.Second {
		t.Errorf("checkInterval = %v, want %v", m.checkInterval, time.Second)
	}
}

func TestMonitor_Stop(t *testing.T) {
	m := NewMonitor(func(string, string) {}, time.Second, nil)
	m.Stop()
}

func TestDetectOnce_NoNetwork(t *testing.T) {
	useFailingClient(t)
	op, err := DetectOnce()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if op.Key != "unknown" {
		t.Errorf("Key = %q, want unknown", op.Key)
	}
}

func TestDetectAndSave_FreshDetected(t *testing.T) {
	initTestDB(t)
	useFailingClient(t)
	clearOperatorTables(t)

	changed := false
	m := NewMonitor(func(cat, msg string) {
		if cat == "isp" && msg != "" {
			t.Logf("log: %s", msg)
		}
	}, time.Minute, func(c OperatorChange) {
		changed = true
		if c.NewKey != "unknown" {
			t.Errorf("NewKey = %q", c.NewKey)
		}
		if c.OldKey != "" {
			t.Errorf("OldKey = %q, want empty on fresh", c.OldKey)
		}
	})

	m.detectAndSave()

	if !changed {
		t.Error("expected onChange to fire on fresh detection")
	}
	if m.GetCurrentKey() != "unknown" {
		t.Errorf("current key = %q, want unknown", m.GetCurrentKey())
	}

	cur, err := database.GetCurrentOperator()
	if err != nil || cur == nil {
		t.Fatalf("current operator not persisted: %v", err)
	}
	if cur.OperatorKey != "unknown" {
		t.Errorf("db current key = %q", cur.OperatorKey)
	}
}

func TestDetectAndSave_NoChange(t *testing.T) {
	initTestDB(t)
	useFailingClient(t)
	clearOperatorTables(t)

	// Первый монитор засевает текущего оператора ("unknown"). Первичное
	// обнаружение легитимно вызывает onChange (см. TestDetectAndSave_FreshDetected),
	// поэтому callback здесь не проверяется.
	m := NewMonitor(func(string, string) {}, time.Minute, nil)
	m.detectAndSave()

	fired := false
	m2 := NewMonitor(func(string, string) {}, time.Minute, func(c OperatorChange) {
		fired = true
	})
	m2.detectAndSave()

	if fired {
		t.Error("onChange should not fire when operator unchanged")
	}
}

func TestDetectAndSave_OperatorChanged(t *testing.T) {
	initTestDB(t)
	useFailingClient(t)
	clearOperatorTables(t)

	if err := database.SetCurrentOperator("prev-isp", "PreviousISP", ""); err != nil {
		t.Fatalf("seed current: %v", err)
	}

	fired := false
	m := NewMonitor(func(cat, msg string) {
		t.Logf("%s: %s", cat, msg)
	}, time.Minute, func(c OperatorChange) {
		fired = true
		if c.OldKey != "prev-isp" {
			t.Errorf("OldKey = %q, want prev-isp", c.OldKey)
		}
		if c.OldName != "PreviousISP" {
			t.Errorf("OldName = %q", c.OldName)
		}
		if c.NewKey != "unknown" {
			t.Errorf("NewKey = %q, want unknown", c.NewKey)
		}
		if c.Op == nil {
			t.Error("Op is nil")
		}
	})

	m.detectAndSave()

	if !fired {
		t.Error("expected onChange to fire on operator change")
	}
}

func TestStart_Stop(t *testing.T) {
	initTestDB(t)
	useFailingClient(t)

	m := NewMonitor(func(string, string) {}, time.Hour, func(OperatorChange) {})

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Start()
	}()

	time.Sleep(150 * time.Millisecond)
	m.Stop()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after Stop within 2s")
	}
}
