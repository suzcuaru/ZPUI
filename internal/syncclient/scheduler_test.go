package syncclient

import (
	"encoding/json"

	"sync"
	"testing"
	"time"
)

// ---- тестовые помощники ----

type memFlags struct {
	mu sync.Mutex
	m  map[string]string
}

func newMemFlags() *memFlags { return &memFlags{m: map[string]string{}} }

func (f *memFlags) Get(key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.m[key], nil
}

func (f *memFlags) Set(key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.m[key] = value
	return nil
}

func (f *memFlags) val(key string) string {
	v, _ := f.Get(key)
	return v
}

type call struct {
	Op     string
	Token  string
	Ticket string
	Body   map[string]any
}

type fakeBackend struct {
	mu    sync.Mutex
	resp  map[string]func() (*Resp, error) // op → фабрика ответа
	calls []call
}

func (b *fakeBackend) Call(op string, dev Dev, token, ticket string, payload any) (*Resp, error) {
	raw, _ := json.Marshal(payload)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	b.mu.Lock()
	b.calls = append(b.calls, call{Op: op, Token: token, Ticket: ticket, Body: m})
	fn, ok := b.resp[op]
	b.mu.Unlock()
	if !ok {
		return &Resp{OK: true}, nil
	}
	return fn()
}

func (b *fakeBackend) count(op string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, c := range b.calls {
		if c.Op == op {
			n++
		}
	}
	return n
}

type fakeSources struct{}

func (fakeSources) Collect(op string) (map[string]any, error) {
	return map[string]any{"v": 1}, nil
}
func (fakeSources) ApplyPull(data json.RawMessage) {}
func (fakeSources) LastRowTs(op string) string     { return "2026-10-23T12:00:00Z" }

func testRunner(b *fakeBackend, flags *memFlags, now time.Time) *Runner {
	return &Runner{
		Backend:    b,
		Dev:        func() Dev { return Dev{ID: "dev-1"} },
		Token:      func() string { return "tok" },
		Flags:      flags,
		Sources:    fakeSources{},
		MaxPerTick: 20,
		Now:        func() time.Time { return now },
		Rand:       func() float64 { return 0.5 },
	}
}

// ---- джиттер ----

func TestJitterBounds(t *testing.T) {
	base := time.Minute
	// рандом 0 → base - 20%; 1 → base + 20%; 0.5 → base
	lo := Jitter(base, 0.2, func() float64 { return 0 })
	hi := Jitter(base, 0.2, func() float64 { return 1 })
	mid := Jitter(base, 0.2, func() float64 { return 0.5 })
	if lo != 48*time.Second {
		t.Fatalf("lo = %v, want 48s", lo)
	}
	if hi != 72*time.Second {
		t.Fatalf("hi = %v, want 72s", hi)
	}
	if mid != base {
		t.Fatalf("mid = %v, want %v", mid, base)
	}
	// вне [0,1] клампится
	clamp := Jitter(base, 0.2, func() float64 { return 5 })
	if clamp != hi {
		t.Fatalf("clamp = %v, want %v", clamp, hi)
	}
	// результат всегда >= 1s
	tiny := Jitter(time.Millisecond, 0.5, func() float64 { return 0 })
	if tiny < time.Second {
		t.Fatalf("tiny = %v, want >= 1s", tiny)
	}
}

// ---- планировщик: просрочка и старшинство ----

func TestPickDueOrderAndLimit(t *testing.T) {
	now := time.Now()
	lastOK := map[string]time.Time{
		"data.status":       now.Add(-3 * time.Minute),  // просрочка 3x
		"data.runtime":      now.Add(-11 * time.Minute), // просрочка ~2.2x
		"data.strategies":   now.Add(-30 * time.Minute), // просрочка 0.5x — не due
		"data.availability": now.Add(-90 * time.Minute), // просрочка 3x (tie со status)
	}
	due := PickDue(Catalog(), nil, lastOK, now, 2, nil)
	if len(due) != 2 {
		t.Fatalf("due = %d, want 2", len(due))
	}
	// Никогда не отправлявшиеся имеют абсолютный приоритет, затем по убыванию просрочки.
	if due[0].Op != "data.traffic" && due[0].Op != "data.system" && due[0].Op != "data.errors" &&
		due[0].Op != "data.events" && due[0].Op != "data.versions" && due[0].Op != "data.config" &&
		due[0].Op != "data.exclusions" && due[0].Op != "data.network" && due[0].Op != "data.operator" &&
		due[0].Op != "data.resources" {
		t.Fatalf("first due should be never-sent section, got %s", due[0].Op)
	}
}

