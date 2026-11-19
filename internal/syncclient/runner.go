package syncclient

// Раннер канала ZPUI Sync (06-Клиент.md §1): тик 60с + джиттер ±20%,
// на тике sync.pull → просроченные секции (максимум 1–2, по старшинству),
// талонная логика (busy → тишина до следующего тика, повтор с талоном),
// курсоры диффов и хеши «по изменению» — в флагах БД. Экономный режим:
// Stretch растягивает тик, уровень 3 пропускает тики.

import (
	"encoding/json"
	"strconv"
	"sync"
	"time"
)

// Flags — хранилище состояния планировщика (database.SetFlag/GetFlag).
type Flags interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// Logger — минимальный интерфейс лога.
type Logger interface {
	Info(source, msg string)
	Warn(source, msg string)
}

// Sources — поставщики данных приложения: сбор секции и применение pull.
type Sources interface {
	// Collect — payload секции (без cursor/hash — их добавляет раннер).
	Collect(op string) (map[string]any, error)
	// ApplyPull — применить ответ sync.pull (списки, recommended, requests).
	ApplyPull(data json.RawMessage)
	// LastRowTs — метка последней строки диффа (для фолбэка курсора, когда
	// сервер не вернул ack); "" — строк нет.
	LastRowTs(op string) string
}

// PullData — разобранный ответ sync.pull (для UI-журнала и биндингов).
type PullData struct {
	ExclusionsVersion int64
	DNSVersion        int64
	Recommended       map[string]any
	RequestsNow       []string
	RequestsSkips     bool
}

// Runner — цикл устройства ZPUI Sync.
type Runner struct {
	Backend    Backend
	Dev        func() Dev
	Token      func() string // "" = без аккаунта: канал работает анонимно, по паспорту dev
	Flags      Flags
	Log        Logger
	Sources    Sources
	Catalog    []Section
	Overridden func() Overrides // текущие юзер-оверрайды (из конфига)
	Stretch    func(time.Duration) time.Duration
	SkipTick   func() bool // экономный режим уровня 3 — пропустить тик
	MaxPerTick int
	Now        func() time.Time

	// born — момент первого тика: первые 2 минуты действует стартовый burst
	// (лимит тика снят — все просроченные секции уходят сразу, «данные
	// подтягиваются при запуске»).
	born time.Time
	Rand       func() float64
	// OnBanned — токен отозван: приложение чистит AccountToken.
	OnBanned func()
	// OnAuthRequired — сессия истекла/несуществующяя (sync.pull ответил
	// auth_required при непустом токене): приложение чистит AccountToken и
	// предлагает войти заново. Иначе канал молчит вечно, а списки
	// (скип-листы, DNS) перестают обновляться незаметно для пользователя.
	OnAuthRequired func()
	// Journal — каждая реальная пересылка канала: who = device (свой запрос)
	// или server (просьба из pull); result = "" либо код ошибки. Для UI-истории.
	Journal func(who, op, result string)

	// requestGate — повторное исполнение просьбы по секции не чаще раза в час.
	RequestGate time.Duration

	mu          sync.Mutex
	ticket      ticketState
	lastTick    time.Time
	lastRTTms   int64
	online      bool
	lastErr     string
	lastNext    int
	pull        PullData
	requestsLog []map[string]any
	bootDone    bool
}

type ticketState struct {
	Op      string `json:"op"`
	Ticket  string `json:"ticket"`
	Issued  int64  `json:"issued"`
	Refresh bool   `json:"refresh"` // повтор тоже отклонён — дальше только талоны
}

const (
	flagPrefixLast   = "sync_last_"   // sync_last_data_status = RFC3339
	flagPrefixCursor = "sync_cursor_" // sync_cursor_data_availability
	flagPrefixHash   = "sync_hash_"   // sync_hash_data_config
	flagTicket       = "sync_ticket"  // JSON ticketState
	flagNextAfter    = "sync_next_after"
)

