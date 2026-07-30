package logdb

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var testDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "logdb-test")
	if err != nil {
		panic(err)
	}
	testDir = dir
	if err := Init(dir); err != nil {
		os.RemoveAll(dir)
		panic(err)
	}
	code := m.Run()
	Close()
	os.RemoveAll(dir)
	os.Exit(code)
}

func resetDB(t *testing.T) {
	t.Helper()
	for _, table := range AllTables {
		if _, err := db.Exec("DELETE FROM " + string(table)); err != nil {
			t.Fatalf("reset %s: %v", table, err)
		}
	}
}

func rawInsert(t *testing.T, table, ts, level, code, msg, category string) {
	t.Helper()
	if _, err := db.Exec(
		"INSERT INTO "+table+" (timestamp, level, code, message, category) VALUES (?, ?, ?, ?, ?)",
		ts, level, code, msg, category,
	); err != nil {
		t.Fatalf("raw insert: %v", err)
	}
}

func TestReady(t *testing.T) {
	if !Ready() {
		t.Fatal("Ready() should be true after Init")
	}
	saved := db
	db = nil
	t.Cleanup(func() { db = saved })
	if Ready() {
		t.Fatal("Ready() should be false when db is nil")
	}
}

func TestInitIdempotent(t *testing.T) {
	if err := Init(testDir); err != nil {
		t.Fatalf("second Init should be no-op nil, got %v", err)
	}
}

func TestInitMkdirError(t *testing.T) {
	saved := db
	db = nil
	t.Cleanup(func() { db = saved })

	file := filepath.Join(t.TempDir(), "iamfile")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Init(file); err == nil {
		t.Fatal("Init under a file path should fail")
	}
}

func TestTableCategory(t *testing.T) {
	cases := []struct {
		cat  string
		want LogTable
	}{
		{"app", TableApp},
		{"config", TableApp},
		{"proxy", TableApp},
		{"monitor", TableApp},
		{"xboxdns", TableApp},
		{"tray", TableApp},
		{"system", TableApp},
		{"network", TableApp},
		{"availability", TableApp},
		{"zapret", TableZapret},
		{"service", TableZapret},
		{"strategy", TableZapret},
		{"install", TableZapret},
		{"winws", TableZapret},
		{"updater", TableUpdater},
		{"selfupdate", TableUpdater},
		{"update", TableUpdater},
		{"report", TableReport},
		{"totally-unknown", TableApp},
		{"", TableApp},
	}
	for _, c := range cases {
		if got := tableCategory(c.cat); got != c.want {
			t.Errorf("tableCategory(%q) = %s, want %s", c.cat, got, c.want)
		}
	}
}

func TestInsertAndQuery(t *testing.T) {
	resetDB(t)

	Insert("app", "INFO", "", "hello app")
	Insert("zapret", "ERROR", "E1", "boom zapret")
	Insert("updater", "WARN", "", "warn upd")
	Insert("report", "INFO", "", "rep")
	Insert("unknown-cat", "INFO", "", "goes default")

	if got := Count(TableApp, ""); got < 2 {
		t.Errorf("TableApp count = %d, want >=2 (app + default)", got)
	}
	if got := Count(TableZapret, ""); got != 1 {
		t.Errorf("TableZapret count = %d, want 1", got)
	}
	if got := Count(TableUpdater, ""); got != 1 {
		t.Errorf("TableUpdater count = %d, want 1", got)
	}
	if got := Count(TableReport, ""); got != 1 {
		t.Errorf("TableReport count = %d, want 1", got)
	}

	entries, err := Query(TableZapret, "", 10, 0)
	if err != nil {
		t.Fatalf("Query zapret: %v", err)
	}
	if len(entries) != 1 || entries[0].Code != "E1" || entries[0].Level != "ERROR" {
		t.Errorf("unexpected zapret entry: %+v", entries)
	}
}

func TestInsertNilDB(t *testing.T) {
	saved := db
	db = nil
	t.Cleanup(func() { db = saved })
	Insert("app", "INFO", "", "should be no-op")
}

func TestQueryLevelFilter(t *testing.T) {
	resetDB(t)
	Insert("app", "INFO", "", "i1")
	Insert("app", "ERROR", "C", "e1")
	Insert("app", "ERROR", "", "e2")

	onlyErr, err := Query(TableApp, "ERROR", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyErr) != 2 {
		t.Errorf("ERROR filter count = %d, want 2", len(onlyErr))
	}
	all, err := Query(TableApp, "ALL", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Errorf("ALL count = %d, want 3", len(all))
	}
}