func TestPickDueNeverSentFirst(t *testing.T) {
	now := time.Now()
	lastOK := map[string]time.Time{
		"data.status": now.Add(-3 * time.Minute), // просрочка 3x, но есть never-sent секции
	}
	due := PickDue(Catalog(), nil, lastOK, now, 1, nil)
	if len(due) != 1 {
		t.Fatalf("due = %d, want 1", len(due))
	}
	if due[0].Op == "data.status" {
		t.Fatalf("never-sent sections must outrank overdue ones, got %s", due[0].Op)
	}
}

func TestPickDueOverrides(t *testing.T) {
	now := time.Now()
	lastOK := map[string]time.Time{
		"data.status": now.Add(-3 * time.Minute), // просрочка 3x
	}
	ov := Overrides{
		"status":  Override{Mult: 5}, // интервал 5 мин — 3 мин < 5 мин, не due
		"runtime": Override{Off: true},
	}
	due := PickDue(Catalog(), ov, lastOK, now, 10, nil)
	for _, s := range due {
		if s.Op == "data.status" {
			t.Fatalf("status with mult=5 must not be due yet")
		}
		if s.Op == "data.runtime" {
			t.Fatalf("disabled section must not be due")
		}
	}
	// mult=2: 3 мин > 2 мин — due
	ov2 := Overrides{"status": Override{Mult: 2}}
	found := false
	for _, s := range PickDue(Catalog(), ov2, lastOK, now, 20, nil) {
		if s.Op == "data.status" {
			found = true
		}
	}
	if !found {
		t.Fatalf("status with mult=2 must be due (3 min elapsed > 2 min)")
	}
}

func TestEffectiveInterval(t *testing.T) {
	s := Section{Op: "data.status", Interval: time.Minute}
	if Overrides(nil).Effective(s) != time.Minute {
		t.Fatalf("nil overrides must keep interval")
	}
	ov2 := Overrides{"status": Override{Mult: 2}}
	if ov2.Effective(s) != 2*time.Minute {
		t.Fatalf("mult=2 must double interval")
	}
	// мусорный множитель → 1
	ov7 := Overrides{"status": Override{Mult: 7}}
	if ov7.Effective(s) != time.Minute {
		t.Fatalf("mult=7 must fall back to 1")
	}
}

// ---- тик: pull, секции, курсоры, талоны ----

func TestTickPullAndSections(t *testing.T) {
	now := time.Now()
	b := &fakeBackend{resp: map[string]func() (*Resp, error){
		OpPull: func() (*Resp, error) {
			return &Resp{OK: true, Data: json.RawMessage(`{"requests":{"now":["status"]}}`), NextAfter: 90}, nil
		},
	}}
	flags := newMemFlags()
	r := testRunner(b, flags, now)

	if !r.Tick(nil) {
		t.Fatalf("tick must send something")
	}
	// Первый тик: hello + pull + (requests → status) + просроченные секции...
	if b.count(OpHello) != 1 {
		t.Fatalf("hello = %d, want 1", b.count(OpHello))
	}
	if b.count(OpPull) != 1 {
		t.Fatalf("pull = %d, want 1", b.count(OpPull))
	}
	// просьба status исполнена немедленно и записан гейт
	if b.count("data.status") < 1 {
		t.Fatalf("requested status must be sent")
	}
	if flags.val("sync_reqexec_data_status") == "" {
		t.Fatalf("request gate not recorded")
	}
	// next_after от сервера учтён
	if flags.val(flagNextAfter) != "90" {
		t.Fatalf("next_after = %q, want 90", flags.val(flagNextAfter))
	}
	// online/RTT зафиксированы
	if !r.Status().Online {
		t.Fatalf("channel must be online after ok")
	}
}

func TestRequestGateHour(t *testing.T) {
	now := time.Now()
	b := &fakeBackend{resp: map[string]func() (*Resp, error){
		OpPull: func() (*Resp, error) {
			return &Resp{OK: true, Data: json.RawMessage(`{"requests":{"now":["status"]}}`)}, nil
		},
	}}
	flags := newMemFlags()
	// все секции «свежие» — в тике может уйти только просьба
	for _, sec := range Catalog() {
		_ = flags.Set("sync_last_"+SectionKey(sec.Op), now.Format(time.RFC3339))
	}
	_ = flags.Set("sync_reqexec_data_status", now.Add(-30*time.Minute).Format(time.RFC3339))
	r := testRunner(b, flags, now)
	r.Tick(nil)
	// 30 мин < час — просьба не исполняется повторно
	if n := b.count("data.status"); n != 0 {
		t.Fatalf("status = %d, want 0 (hour gate)", n)
	}
	// Через час после последнего исполнения просьба уходит.
	_ = flags.Set("sync_reqexec_data_status", now.Add(-2*time.Hour).Format(time.RFC3339))
	r2 := testRunner(b, flags, now.Add(10*time.Minute))
	r2.Tick(nil)
	if n := b.count("data.status"); n != 1 {
		t.Fatalf("status = %d, want 1 (gate expired)", n)
	}
}

