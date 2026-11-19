package syncclient

import "time"

// Каталог секций (03-Секции-данных.md §2). Состав жёстко зашит: устройство
// не имеет операции для чего-либо вне каталога — это граница приватности.
// Интервалы — серверный контракт; множители юзер-оверрайдов только растягивают.

const (
	OpHello       = "sync.hello"
	OpPull        = "sync.pull"
	OpDataStatus  = "data.status"
	opDataPrefix  = "data."
	sectionPrefix = "data_"
)

// Section — запись каталога.
type Section struct {
	Op       string        // имя операции ("data.status")
	Interval time.Duration // частота отправки
	OnChange bool          // «по изменению»: локальный хеш, отправка при смене (плюс редкий re-assert по интервалу)
	Diff     bool          // курсорная секция: курсор сдвигается после ack сервера
}

// Catalog — все 13 секций каталога в порядке 03 §2.
func Catalog() []Section {
	return []Section{
		{Op: "data.status", Interval: 60 * time.Second},
		{Op: "data.runtime", Interval: 5 * time.Minute},
		{Op: "data.availability", Interval: 30 * time.Minute, Diff: true},
		{Op: "data.resources", Interval: 24 * time.Hour, Diff: true},
		{Op: "data.network", Interval: 30 * time.Minute},
		{Op: "data.strategies", Interval: time.Hour},
		{Op: "data.traffic", Interval: time.Hour},
		{Op: "data.operator", Interval: 6 * time.Hour, OnChange: true},
		{Op: "data.errors", Interval: 6 * time.Hour, Diff: true},
		{Op: "data.events", Interval: 6 * time.Hour, Diff: true},
		{Op: "data.system", Interval: 24 * time.Hour},
		{Op: "data.versions", Interval: 24 * time.Hour, OnChange: true},
		{Op: "data.config", Interval: 24 * time.Hour, OnChange: true},
		{Op: "data.exclusions", Interval: 24 * time.Hour, OnChange: true},
	}
}

// FindSection — запись каталога по имени операции.
func FindSection(op string) (Section, bool) {
	for _, s := range Catalog() {
		if s.Op == op {
			return s, true
		}
	}
	return Section{}, false
}

// SectionKey — ключ флагов по операции: "data.status" → "data_status".
func SectionKey(op string) string {
	return sectionPrefix + op[len(opDataPrefix):]
}

// Override — юзер-оверрайд секции: множитель частоты (1|2|5) или выключение.
type Override struct {
	Mult int  `json:"mult,omitempty"`
	Off  bool `json:"off,omitempty"`
}

// Overrides — оверрайды по имени секции ("status", "runtime", ...).
type Overrides map[string]Override

// sectionName — короткое имя секции из операции ("data.status" → "status").
func sectionName(op string) string {
	return op[len(opDataPrefix):]
}

// normalizeMult клампит множитель в {1,2,5}; прочие значения → 1.
func normalizeMult(m int) int {
	switch m {
	case 2, 5:
		return m
	default:
		return 1
	}
}

// Disabled — секция выключена пользователем.
func (o Overrides) Disabled(op string) bool {
	if o == nil {
		return false
	}
	if ov, ok := o[sectionName(op)]; ok {
		return ov.Off
	}
	return false
}

// Effective — интервал с оверрайдом (множитель растягивает интервал).
func (o Overrides) Effective(s Section) time.Duration {
	if o == nil {
		return s.Interval
	}
	m := 1
	if ov, ok := o[sectionName(s.Op)]; ok {
		m = normalizeMult(ov.Mult)
	}
	return s.Interval * time.Duration(m)
}

// PickDue — просроченные секции по старшинству просрочки (отношение
// «прошло с последнего ack» к эффективному интервалу), максимум max штук.
// Выключенные оверрайдом и имеющие активный талон не возвращаются.
func PickDue(catalog []Section, ov Overrides, lastOK map[string]time.Time, now time.Time, max int, skip map[string]bool) []Section {
	type cand struct {
		s        Section
		overdueF float64
	}
	var out []cand
	for _, s := range catalog {
		if ov.Disabled(s.Op) || skip[s.Op] {
			continue
		}
		interval := ov.Effective(s)
		if interval <= 0 {
			continue
		}
		last, seen := lastOK[s.Op]
		if !seen || last.IsZero() {
			out = append(out, cand{s, 1e18}) // никогда не отправлялась — первый приоритет
			continue
		}
		elapsed := now.Sub(last)
		if elapsed < interval {
			continue
		}
		out = append(out, cand{s, float64(elapsed) / float64(interval)})
	}
	// по убыванию просрочки
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].overdueF > out[j-1].overdueF; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	res := make([]Section, 0, len(out))
	for _, c := range out {
		res = append(res, c.s)
	}
	return res
}