func TestQueryLimitOffsetAndDefaults(t *testing.T) {
	resetDB(t)
	for i := 0; i < 5; i++ {
		Insert("app", "INFO", "", "msg")
	}
	first2, _ := Query(TableApp, "", 2, 0)
	if len(first2) != 2 {
		t.Errorf("limit=2 -> %d", len(first2))
	}
	zeroLimit, _ := Query(TableApp, "", 0, 0)
	if len(zeroLimit) != 5 {
		t.Errorf("limit<=0 default 200 but only 5 rows exist -> %d", len(zeroLimit))
	}
	negOffset, _ := Query(TableApp, "", 10, -5)
	if len(negOffset) != 5 {
		t.Errorf("negative offset normalized to 0 -> %d", len(negOffset))
	}
	offset3, _ := Query(TableApp, "", 10, 3)
	if len(offset3) != 2 {
		t.Errorf("offset=3 of 5 -> %d, want 2", len(offset3))
	}
}

func TestQueryNilDB(t *testing.T) {
	saved := db
	db = nil
	t.Cleanup(func() { db = saved })
	if _, err := Query(TableApp, "", 10, 0); err == nil {
		t.Fatal("Query with nil db should error")
	}
}

func TestQueryByID(t *testing.T) {
	resetDB(t)
	rawInsert(t, "app_logs", "2024-01-01 00:00:00", "INFO", "CODE1", "msg", "app")

	var id int64
	if err := db.QueryRow("SELECT id FROM app_logs LIMIT 1").Scan(&id); err != nil {
		t.Fatal(err)
	}
	e, err := QueryByID(TableApp, id)
	if err != nil {
		t.Fatalf("QueryByID existing: %v", err)
	}
	if e.Code != "CODE1" || e.Message != "msg" {
		t.Errorf("unexpected entry %+v", e)
	}
	if _, err := QueryByID(TableApp, 99999999); err == nil {
		t.Error("QueryByID missing should return error")
	}
}

func TestQueryByIDNilDB(t *testing.T) {
	saved := db
	db = nil
	t.Cleanup(func() { db = saved })
	if _, err := QueryByID(TableApp, 1); err == nil {
		t.Fatal("QueryByID with nil db should error")
	}
}

func TestGetStats(t *testing.T) {
	resetDB(t)
	Insert("app", "INFO", "", "a")
	Insert("app", "ERROR", "", "b")
	Insert("zapret", "ERROR", "", "c")

	s := GetStats()
	if s.Total != 3 {
		t.Errorf("Total = %d, want 3", s.Total)
	}
	if s.Errors != 2 {
		t.Errorf("Errors = %d, want 2", s.Errors)
	}
	if s.DbSize <= 0 {
		t.Errorf("DbSize = %d, want >0", s.DbSize)
	}

	saved := db
	db = nil
	t.Cleanup(func() { db = saved })
	empty := GetStats()
	if empty != (LogStats{}) {
		t.Errorf("nil db GetStats = %+v, want zero", empty)
	}
}

func TestCountWithLevel(t *testing.T) {
	resetDB(t)
	Insert("app", "INFO", "", "i")
	Insert("app", "ERROR", "", "e")
	if got := Count(TableApp, "ERROR"); got != 1 {
		t.Errorf("Count ERROR = %d, want 1", got)
	}
	if got := Count(TableApp, "ALL"); got != 2 {
		t.Errorf("Count ALL = %d, want 2", got)
	}

	saved := db
	db = nil
	t.Cleanup(func() { db = saved })
	if got := Count(TableApp, ""); got != 0 {
		t.Errorf("Count nil db = %d, want 0", got)
	}
}

func TestClear(t *testing.T) {
	resetDB(t)
	Insert("app", "INFO", "", "a")
	Insert("zapret", "INFO", "", "z")

	if err := Clear(TableApp); err != nil {
		t.Fatal(err)
	}
	if got := Count(TableApp, ""); got != 0 {
		t.Errorf("after Clear TableApp count = %d", got)
	}
	if got := Count(TableZapret, ""); got != 1 {
		t.Errorf("Clear single table must not touch others; zapret = %d", got)
	}
}

