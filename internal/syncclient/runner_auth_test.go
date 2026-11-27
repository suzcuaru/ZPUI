package syncclient

// Бэкофф после auth_required (гашение 401-шторма): тишина до смены токена,
// одна пробная попытка (hello+pull) раз в authProbeInterval, снятие блока
// по смене токена или успешной пробе.

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

type warnLog struct {
	mu    sync.Mutex
	warns []string
}

func (l *warnLog) Info(source, msg string) {}
func (l *warnLog) Warn(source, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, msg)
}

func (l *warnLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.warns)
}

// authBackend — конвертные ошибки по конфигурации: pull/секции отвечают
// auth_required только для «протухшего» токена (failToken).
type authBackend struct {
	fakeBackend
	failToken string
}

func (b *authBackend) Call(op string, dev Dev, token, ticket string, payload any) (*Resp, error) {
	if token == b.failToken && (op == OpPull || strings.HasPrefix(op, "data.")) {
		raw, _ := json.Marshal(payload)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		b.mu.Lock()
		b.calls = append(b.calls, call{Op: op, Token: token, Ticket: ticket, Body: m})
		b.mu.Unlock()
		return &Resp{OK: false}, &Error{Code: "auth_required"}
	}
	return b.fakeBackend.Call(op, dev, token, ticket, payload)
}

func newAuthBackend(stale string) *authBackend {
	return &authBackend{fakeBackend{resp: map[string]func() (*Resp, error){}}, stale}
}

func runnerAt(b Backend, flags *memFlags, clock *time.Time) *Runner {
	return &Runner{
		Backend:    b,
		Dev:        func() Dev { return Dev{ID: "dev-1"} },
		Flags:      flags,
		Sources:    fakeSources{},
		MaxPerTick: 20,
		Now:        func() time.Time { return *clock },
		Rand:       func() float64 { return 0.5 },
	}
}

func TestAuthBackoffStops401Storm(t *testing.T) {
	t0 := time.Now()
	clock := t0
	b := newAuthBackend("tok")
	flags := newMemFlags()
	r := runnerAt(b, flags, &clock)
	tok := "tok"
	r.Token = func() string { return tok }

	// Тик 1: hello + pull + просроченные секции — все data.* с 401.
	r.Tick(nil)
	if st := r.Status(); st.AuthState != "relogin" {
		t.Fatalf("auth_state = %q, want relogin", st.AuthState)
	}
	pulls, hellos := b.count(OpPull), b.count(OpHello)
	sections := b.count(OpDataStatus) + b.count("data.runtime")
	if pulls != 1 || hellos != 1 || sections == 0 {
		t.Fatalf("tick1: hello=%d pull=%d sections=%d", hellos, pulls, sections)
	}
	statusAfterTick1 := b.count(OpDataStatus)

	// Тик 2 (+60с): бэкофф — полная тишина, никаких запросов.
	clock = t0.Add(time.Minute)
	if r.Tick(nil) {
		t.Fatalf("backoff tick must be silent")
	}
	if b.count(OpPull) != pulls || b.count(OpHello) != hellos || b.count(OpDataStatus) != statusAfterTick1 {
		t.Fatalf("backoff tick must not send anything")
	}

	// Тик 3 (+31 мин): одна пробная попытка — hello + pull, секций нет.
	clock = t0.Add(31 * time.Minute)
	if r.Tick(nil) {
		t.Fatalf("failed probe tick must report nothing sent")
	}
	if got := b.count(OpPull); got != pulls+1 {
		t.Fatalf("probe pull = %d, want %d", got, pulls+1)
	}
	if got := b.count(OpHello); got != hellos+1 {
		t.Fatalf("probe hello = %d, want %d", got, hellos+1)
	}
	if got := b.count(OpDataStatus); got != statusAfterTick1 {
		t.Fatalf("probe must not send sections: %d -> %d", statusAfterTick1, got)
	}

	// Тик 4 (+32 мин): снова тишина до следующего окна пробы.
	clock = t0.Add(32 * time.Minute)
	r.Tick(nil)
	if got := b.count(OpPull); got != pulls+1 {
		t.Fatalf("silent tick sent pull: %d", got)
	}
}

func TestAuthBackoffLiftOnTokenChange(t *testing.T) {
	t0 := time.Now()
	clock := t0
	b := newAuthBackend("tok")
	flags := newMemFlags()
	r := runnerAt(b, flags, &clock)
	tok := "tok"
	r.Token = func() string { return tok }
	r.Tick(nil) // 401-шторм, бэкофф включён с "tok"

	// Пользователь вошёл заново — токен сменился: бэкофф снимается немедленно.
	tok = "fresh"
	clock = t0.Add(time.Minute)
	if !r.Tick(nil) {
		t.Fatalf("tick after re-login must send")
	}
	if got := b.count(OpPull); got != 2 {
		t.Fatalf("pull = %d, want 2 (storm tick + resumed cycle)", got)
	}
	// Секции с новым токеном уходят: сервер (фейк) отвечает ок на "fresh".
	if got := b.count(OpDataStatus); got == 0 {
		t.Fatalf("sections must resume after token change")
	}
	if st := r.Status(); st.AuthState != "" {
		t.Fatalf("auth_state = %q, want cleared after success", st.AuthState)
	}
}

