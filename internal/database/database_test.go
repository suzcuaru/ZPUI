package database

import (
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

var seq int64

func uniq(prefix string) string {
	n := atomic.AddInt64(&seq, 1)
	return prefix + "-" + strconv.FormatInt(n, 10)
}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "zpui-dbtest-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	dbPath := filepath.Join(dir, "test.db")
	if err := Init(dbPath); err != nil {
		panic(err)
	}

	// Production migrate() declares ON CONFLICT(mac) in UpsertDevice but only
	// creates a non-unique index on mac — add the missing unique constraint so
	// UpsertDevice's upsert path is exercisable. (Production bug, flagged.)
	if _, err := DB().Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uq_session_devices_mac ON session_devices(mac)`); err != nil {
		panic(err)
	}

	code := m.Run()

	if err := Close(); err != nil {
		panic(err)
	}
	os.Exit(code)
}

func TestInitIdempotentAndDB(t *testing.T) {
	if DB() == nil {
		t.Fatal("DB() is nil after Init")
	}
	if err := Init(dbPathInTest); err != nil {
		t.Errorf("second Init call should be no-op, got: %v", err)
	}
	if DB() == nil {
		t.Fatal("DB() is nil after second Init")
	}
}

const dbPathInTest = "ignored-by-once"

func countRows(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := DB().QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestEmptyCurrentOperatorEarly(t *testing.T) {
	o, err := GetCurrentOperator()
	if err != nil {
		t.Fatalf("GetCurrentOperator: %v", err)
	}
	if o != nil {
		t.Skipf("current_operator already populated: %+v", o)
	}
	if GetZapretJustUpdated() {
		t.Error("GetZapretJustUpdated should be false when no row exists")
	}
}

func TestSessionDevices(t *testing.T) {
	now := time.Now()
	mac := uniq("mac")

	d := &SessionDevice{
		ID: "preset-id", MAC: mac, IP: "10.0.0.1", Hostname: "dev1",
		FirstSeen: now, LastSeen: now, TotalDL: 100, TotalUL: 50, IsOnline: true,
	}
	if err := UpsertDevice(d); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}
	if d.ID != "preset-id" {
		t.Errorf("preset ID overwritten: %q", d.ID)
	}

	upd := &SessionDevice{
		MAC: mac, IP: "10.0.0.2", Hostname: "dev1-up",
		FirstSeen: now, LastSeen: now.Add(time.Hour), TotalDL: 200, TotalUL: 100, IsOnline: true,
	}
	if err := UpsertDevice(upd); err != nil {
		t.Fatalf("UpsertDevice update: %v", err)
	}

	got, err := GetDeviceByMAC(mac)
	if err != nil {
		t.Fatalf("GetDeviceByMAC: %v", err)
	}
	if got == nil {
		t.Fatal("expected device, got nil")
	}
	if got.IP != "10.0.0.2" {
		t.Errorf("IP = %s, want 10.0.0.2", got.IP)
	}
	if got.Hostname != "dev1-up" {
		t.Errorf("Hostname = %s, want dev1-up", got.Hostname)
	}
	if got.TotalDL != 200 || got.TotalUL != 100 {
		t.Errorf("totals = %d/%d, want 200/100", got.TotalDL, got.TotalUL)
	}
	if !got.IsOnline {
		t.Error("expected online")
	}

	missing, err := GetDeviceByMAC(uniq("nope"))
	if err != nil {
		t.Fatalf("GetDeviceByMAC missing: %v", err)
	}
	if missing != nil {
		t.Errorf("expected nil for missing mac, got %+v", missing)
	}

	all, err := GetAllDevices()
	if err != nil {
		t.Fatalf("GetAllDevices: %v", err)
	}
	found := false
	for i := range all {
		if all[i].MAC == mac {
			found = true
		}
	}
	if !found {
		t.Error("device not present in GetAllDevices")
	}

	if err := SetAllDevicesOffline(); err != nil {
		t.Fatalf("SetAllDevicesOffline: %v", err)
	}
	if g2, _ := GetDeviceByMAC(mac); g2 == nil || g2.IsOnline {
		t.Error("expected offline after SetAllDevicesOffline")
	}

	if err := DeleteDevice(mac); err != nil {
		t.Fatalf("DeleteDevice: %v", err)
	}
	if g3, _ := GetDeviceByMAC(mac); g3 != nil {
		t.Error("expected nil after DeleteDevice")
	}
}

func TestClearDevices(t *testing.T) {
	if err := UpsertDevice(&SessionDevice{MAC: uniq("mac"), IP: "1.2.3.4", LastSeen: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := ClearDevices(); err != nil {
		t.Fatalf("ClearDevices: %v", err)
	}
	if n := countRows(t, "session_devices"); n != 0 {
		t.Errorf("expected 0 devices after clear, got %d", n)
	}
}

func TestDeviceConnections(t *testing.T) {
	mac := uniq("mac")
	dev := &SessionDevice{MAC: mac, IP: "10.0.0.9", LastSeen: time.Now()}
	if err := UpsertDevice(dev); err != nil {
		t.Fatal(err)
	}
	devID := dev.ID

	now := time.Now()
	c1 := &DeviceConnection{DeviceID: devID, DstHost: "a.com", DstPort: 443, BytesDL: 10, BytesUL: 5, StartedAt: now, ClosedAt: nil}
	if err := InsertConnection(c1); err != nil {
		t.Fatalf("InsertConnection: %v", err)
	}
	if c1.ID == "" {
		t.Error("ID not assigned")
	}

	later := now.Add(time.Minute)
	c2 := &DeviceConnection{DeviceID: devID, DstHost: "b.com", DstPort: 80, BytesDL: 20, BytesUL: 0, StartedAt: later}
	c2.ClosedAt = &later
	if err := InsertConnection(c2); err != nil {
		t.Fatalf("InsertConnection 2: %v", err)
	}

	byID, err := GetDeviceConnections(devID, 10, 0)
	if err != nil {
		t.Fatalf("GetDeviceConnections by id: %v", err)
	}
	if len(byID) != 2 {
		t.Errorf("by id: got %d conns, want 2", len(byID))
	}
	if byID[0].StartedAt.Before(byID[1].StartedAt) {
		t.Error("expected DESC order by started_at")
	}
	if byID[0].DstHost != "b.com" {
		t.Errorf("first = %s, want b.com", byID[0].DstHost)
	}

	byMAC, err := GetDeviceConnections(mac, 0, 0) // limit<=0 -> 50
	if err != nil {
		t.Fatalf("GetDeviceConnections by mac: %v", err)
	}
	if len(byMAC) != 2 {
		t.Errorf("by mac: got %d conns, want 2", len(byMAC))
	}

	empty, err := GetDeviceConnections(uniq("unknown"), 10, 0)
	if err != nil {
		t.Fatalf("GetDeviceConnections unknown: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("unknown device: got %d, want 0", len(empty))
	}
}

func TestCleanOldConnections(t *testing.T) {
	old := time.Now().Add(-2 * time.Hour)
	c := &DeviceConnection{DeviceID: "ghost", DstHost: "h", DstPort: 1, BytesDL: 1, StartedAt: old, ClosedAt: &old}
	if err := InsertConnection(c); err != nil {
		t.Fatal(err)
	}
	before := countRows(t, "device_connections")

	if err := CleanOldConnections(time.Hour); err != nil {
		t.Fatalf("CleanOldConnections: %v", err)
	}
	after := countRows(t, "device_connections")
	if after != before-1 {
		t.Errorf("expected %d rows after clean, got %d", before-1, after)
	}
}

func TestSnapshots(t *testing.T) {
	old := time.Now().Add(-2 * time.Hour)
	recent := time.Now().Add(-5 * time.Minute)

	if err := InsertSnapshot(&TrafficSnapshot{Timestamp: old, DLSpeed: 100, ULSpeed: 10, TotalDL: 1, TotalUL: 1, ConnCount: 5}); err != nil {
		t.Fatal(err)
	}
	if err := InsertSnapshot(&TrafficSnapshot{Timestamp: recent, DLSpeed: 200, ULSpeed: 20, TotalDL: 2, TotalUL: 2, ConnCount: 8}); err != nil {
		t.Fatal(err)
	}

	snaps, err := GetSnapshots(time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("GetSnapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Errorf("expected 1 recent snapshot, got %d", len(snaps))
	} else if snaps[0].DLSpeed != 200 {
		t.Errorf("DLSpeed = %v, want 200", snaps[0].DLSpeed)
	}

	if err := CleanOldSnapshots(time.Hour); err != nil {
		t.Fatalf("CleanOldSnapshots: %v", err)
	}
	all, _ := GetSnapshots(time.Now().Add(-24 * time.Hour))
	if len(all) != 1 {
		t.Errorf("expected 1 snapshot after clean, got %d", len(all))
	}
}

func TestActionLogs(t *testing.T) {
	cat := uniq("cat")
	if err := InsertActionLog(&ActionLog{Category: cat, Action: "a1", Details: "d1"}); err != nil {
		t.Fatal(err)
	}
	if err := InsertActionLog(&ActionLog{Category: cat, Action: "a2"}); err != nil {
		t.Fatal(err)
	}
	if err := InsertActionLog(&ActionLog{Category: "other-cat", Action: "a3", Details: "x"}); err != nil {
		t.Fatal(err)
	}

	byCat, err := GetActionLogs(cat, 0, 0) // limit<=0 -> 100
	if err != nil {
		t.Fatalf("GetActionLogs by cat: %v", err)
	}
	if len(byCat) != 2 {
		t.Errorf("by cat: got %d, want 2", len(byCat))
	}
	for _, l := range byCat {
		if l.Category != cat {
			t.Errorf("category = %s, want %s", l.Category, cat)
		}
	}

	all, err := GetActionLogs("", 10, 0)
	if err != nil {
		t.Fatalf("GetActionLogs all: %v", err)
	}
	if len(all) < 3 {
		t.Errorf("all: got %d, want >=3", len(all))
	}
	if all[0].Timestamp.Before(all[len(all)-1].Timestamp) {
		t.Error("expected DESC order")
	}

	if err := BatchInsertActionLogs(nil); err != nil {
		t.Errorf("BatchInsertActionLogs(nil): %v", err)
	}
	if err := BatchInsertActionLogs([]ActionLog{}); err != nil {
		t.Errorf("BatchInsertActionLogs(empty): %v", err)
	}

	batch := []ActionLog{
		{Category: cat, Action: "b1"},
		{Category: cat, Action: "b2", Details: "bd"},
	}
	if err := BatchInsertActionLogs(batch); err != nil {
		t.Fatalf("BatchInsertActionLogs: %v", err)
	}
	got, _ := GetActionLogs(cat, 100, 0)
	if len(got) != 4 {
		t.Errorf("after batch: got %d, want 4", len(got))
	}
}

func TestAvailabilityHistory(t *testing.T) {
	old := time.Now().Add(-2 * time.Hour)
	recent := time.Now().Add(-5 * time.Minute)

	if err := InsertAvailabilitySnapshot(&AvailabilityRecord{Timestamp: old, Type: "all", TotalResources: 10, OKResources: 5, Pct: 50}); err != nil {
		t.Fatal(err)
	}
	if err := InsertAvailabilitySnapshot(&AvailabilityRecord{Timestamp: recent, Type: "all", TotalResources: 10, OKResources: 8, Pct: 80}); err != nil {
		t.Fatal(err)
	}

	recs, err := GetAvailabilityHistory(time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("GetAvailabilityHistory: %v", err)
	}
	if len(recs) != 1 {
		t.Errorf("expected 1 recent record, got %d", len(recs))
	} else if recs[0].Pct != 80 {
		t.Errorf("Pct = %v, want 80", recs[0].Pct)
	}

	if err := CleanOldAvailability(time.Hour); err != nil {
		t.Fatalf("CleanOldAvailability: %v", err)
	}
	if n := countRows(t, "availability_history"); n != 1 {
		t.Errorf("expected 1 record after clean, got %d", n)
	}
}

func TestResourceAvailability(t *testing.T) {
	host := uniq("host")
	other := uniq("host")
	now := time.Now()

	if err := InsertResourceAvailability(&ResourceAvailability{Timestamp: now, OperatorKey: "op", Host: host, Type: "standard", Ok: true, Verdict: "ok", LatencyMs: 10}); err != nil {
		t.Fatal(err)
	}
	if err := InsertResourceAvailability(&ResourceAvailability{Timestamp: now, OperatorKey: "op", Host: other, Ok: false, Verdict: "blocked", LatencyMs: 0}); err != nil {
		t.Fatal(err)
	}

	byHost, err := GetResourceAvailabilityHistory(now.Add(-time.Hour), host)
	if err != nil {
		t.Fatalf("GetResourceAvailabilityHistory by host: %v", err)
	}
	if len(byHost) != 1 {
		t.Errorf("by host: got %d, want 1", len(byHost))
	} else {
		if !byHost[0].Ok {
			t.Error("expected Ok=true")
		}
		if byHost[0].Verdict != "ok" {
			t.Errorf("Verdict = %s, want ok", byHost[0].Verdict)
		}
	}

	all, err := GetResourceAvailabilityHistory(now.Add(-time.Hour), "")
	if err != nil {
		t.Fatalf("GetResourceAvailabilityHistory all: %v", err)
	}
	if len(all) < 2 {
		t.Errorf("all: got %d, want >=2", len(all))
	}

	today := now.Format("2006-01-02")
	if err := RollupResourceDaily(today); err != nil {
		t.Fatalf("RollupResourceDaily: %v", err)
	}
	daily, err := GetResourceDailyHistory(host, 1)
	if err != nil {
		t.Fatalf("GetResourceDailyHistory: %v", err)
	}
	found := false
	for _, d := range daily {
		if d.Host == host {
			found = true
			if d.ChecksTotal != 1 || d.ChecksOK != 1 {
				t.Errorf("daily totals = %d/%d, want 1/1", d.ChecksTotal, d.ChecksOK)
			}
			if d.Pct != 100 {
				t.Errorf("Pct = %v, want 100", d.Pct)
			}
		}
	}
	if !found {
		t.Error("host not found in daily history")
	}

	if err := RollupResourceDaily("not-a-date"); err == nil {
		t.Error("expected error for invalid date")
	}

	allDaily, err := GetResourceDailyHistory("", 1)
	if err != nil {
		t.Fatalf("GetResourceDailyHistory all: %v", err)
	}
	if len(allDaily) < 1 {
		t.Errorf("all daily: got %d, want >=1", len(allDaily))
	}

	if err := CleanOldResourceAvailability(time.Nanosecond); err != nil {
		t.Fatalf("CleanOldResourceAvailability: %v", err)
	}
	if n := countRows(t, "resource_availability"); n != 0 {
		t.Errorf("expected 0 raw rows after clean, got %d", n)
	}
}

func TestZapretBackup(t *testing.T) {
	if err := DeleteZapretBackup(); err != nil {
		t.Fatal(err)
	}
	d, err := GetZapretBackup()
	if err != nil {
		t.Fatalf("GetZapretBackup empty: %v", err)
	}
	if d != "" {
		t.Errorf("expected empty, got %q", d)
	}

	if err := SaveZapretBackup("data1"); err != nil {
		t.Fatal(err)
	}
	d, _ = GetZapretBackup()
	if d != "data1" {
		t.Errorf("got %q, want data1", d)
	}

	if err := SaveZapretBackup("data2"); err != nil {
		t.Fatal(err)
	}
	d, _ = GetZapretBackup()
	if d != "data2" {
		t.Errorf("got %q, want data2", d)
	}

	if err := DeleteZapretBackup(); err != nil {
		t.Fatal(err)
	}
	d, _ = GetZapretBackup()
	if d != "" {
		t.Errorf("expected empty after delete, got %q", d)
	}
}

func TestComponentVersions(t *testing.T) {
	id := uniq("comp")

	v, err := GetComponentVersion(id)
	if err != nil {
		t.Fatalf("GetComponentVersion missing: %v", err)
	}
	if v != nil {
		t.Errorf("expected nil for missing, got %+v", v)
	}

	rt := time.Now().Add(-time.Hour)
	if err := SaveComponentVersion(&ComponentVersion{ID: id, InstalledVersion: "1.0", RemoteVersion: "2.0", RemoteSource: "gh", RemoteUpdatedAt: &rt}); err != nil {
		t.Fatal(err)
	}
	v, _ = GetComponentVersion(id)
	if v == nil {
		t.Fatal("expected version after save")
	}
	if v.InstalledVersion != "1.0" || v.RemoteVersion != "2.0" || v.RemoteSource != "gh" {
		t.Errorf("unexpected version: %+v", v)
	}
	if v.RemoteUpdatedAt == nil || !v.RemoteUpdatedAt.Equal(rt) {
		t.Errorf("RemoteUpdatedAt = %v, want %v", v.RemoteUpdatedAt, rt)
	}
	if v.LocalUpdatedAt.IsZero() {
		t.Error("LocalUpdatedAt should be set")
	}

	// Re-save: installed updates; nil *time.Time -> NULL -> COALESCE keeps prior remote_updated_at.
	if err := SaveComponentVersion(&ComponentVersion{ID: id, InstalledVersion: "1.1"}); err != nil {
		t.Fatal(err)
	}
	v, _ = GetComponentVersion(id)
	if v.InstalledVersion != "1.1" {
		t.Errorf("InstalledVersion = %s, want 1.1", v.InstalledVersion)
	}
	if v.RemoteUpdatedAt == nil || !v.RemoteUpdatedAt.Equal(rt) {
		t.Errorf("COALESCE: RemoteUpdatedAt not preserved: %v", v.RemoteUpdatedAt)
	}

	all, err := GetAllComponentVersions()
	if err != nil {
		t.Fatalf("GetAllComponentVersions: %v", err)
	}
	found := false
	for i := range all {
		if all[i].ID == id {
			found = true
		}
	}
	if !found {
		t.Error("component not in GetAllComponentVersions")
	}
}

func TestOperatorInfo(t *testing.T) {
	key := uniq("op")

	o, err := GetOperatorInfo(key)
	if err != nil {
		t.Fatalf("GetOperatorInfo missing: %v", err)
	}
	if o != nil {
		t.Errorf("expected nil for missing, got %+v", o)
	}

	if err := UpsertOperatorInfo(&OperatorInfo{Key: key, Name: "Op1", ISP: "isp1", ASN: "1", City: "c", Org: "org1"}); err != nil {
		t.Fatal(err)
	}
	o, _ = GetOperatorInfo(key)
	if o == nil || o.Name != "Op1" || o.ISP != "isp1" || o.Org != "org1" {
		t.Errorf("unexpected operator: %+v", o)
	}

	if err := UpsertOperatorInfo(&OperatorInfo{Key: key, Name: "Op2", ISP: "isp2"}); err != nil {
		t.Fatal(err)
	}
	o, _ = GetOperatorInfo(key)
	if o.Name != "Op2" || o.ISP != "isp2" {
		t.Errorf("upsert not applied: %+v", o)
	}

	all, err := GetAllOperatorInfo()
	if err != nil {
		t.Fatalf("GetAllOperatorInfo: %v", err)
	}
	found := false
	for i := range all {
		if all[i].Key == key {
			found = true
		}
	}
	if !found {
		t.Error("operator not in GetAllOperatorInfo")
	}
}

func TestSaveOperatorStrategyCOALESCE(t *testing.T) {
	key := uniq("op")
	strat := uniq("strat")

	if err := SaveOperatorStrategy(key, strat, `{"avail":90}`); err != nil {
		t.Fatal(err)
	}
	got, err := GetOperatorStrategy(key)
	if err != nil {
		t.Fatalf("GetOperatorStrategy: %v", err)
	}
	if got != strat {
		t.Errorf("strategy = %q, want %q", got, strat)
	}
	res, err := GetOperatorTestResults(key)
	if err != nil {
		t.Fatalf("GetOperatorTestResults: %v", err)
	}
	if res != `{"avail":90}` {
		t.Errorf("test results = %q, want {\"avail\":90}", res)
	}

	if err := SaveOperatorStrategy(key, strat, ""); err != nil {
		t.Fatal(err)
	}
	res2, _ := GetOperatorTestResults(key)
	if res2 != `{"avail":90}` {
		t.Errorf("COALESCE bug: empty testResults overwrote existing; got %q", res2)
	}

	strat2 := uniq("strat")
	if err := SaveOperatorStrategy(key, strat2, ""); err != nil {
		t.Fatal(err)
	}
	res3, _ := GetOperatorTestResults(key)
	if res3 != `{"avail":90}` {
		t.Errorf("expected original test results preserved, got %q", res3)
	}

	nf, err := GetOperatorStrategy(uniq("none"))
	if err != nil || nf != "" {
		t.Errorf("GetOperatorStrategy missing: %q err=%v", nf, err)
	}
	resNF, err := GetOperatorTestResults(uniq("none"))
	if err != nil || resNF != "" {
		t.Errorf("GetOperatorTestResults missing: %q err=%v", resNF, err)
	}
}

func TestEnsureOperatorStrategy(t *testing.T) {
	key := uniq("op")
	strat := uniq("strat")

	if err := EnsureOperatorStrategy(key, strat, "Display"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureOperatorStrategy(key, strat, ""); err != nil {
		t.Fatal(err)
	}

	list, err := GetOperatorStrategies(key)
	if err != nil {
		t.Fatalf("GetOperatorStrategies: %v", err)
	}
	var target *OperatorStrategy
	for i := range list {
		if list[i].Strategy == strat {
			target = &list[i]
		}
	}
	if target == nil {
		t.Fatal("strategy not found")
	}
	if target.DisplayName != "Display" {
		t.Errorf("DisplayName = %q, want Display (COALESCE should keep)", target.DisplayName)
	}
	if target.AvailabilityPct != -1 {
		t.Errorf("default AvailabilityPct = %v, want -1", target.AvailabilityPct)
	}
}

func TestMarkActiveStrategy(t *testing.T) {
	key := uniq("op")
	s1 := uniq("s")
	s2 := uniq("s")
	if err := EnsureOperatorStrategy(key, s1, "S1"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureOperatorStrategy(key, s2, "S2"); err != nil {
		t.Fatal(err)
	}

	if err := UpdateStrategyAvailability(key, s1, 75.5); err != nil {
		t.Fatal(err)
	}
	if err := MarkActiveStrategy(key, s1, "auto"); err != nil {
		t.Fatal(err)
	}

	list, _ := GetOperatorStrategies(key)
	find := func(s string) *OperatorStrategy {
		for i := range list {
			if list[i].Strategy == s {
				return &list[i]
			}
		}
		return nil
	}
	a1 := find(s1)
	a2 := find(s2)
	if a1 == nil || a2 == nil {
		t.Fatal("strategies missing")
	}
	if !a1.IsActive || a2.IsActive {
		t.Errorf("active flags wrong: s1=%v s2=%v", a1.IsActive, a2.IsActive)
	}
	if a1.UseCount != 1 {
		t.Errorf("UseCount = %d, want 1", a1.UseCount)
	}
	if a1.LastSource != "auto" {
		t.Errorf("LastSource = %q, want auto", a1.LastSource)
	}
	if a1.AvailabilityPct != 75.5 {
		t.Errorf("AvailabilityPct = %v, want 75.5", a1.AvailabilityPct)
	}

	if err := MarkActiveStrategy(key, s2, "manual"); err != nil {
		t.Fatal(err)
	}
	list2, _ := GetOperatorStrategies(key)
	for i := range list2 {
		if list2[i].Strategy == s1 && list2[i].IsActive {
			t.Error("s1 should be inactive after marking s2")
		}
		if list2[i].Strategy == s2 && !list2[i].IsActive {
			t.Error("s2 should be active")
		}
	}
}

func TestCurrentOperatorAndZapretFlag(t *testing.T) {
	key := uniq("op")
	if err := SetCurrentOperator(key, "OpName", "strategyX"); err != nil {
		t.Fatal(err)
	}
	o, err := GetCurrentOperator()
	if err != nil {
		t.Fatalf("GetCurrentOperator: %v", err)
	}
	if o == nil {
		t.Fatal("expected current operator")
	}
	if o.OperatorKey != key || o.OperatorName != "OpName" || o.Strategy != "strategyX" {
		t.Errorf("unexpected: %+v", o)
	}
	if o.DetectedAt == nil {
		t.Error("DetectedAt should be set")
	}

	if err := SetZapretJustUpdated(true); err != nil {
		t.Fatal(err)
	}
	if !GetZapretJustUpdated() {
		t.Error("expected zapret just updated = true")
	}
	if err := SetZapretJustUpdated(false); err != nil {
		t.Fatal(err)
	}
	if GetZapretJustUpdated() {
		t.Error("expected zapret just updated = false")
	}

	o2, _ := GetCurrentOperator()
	if o2 == nil || o2.OperatorKey != key {
		t.Errorf("operator key changed by zapret flag: %+v", o2)
	}
}

func TestCloseNilIsSafe(t *testing.T) {
	saved := db
	db = nil
	if err := Close(); err != nil {
		t.Errorf("Close with nil db: %v", err)
	}
	db = saved
}