func TestCursorShiftOnlyAfterAck(t *testing.T) {
	now := time.Now()
	var respData json.RawMessage
	b := &fakeBackend{resp: map[string]func() (*Resp, error){
		"data.availability": func() (*Resp, error) {
			return &Resp{OK: true, Data: respData}, nil
		},
	}}
	flags := newMemFlags()
	r := testRunner(b, flags, now)

	// Первый тик: availability в числе never-sent — уходит с пустым курсором.
	respData = json.RawMessage(`{}`) // ack нет
	r.Tick(nil)
	if n := b.count("data.availability"); n != 1 {
		t.Fatalf("availability = %d, want 1", n)
	}
	if got := flags.val("sync_cursor_data_availability"); got != "2026-10-23T12:00:00Z" {
		t.Fatalf("cursor fallback to last row ts = %q", got)
	}

	// Второй прогон: сервер вернул cursor_ack — курсор берётся из ack.
	respData = json.RawMessage(`{"ack":{"cursor_ack":"2026-10-23T13:00:00Z"}}`)
	r2 := testRunner(b, flags, now.Add(time.Hour))
	// форсируем просрочку
	_ = flags.Set("sync_last_data_availability", now.Format(time.RFC3339))
	r2.Tick(nil)
	if got := flags.val("sync_cursor_data_availability"); got != "2026-10-23T13:00:00Z" {
		t.Fatalf("cursor after ack = %q", got)
	}
}

func TestOnChangeHashGate(t *testing.T) {
	now := time.Now()
	b := &fakeBackend{}
	flags := newMemFlags()
	// exclusions «не менялась»: тот же хеш → повторной отправки нет.
	// Хеш раннер считает от payload БЕЗ collected_at; Collect фейка отдаёт {"v":1}.
	_ = flags.Set("sync_last_data_exclusions", now.Add(-25*time.Hour).Format(time.RFC3339))
	_ = flags.Set("sync_hash_data_exclusions", HashJSON(map[string]any{"v": 1}))
	r := testRunner(b, flags, now)
	r.Tick(nil)
	if n := b.count("data.exclusions"); n != 0 {
		t.Fatalf("exclusions = %d, want 0 (hash unchanged)", n)
	}
	// last_ok обновлён без отправки
	if flags.val("sync_last_data_exclusions") == "" {
		t.Fatalf("last_ok must be refreshed on unchanged hash")
	}
}

func TestOnChangeSendsWhenHashChanged(t *testing.T) {
	now := time.Now()
	b := &fakeBackend{}
	flags := newMemFlags()
	_ = flags.Set("sync_last_data_exclusions", now.Add(-25*time.Hour).Format(time.RFC3339))
	_ = flags.Set("sync_hash_data_exclusions", "old-hash")
	r := testRunner(b, flags, now)
	r.Tick(nil)
	if n := b.count("data.exclusions"); n != 0 {
		t.Fatalf("exclusions sent on first tick, want deferred by debounce")
	}
	// дебаунс истёк — список уезжает целиком
	_ = flags.Set("sync_excl_dirty", now.Add(-time.Minute).Format(time.RFC3339))
	r.Tick(nil)
	if n := b.count("data.exclusions"); n != 1 {
		t.Fatalf("exclusions = %d, want 1 (debounce истёк, hash changed)", n)
	}
	if got := flags.val("sync_hash_data_exclusions"); got == "old-hash" {
		t.Fatalf("hash must be updated after send")
	}
}

func TestBusyTicketFlow(t *testing.T) {
	now := time.Now()
	busy := true
	var gotTicket string
	b := &fakeBackend{resp: map[string]func() (*Resp, error){
		"data.status": func() (*Resp, error) {
			if busy {
				return &Resp{OK: false}, &Error{Code: "busy", Ticket: "t_1", RetryAfter: 45}
			}
			return &Resp{OK: true}, nil
		},
	}}
	flags := newMemFlags()
	r := testRunner(b, flags, now)

	// Тик 1: status просрочен (never-sent) → busy, талон сохранён.
	r.Tick(nil)
	if n := b.count("data.status"); n == 0 {
		t.Fatalf("status must have been attempted")
	}
	st := r.Status()
	if st.TicketOp != "data.status" {
		t.Fatalf("ticket op = %q, want data.status", st.TicketOp)
	}
	if flags.val(flagTicket) == "" {
		t.Fatalf("ticket must be persisted to flags")
	}

	// Тик 2: повтор идёт с талоном; busy сохраняет талон (молчание).
	busy = true
	r.Tick(nil)
	b.mu.Lock()
	var last call
	for _, c := range b.calls {
		if c.Op == "data.status" {
			last = c
		}
	}
	b.mu.Unlock()
	if last.Ticket != "t_1" {
		t.Fatalf("retry ticket = %q, want t_1", last.Ticket)
	}

	// Тик 3: сервер принял — талон снят.
	busy = false
	r.Tick(nil)
	if st := r.Status(); st.TicketOp != "" {
		t.Fatalf("ticket must be cleared after ok, got %q", st.TicketOp)
	}
	// После снятия талона очередь двигается дальше без талона.
	_ = gotTicket
}