func TestClearAll(t *testing.T) {
	resetDB(t)
	Insert("app", "INFO", "", "a")
	Insert("zapret", "INFO", "", "z")
	Insert("updater", "INFO", "", "u")

	if err := Clear("all"); err != nil {
		t.Fatal(err)
	}
	for _, table := range AllTables {
		if got := Count(table, ""); got != 0 {
			t.Errorf("after Clear all, %s count = %d", table, got)
		}
	}
}

func TestClearNilDB(t *testing.T) {
	saved := db
	db = nil
	t.Cleanup(func() { db = saved })
	if err := Clear(TableApp); err == nil {
		t.Fatal("Clear with nil db should error")
	}
}

func TestCleanOld(t *testing.T) {
	resetDB(t)
	Insert("app", "INFO", "", "recent")
	rawInsert(t, "app_logs", "2000-01-01 00:00:00", "INFO", "", "ancient", "app")

	before := Count(TableApp, "")
	if before != 2 {
		t.Fatalf("setup count = %d, want 2", before)
	}

	CleanOld(24 * time.Hour)

	after := Count(TableApp, "")
	if after != 1 {
		t.Errorf("after CleanOld count = %d, want 1 (ancient removed)", after)
	}
}

func TestCleanOldNilDB(t *testing.T) {
	saved := db
	db = nil
	t.Cleanup(func() { db = saved })
	CleanOld(time.Hour)
}

func TestExportAll(t *testing.T) {
	resetDB(t)
	Insert("app", "INFO", "", "plain message")
	Insert("zapret", "ERROR", "E7", "with code")
	Insert("updater", "WARN", "", "u")

	out := ExportAll()
	if out == nil {
		t.Fatal("ExportAll nil")
	}
	if _, ok := out["app_logs"]; !ok {
		t.Error("app_logs missing from export")
	}
	if appStr := out["app_logs"]; appStr == "" {
		t.Error("app_logs export empty")
	}
	if zStr := out["zapret_logs"]; !contains(zStr, "[E7]") {
		t.Errorf("zapret export should contain code: %q", zStr)
	}

	report, ok := out["report_logs"]
	if ok && report != "" {
		t.Errorf("report_logs empty table should be omitted, got %q", report)
	}

	saved := db
	db = nil
	t.Cleanup(func() { db = saved })
	if ExportAll() != nil {
		t.Error("ExportAll with nil db should return nil")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestCloseNotInitialized(t *testing.T) {
	saved := db
	db = nil
	t.Cleanup(func() { db = saved })
	if err := Close(); err != nil {
		t.Errorf("Close nil db should return nil, got %v", err)
	}
}

func TestCloseRealReinit(t *testing.T) {
	if err := Close(); err != nil {
		t.Fatalf("Close real db: %v", err)
	}
	db = nil

	dir, err := os.MkdirTemp("", "logdb-reinit")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})
	if err := Init(dir); err != nil {
		t.Fatalf("reinit after close: %v", err)
	}
	if !Ready() {
		t.Fatal("Ready() should be true after reinit")
	}
}

func TestMigrateCoversAllTables(t *testing.T) {
	var count int
	for _, table := range AllTables {
		var n int
		if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", string(table)).Scan(&n); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if n != 1 {
			t.Errorf("table %s not present", table)
		}
		count++
	}
	if count != len(AllTables) {
		t.Errorf("checked %d tables, want %d", count, len(AllTables))
	}
}

func TestQueryScanRows(t *testing.T) {
	resetDB(t)
	rawInsert(t, "app_logs", "2024-01-01 00:00:00", "INFO", "C", "m", "app")
	rows, err := db.Query("SELECT id, timestamp, level, COALESCE(code,''), message, COALESCE(category,'') FROM app_logs")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var e LogEntry
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.Level, &e.Code, &e.Message, &e.Category); err != nil {
			t.Fatal(err)
		}
		n++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if n != 1 {
		t.Errorf("scanned %d rows, want 1", n)
	}
}

func TestQueryScanMatchesStruct(t *testing.T) {
	resetDB(t)
	Insert("app", "INFO", "", "x")
	entries, err := Query(TableApp, "", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries", len(entries))
	}
	var dummy *sql.DB
	_ = dummy
}