func TestAuthStateAnonRequired(t *testing.T) {
	// Пустой токен + auth_required (чужой отпечаток/нет регистрации) —
	// состояние auth_required, и шторм тоже гасится бэкоффом.
	t0 := time.Now()
	clock := t0
	b := newAuthBackend("")
	flags := newMemFlags()
	r := runnerAt(b, flags, &clock)
	r.Token = func() string { return "" }
	r.Tick(nil)
	if st := r.Status(); st.AuthState != "auth_required" {
		t.Fatalf("auth_state = %q, want auth_required", st.AuthState)
	}
	clock = t0.Add(time.Minute)
	if r.Tick(nil) {
		t.Fatalf("anon auth_required must enter backoff silence")
	}
	if got := b.count(OpPull); got != 1 {
		t.Fatalf("pull = %d, want 1 (no repeated 401)", got)
	}
}

func TestProbeRecovery(t *testing.T) {
	t0 := time.Now()
	clock := t0
	b := newAuthBackend("tok")
	flags := newMemFlags()
	r := runnerAt(b, flags, &clock)
	tok := "tok"
	r.Token = func() string { return tok }
	r.Tick(nil)

	// «Проблема паспорта» ушла на сервере — проба проходит, цикл возобновляется.
	b.failToken = "nobody"
	clock = t0.Add(31 * time.Minute)
	if !r.Tick(nil) {
		t.Fatalf("recovery tick must send")
	}
	if st := r.Status(); st.AuthState != "" {
		t.Fatalf("auth_state = %q, want cleared on recovery", st.AuthState)
	}
	if got := b.count(OpDataStatus); got == 0 {
		t.Fatalf("sections must resume after successful probe")
	}
	// Блок снят: следующий тик снова полноценный (не молчит).
	clock = t0.Add(32 * time.Minute)
	if !r.Tick(nil) {
		t.Fatalf("tick after recovery must send normally")
	}
}

func TestBackoffDoesNotAffectHealthyChannel(t *testing.T) {
	// Обычный анонимный канал (без ошибок) не попадает в бэкофф: hello+pull
	// и секции идут каждый тик (политика 11.2026).
	t0 := time.Now()
	clock := t0
	b := &fakeBackend{}
	flags := newMemFlags()
	r := runnerAt(b, flags, &clock)
	r.Token = func() string { return "" }
	r.Tick(nil)
	clock = t0.Add(time.Minute)
	r.Tick(nil)
	if got := b.count(OpPull); got != 2 {
		t.Fatalf("pull = %d, want 2 (normal cycle)", got)
	}
	if st := r.Status(); st.AuthState != "" {
		t.Fatalf("auth_state = %q, want empty on healthy channel", st.AuthState)
	}
}

func TestSendOneCapsOversizedPayload(t *testing.T) {
	t0 := time.Now()
	clock := t0
	b := &fakeBackend{}
	flags := newMemFlags()
	log := &warnLog{}
	r := runnerAt(b, flags, &clock)
	r.Log = log
	big := make([]map[string]any, 4000)
	for i := range big {
		big[i] = map[string]any{"ts": t0.Add(time.Duration(i) * time.Second).Format(time.RFC3339), "msg": makeString(200, 'x')}
	}
	r.Sources = cappedSources{rows: big}
	r.Tick(nil)
	if b.count(OpDataStatus) == 0 {
		t.Fatalf("status section must be attempted")
	}
	for _, c := range b.calls {
		if c.Op != OpDataStatus {
			continue
		}
		raw, _ := json.Marshal(c.Body)
		if len(raw) > maxSectionPayload+4096 {
			t.Fatalf("sent body = %d B, want <= %d (+envelope slack)", len(raw), maxSectionPayload)
		}
		if log.count() == 0 {
			t.Fatalf("truncation must be logged (Warn)")
		}
		// Хвост сохранён: последняя строка исходного списка доехала.
		rows, _ := c.Body["rows"].([]any)
		if len(rows) == 0 {
			t.Fatalf("rows must survive truncation")
		}
		last, _ := rows[len(rows)-1].(map[string]any)
		if last["msg"] != makeString(200, 'x') || last["ts"] == "" {
			t.Fatalf("tail row lost: %v", last)
		}
	}
}

func makeString(n int, c byte) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = c
	}
	return string(b)
}

type cappedSources struct {
	rows []map[string]any
}

func (s cappedSources) Collect(op string) (map[string]any, error) {
	return map[string]any{"rows": s.rows, "v": 1}, nil
}
func (cappedSources) ApplyPull(data json.RawMessage) {}
func (cappedSources) LastRowTs(op string) string     { return "" }