func TestBannedClearsTokenViaHook(t *testing.T) {
	now := time.Now()
	b := &fakeBackend{resp: map[string]func() (*Resp, error){
		OpHello: func() (*Resp, error) {
			return &Resp{OK: false}, &Error{Code: "banned"}
		},
	}}
	flags := newMemFlags()
	r := testRunner(b, flags, now)
	banned := false
	r.OnBanned = func() { banned = true }
	r.Tick(nil)
	if !banned {
		t.Fatalf("OnBanned hook must fire")
	}
}

func TestSkipTickPowerSaving(t *testing.T) {
	now := time.Now()
	b := &fakeBackend{}
	flags := newMemFlags()
	r := testRunner(b, flags, now)
	r.SkipTick = func() bool { return true }
	if r.Tick(nil) {
		t.Fatalf("level-3 power saving must skip the whole tick")
	}
	if b.count(OpHello) != 0 {
		t.Fatalf("no calls expected, got hello=%d", b.count(OpHello))
	}
}

func TestNoTokenAnonymousChannel(t *testing.T) {
	// Политика 11.2026: канал данных работает и без аккаунта — hello, pull
	// и секции уходят анонимно, по паспорту dev (02 §6).
	now := time.Now()
	b := &fakeBackend{}
	flags := newMemFlags()
	r := testRunner(b, flags, now)
	r.Token = func() string { return "" }
	r.Tick(nil)
	if b.count(OpHello) == 0 {
		t.Fatalf("hello must be sent without token")
	}
	if b.count(OpPull) == 0 {
		t.Fatalf("pull must be sent without token (anonymous channel)")
	}
	if b.count(OpDataStatus) == 0 {
		t.Fatalf("data sections must be sent without token (anonymous channel)")
	}
}

func TestNextAfterClamp(t *testing.T) {
	now := time.Now()
	b := &fakeBackend{resp: map[string]func() (*Resp, error){
		OpPull: func() (*Resp, error) {
			return &Resp{OK: true, NextAfter: 3600}, nil
		},
	}}
	flags := newMemFlags()
	r := testRunner(b, flags, now)
	r.Tick(nil)
	wait := r.nextWait()
	if wait > 12*time.Minute {
		t.Fatalf("wait = %v, must be clamped to 10min + jitter (<=12min)", wait)
	}
}

func TestStatusSnapshotRoundTrip(t *testing.T) {
	// журнал просьб переживает пересоздание раннера (флаги)
	now := time.Now()
	b := &fakeBackend{}
	flags := newMemFlags()
	r := testRunner(b, flags, now)
	r.AddRequestsLog(map[string]any{"at": now.Format(time.RFC3339), "sections": []string{"exclusions"}})

	r2 := testRunner(b, flags, now)
	r2.loadRequestsLog()
	if len(r2.Status().RequestsLog) != 1 {
		t.Fatalf("requests log lost across runner restarts")
	}
	// кольцо на 20 записей
	for i := 0; i < 30; i++ {
		r.AddRequestsLog(map[string]any{"at": now.Add(time.Duration(i) * time.Second).Format(time.RFC3339)})
	}
	if len(r.Status().RequestsLog) != 20 {
		t.Fatalf("requests log = %d, want 20 (ring)", len(r.Status().RequestsLog))
	}
}

func TestRequestsLogTTL(t *testing.T) {
	now := time.Now()
	b := &fakeBackend{}
	r := testRunner(b, newMemFlags(), now)
	r.AddRequestsLog(map[string]any{"at": now.Add(-2 * time.Hour).Format(time.RFC3339)})
	r.AddRequestsLog(map[string]any{"at": now.Format(time.RFC3339)})
	if len(r.Status().RequestsLog) != 1 {
		t.Fatalf("requests log = %d, want 1 (entries older than 1h dropped)", len(r.Status().RequestsLog))
	}
}