// Status — снимок состояния канала для UI (GetSyncInfo).
type Status struct {
	Online      bool
	LastTick    time.Time
	RTTms       int64
	LastError   string
	NextAfter   int
	TicketOp    string
	Pull        PullData
	RequestsLog []map[string]any
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// jrn — запись в журнал истории (nil-safe).
func (r *Runner) jrn(who, op, result string) {
	if r.Journal != nil {
		r.Journal(who, op, result)
	}
}

// jrnErr — код ошибки конверта для журнала; сетевая — "net".
func jrnErr(err error) string {
	if err == nil {
		return ""
	}
	if e, ok := AsError(err); ok {
		return e.Code
	}
	return "net"
}

func (r *Runner) maxPerTick() int {
	now := r.now()
	r.mu.Lock()
	if r.born.IsZero() {
		r.born = now
	}
	born := r.born
	r.mu.Unlock()
	if now.Sub(born) < 2*time.Minute {
		return len(Catalog()) + 4 // стартовый burst: весь каталог сразу
	}
	if r.MaxPerTick > 0 {
		return r.MaxPerTick
	}
	return 2
}

// exclusionsDebounce — смена пользовательского скип-листа уезжает не сразу:
// ждём ~минуту после ПОСЛЕДНЕГО изменения (владелец: «система ждёт, пока
// пользователь перестанет добавлять»).
const exclusionsDebounce = time.Minute

func (r *Runner) exclusionsDirty(now time.Time) (time.Time, bool) {
	v := r.getFlag("sync_excl_dirty")
	if v == "" {
		return time.Time{}, false
	}
	t, _ := time.Parse(time.RFC3339, v)
	if t.IsZero() {
		return time.Time{}, false
	}
	return t, now.Sub(t) >= exclusionsDebounce
}

func (r *Runner) catalog() []Section {
	if len(r.Catalog) > 0 {
		return r.Catalog
	}
	return Catalog()
}

// ---- состояние в флагах ----

func (r *Runner) getFlag(key string) string {
	if r.Flags == nil {
		return ""
	}
	v, _ := r.Flags.Get(key)
	return v
}

func (r *Runner) setFlag(key, value string) {
	if r.Flags == nil {
		return
	}
	_ = r.Flags.Set(key, value)
}

func parseRFC3339(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func (r *Runner) lastOKMap() map[string]time.Time {
	m := map[string]time.Time{}
	for _, s := range r.catalog() {
		if t := parseRFC3339(r.getFlag(flagPrefixLast + SectionKey(s.Op))); !t.IsZero() {
			m[s.Op] = t
		}
	}
	return m
}

func (r *Runner) markOK(op string, t time.Time) {
	r.setFlag(flagPrefixLast+SectionKey(op), t.Format(time.RFC3339))
}

func (r *Runner) cursor(op string) string {
	return r.getFlag(flagPrefixCursor + SectionKey(op))
}

func (r *Runner) setCursor(op, v string) {
	r.setFlag(flagPrefixCursor+SectionKey(op), v)
}

func (r *Runner) hash(op string) string {
	return r.getFlag(flagPrefixHash + SectionKey(op))
}

func (r *Runner) setHash(op, v string) {
	r.setFlag(flagPrefixHash+SectionKey(op), v)
}

func (r *Runner) loadTicket() (ticketState, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ticket.Ticket != "" {
		return r.ticket, true
	}
	return ticketState{}, false
}

func (r *Runner) saveTicket(t ticketState) {
	r.mu.Lock()
	r.ticket = t
	r.mu.Unlock()
	raw, _ := json.Marshal(t)
	r.setFlag(flagTicket, string(raw))
}

func (r *Runner) clearTicket() {
	r.mu.Lock()
	r.ticket = ticketState{}
	r.mu.Unlock()
	r.setFlag(flagTicket, "")
}

func (r *Runner) statusSnap(fn func(Status)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn(Status{
		Online:      r.online,
		LastTick:    r.lastTick,
		RTTms:       r.lastRTTms,
		LastError:   r.lastErr,
		NextAfter:   r.lastNext,
		TicketOp:    r.ticket.Op,
		Pull:        r.pull,
		RequestsLog: r.requestsLog,
	})
}

// Status — текущее состояние канала.
func (r *Runner) Status() Status {
	var s Status
	r.statusSnap(func(st Status) { s = st })
	return s
}

// CursorOf — текущий курсор дифф-секции (для LastRowTs и UI).
func (r *Runner) CursorOf(op string) string {
	return r.cursor(op)
}

// AddRequestsLog — дописать запись в журнал запросов сервера.
// Кольцо на 20 записей и TTL: записи старше часа не хранятся.
func (r *Runner) AddRequestsLog(entry map[string]any) {
	r.mu.Lock()
	r.requestsLog = append(r.requestsLog, entry)
	r.requestsLog = trimRequestsLog(r.requestsLog)
	log := make([]map[string]any, len(r.requestsLog))
	copy(log, r.requestsLog)
	r.mu.Unlock()
	raw, _ := json.Marshal(log)
	r.setFlag("sync_requests_log", string(raw))
}

// requestsLogTTL — сколько хранить записи журнала просьб сервера.
const requestsLogTTL = time.Hour

func requestsLogAt(entry map[string]any) time.Time {
	s, _ := entry["at"].(string)
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// trimRequestsLog — кольцо на 20 + срез записей старше часа
// (нераспознанное время трактуем как просроченное).
func trimRequestsLog(log []map[string]any) []map[string]any {
	if len(log) > 20 {
		log = log[len(log)-20:]
	}
	fresh := log[:0]
	cut := time.Now().Add(-requestsLogTTL)
	for _, e := range log {
		at := requestsLogAt(e)
		if at.IsZero() || at.Before(cut) {
			continue
		}
		fresh = append(fresh, e)
	}
	return fresh
}

func (r *Runner) loadRequestsLog() {
	raw := r.getFlag("sync_requests_log")
	if raw == "" {
		return
	}
	var log []map[string]any
	if json.Unmarshal([]byte(raw), &log) == nil && len(log) > 0 {
		log = trimRequestsLog(log)
		r.mu.Lock()
		r.requestsLog = log
		r.mu.Unlock()
	}
}

// ---- тик ----

// Run — основной цикл до закрытия stop. Первый тик — сразу.
func (r *Runner) Run(stop <-chan struct{}) {
	r.loadRequestsLog()
	for {
		r.Tick(stop)
		wait := r.nextWait()
		if r.Stretch != nil {
			wait = r.Stretch(wait)
		}
		select {
		case <-stop:
			return
		case <-time.After(wait):
		}
	}
}

// nextWait — пауза до следующего тика: next_after сервера (по умолчанию 60с)
// с джиттером ±20%, зажатый в [30с, 10мин].
func (r *Runner) nextWait() time.Duration {
	base := 60 * time.Second
	r.mu.Lock()
	na := r.lastNext
	r.mu.Unlock()
	if na > 0 {
		base = time.Duration(na) * time.Second
	}
	if base < 30*time.Second {
		base = 30 * time.Second
	}
	if base > 10*time.Minute {
		base = 10 * time.Minute
	}
	return Jitter(base, 0.2, r.Rand)
}

// Tick — один тик канала. Возвращает true, если хоть один запрос ушёл.
func (r *Runner) Tick(stop <-chan struct{}) bool {
	if r.SkipTick != nil && r.SkipTick() {
		return false // экономный режим уровня 3: тик пропущен
	}
	dev := Dev{}
	if r.Dev != nil {
		dev = r.Dev()
	}
	if dev.ID == "" {
		return false
	}
	token := ""
	if r.Token != nil {
		token = r.Token()
	}
	now := r.now()

	sent := false
	// hello: регистрация/обновление паспорта — при старте и пока нет токена.
	if !r.bootDone || token == "" {
		if r.sendHello(dev, token) {
			sent = true
		}
		r.mu.Lock()
		r.bootDone = true
		r.mu.Unlock()
	}

	// 1. sync.pull — лёгкое чтение (списки, recommended, requests), каждый тик.
	r.doPull(dev, token)
	sent = true

	// 2. Просьбы сервера («пришли секцию сейчас») — приоритет над расписанием,
	//    но не чаще раза в час на секцию (03 §4).
	if r.sendRequested(dev, token, now) {
		sent = true
	}

	// 3. Просроченные секции.
	if r.sendDueSections(dev, token, now, stop) {
		sent = true
	}

	r.mu.Lock()
	r.lastTick = now
	r.mu.Unlock()
	return sent
}

func (r *Runner) sendHello(dev Dev, token string) bool {
	start := time.Now()
	resp, err := r.Backend.Call(OpHello, dev, token, "", map[string]any{})
	r.recordCall(start, err)
	r.jrn("device", OpHello, jrnErr(err))
	if err != nil {
		r.handleEnvelopeError(err)
		return false
	}
	if resp != nil && resp.NextAfter > 0 {
		r.setNextAfter(resp.NextAfter)
	}
	return true
}

func (r *Runner) doPull(dev Dev, token string) {
	start := time.Now()
	resp, err := r.Backend.Call(OpPull, dev, token, "", map[string]any{
		"have": map[string]int64{
			"exclusions":    int64FromJSON(r.getFlag("exclusions_version")),
			"dns_providers": int64FromJSON(r.getFlag("dns_system_version")),
		},
	})
	r.recordCall(start, err)
	r.jrn("device", OpPull, jrnErr(err))
	if err != nil {
		r.handleEnvelopeError(err)
		return
	}
	if resp.NextAfter > 0 {
		r.setNextAfter(resp.NextAfter)
	}
	if r.Sources != nil && len(resp.Data) > 0 {
		r.Sources.ApplyPull(resp.Data)
	}
	r.parsePull(resp.Data)
}

func (r *Runner) parsePull(data json.RawMessage) {
	if len(data) == 0 {
		return
	}
	var pd struct {
		Recommended map[string]any `json:"recommended"`
		Requests    struct {
			Now   []string `json:"now"`
			Skips bool     `json:"skips"`
		} `json:"requests"`
	}
	if err := json.Unmarshal(data, &pd); err != nil {
		return
	}
	seen := map[string]bool{}
	for _, name := range pd.Requests.Now {
		seen[name] = true
		r.jrn("server", opDataPrefix+name, "")
	}
	if pd.Requests.Skips && !seen["exclusions"] {
		r.jrn("server", opDataPrefix+"exclusions", "")
	}
	r.mu.Lock()
	r.pull = PullData{
		Recommended:   pd.Recommended,
		RequestsNow:   pd.Requests.Now,
		RequestsSkips: pd.Requests.Skips,
	}
	r.mu.Unlock()
}

func int64FromJSON(s string) int64 {
	if s == "" {
		return 0
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func (r *Runner) setNextAfter(na int) {
	r.mu.Lock()
	r.lastNext = na
	r.mu.Unlock()
	r.setFlag(flagNextAfter, itoa(na))
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func (r *Runner) recordCall(start time.Time, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastRTTms = time.Since(start).Milliseconds()
	if err != nil {
		r.online = false
		r.lastErr = err.Error()
	} else {
		r.online = true
		r.lastErr = ""
	}
}

// handleEnvelopeError — реакции на коды конверта (banned → чистим токен).
func (r *Runner) handleEnvelopeError(err error) {
	e, ok := AsError(err)
	if !ok {
		return
	}
	switch e.Code {
	case "banned":
		if r.OnBanned != nil {
			r.OnBanned()
		}
	case "auth_required":
		// Сессии нет (истекла/потеряна на сервере), а токен в конфиге ещё
		// стоит: гасим токен, приложение вернётся в неавторизованное
		// состояние и предложит вход. Без этого канал ошибок не показывал.
		if r.OnAuthRequired != nil {
			r.OnAuthRequired()
		}
	}
}

// sendRequested — исполнить просьбы сервера из последнего pull: секция
// уходит немедленно (в бюджет тика), но не чаще requestGate (час) на секцию.
func (r *Runner) sendRequested(dev Dev, token string, now time.Time) bool {
	r.mu.Lock()
	nowReq := append([]string(nil), r.pull.RequestsNow...)
	skips := r.pull.RequestsSkips
	r.mu.Unlock()
	if len(nowReq) == 0 && !skips {
		return false
	}
	if skips {
		// «скипы» — просьба прислать свежие скип-листы: секция exclusions.
		found := false
		for _, s := range nowReq {
			if s == "exclusions" {
				found = true
			}
		}
		if !found {
			nowReq = append(nowReq, "exclusions")
		}
	}
	gate := r.RequestGate
	if gate <= 0 {
		gate = time.Hour
	}
	sent := false
	r.mu.Lock()
	ticket := r.ticket
	r.mu.Unlock()
	if ticket.Ticket != "" {
		return false // активный талон — просьбы подождут следующего тика
	}
	budget := r.maxPerTick()
	for _, name := range nowReq {
		if budget <= 0 {
			break
		}
		op := opDataPrefix + name
		if _, ok := FindSection(op); !ok {
			continue
		}
		lastExec := parseRFC3339(r.getFlag("sync_reqexec_" + SectionKey(op)))
		if !lastExec.IsZero() && now.Sub(lastExec) < gate {
			continue
		}
		if r.sendOne(dev, token, op, "", now) {
			sent = true
			budget--
		}
		r.setFlag("sync_reqexec_"+SectionKey(op), now.Format(time.RFC3339))
		if _, has := r.loadTicket(); has {
			break // busy — тишина до следующего тика
		}
	}
	return sent
}

// sendDueSections — активный талон (повтор с ним) либо до maxPerTick
// просроченных секций. busy → талон сохранён, дальше молчим до следующего тика.
// Канал работает и без аккаунта: секции уходят анонимно, по паспорту dev
// (политика 11.2026, 02 §6).
func (r *Runner) sendDueSections(dev Dev, token string, now time.Time, stop <-chan struct{}) bool {
	// Активный талон: повтор именно этой секции с талоном, больше ничего.
	if t, ok := r.loadTicket(); ok {
		r.sendOne(dev, token, t.Op, t.Ticket, now)
		return true
	}

	ov := Overrides{}
	if r.Overridden != nil {
		ov = r.Overridden()
	}
	due := PickDue(r.catalog(), ov, r.lastOKMap(), now, r.maxPerTick(), nil)
	if len(due) == 0 {
		return false
	}
	sent := false
	for _, s := range due {
		if r.sendOne(dev, token, s.Op, "", now) {
			sent = true
		}
		// busy: тишина до следующего тика — бюджет тика исчерпан.
		if _, hasTicket := r.loadTicket(); hasTicket {
			break
		}
	}
	return sent
}

// sendOne — собрать и отправить одну секцию; обработать ack/хеш/талон.
// Возвращает true, если запрос реально ушёл.
func (r *Runner) sendOne(dev Dev, token, op, ticket string, now time.Time) bool {
	sec, ok := FindSection(op)
	if !ok {
		return false
	}
	payload, err := r.Sources.Collect(op)
	if err != nil || payload == nil {
		payload = map[string]any{}
	}
	payload["collected_at"] = now.Format(time.RFC3339)

	// «По изменению»: хеш от данных без collected_at — совпал, отправки нет.
	hashable := func() map[string]any {
		cp := make(map[string]any, len(payload))
		for k, v := range payload {
			if k == "collected_at" {
				continue
			}
			cp[k] = v
		}
		return cp
	}
	if sec.OnChange && ticket == "" {
		sum := HashJSON(hashable())
		if sum != "" && sum == r.hash(op) {
			r.markOK(op, now) // данных не изменилось — секция «актуальна»
			if op == "data.exclusions" {
				r.setFlag("sync_excl_dirty", "")
			}
			return false
		}
		if op == "data.exclusions" && ticket == "" {
			// Дебаунс: изменение замечено — ждём минуту от последнего
			// изменения, затем список уезжает целиком.
			if _, ready := r.exclusionsDirty(now); !ready {
				r.setFlag("sync_excl_dirty", now.Format(time.RFC3339))
				return false
			}
			r.setFlag("sync_excl_dirty", "")
		}
	}
	// Дифф: курсор в payload; сдвиг только после ack сервера (03 §3).
	if sec.Diff {
		if cur := r.cursor(op); cur != "" {
			payload["cursor"] = cur
		}
	}

	start := time.Now()
	resp, err := r.Backend.Call(op, dev, token, ticket, payload)
	r.recordCall(start, err)
	r.jrn("device", op, jrnErr(err))
	if err != nil {
		r.handleEnvelopeError(err)
		if e, ok := AsError(err); ok && e.Code == "busy" && e.Ticket != "" {
			r.saveTicket(ticketState{Op: op, Ticket: e.Ticket, Issued: now.Unix(), Refresh: ticket != ""})
			if r.Log != nil {
				r.Log.Warn("sync", "busy: ticket "+e.Ticket+" for "+op+", silent until next tick")
			}
		}
		return true
	}
	if resp.NextAfter > 0 {
		r.setNextAfter(resp.NextAfter)
	}
	if ticket != "" {
		r.clearTicket() // слот получен — талон погашен (02 §4)
	}
	r.markOK(op, now)
	if op == "data.exclusions" {
		r.setFlag("sync_excl_dirty", "")
	}
	if sec.OnChange {
		if sum := HashJSON(hashable()); sum != "" {
			r.setHash(op, sum)
		}
	}
	if sec.Diff {
		ack := ackCursor(resp.Data)
		if ack == "" {
			ack = r.Sources.LastRowTs(op)
		}
		if ack != "" {
			r.setCursor(op, ack)
		}
	}
	return true
}

// ackCursor — cursor_ack из data.ack ответа (03 §3).
func ackCursor(data json.RawMessage) string {
	if len(data) == 0 {
		return ""
	}
	var d struct {
		Ack struct {
			CursorAck string `json:"cursor_ack"`
		} `json:"ack"`
	}
	if json.Unmarshal(data, &d) != nil {
		return ""
	}
	return d.Ack.CursorAck
}
