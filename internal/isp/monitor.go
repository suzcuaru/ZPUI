package isp

import (
	"fmt"
	"sync"
	"time"

	"zpui/internal/database"
	"zpui/internal/power"
)

type OperatorChange struct {
	OldKey  string
	OldName string
	NewKey  string
	NewName string
	Op      *Operator
}

type LogFunc func(category, msg string)

// retryInterval — повтор неудачного детекта (до первого успеха).
const retryInterval = 12 * time.Minute

type Monitor struct {
	log           LogFunc
	checkInterval time.Duration
	onChange      func(OperatorChange)
	stopCh        chan struct{}

	mu         sync.Mutex
	currentKey string
	lastOK     bool      // успех последнего детекта (для ретрай-интервала)
	lastGood   *Operator // кэш последнего успешного определения
}

func NewMonitor(log LogFunc, checkInterval time.Duration, onChange func(OperatorChange)) *Monitor {
	return &Monitor{
		log:           log,
		checkInterval: checkInterval,
		onChange:      onChange,
		stopCh:        make(chan struct{}),
	}
}

func (m *Monitor) Start() {
	m.detectAndSave()

	for {
		m.mu.Lock()
		wait := m.checkInterval
		if !m.lastOK {
			wait = retryInterval
		}
		m.mu.Unlock()
		// Экономный режим растягивает опрос: каждый тик — внешние запросы.
		interval := power.Stretch(wait)
		select {
		case <-m.stopCh:
			return
		case <-time.After(interval):
			// Экономный режим уровня 3: внешние запросы приостановлены.
			if power.ServicesPaused() {
				continue
			}
			m.detectAndSave()
		}
	}
}

func (m *Monitor) Stop() {
	close(m.stopCh)
}

// detectAndSave — один цикл определения. Возвращает успех: частичные данные
// и сетевые сбои — не успех (ретрай через retryInterval), полная неудача
// без кэша успеха деградирует в Unknown (легаси-механика миграции стратегий).
func (m *Monitor) detectAndSave() bool {
	op, err := DetectOperator()
	if err != nil {
		m.log("isp", "detect failed: "+err.Error())
		m.setLastOK(false)
		return false
	}

	// Частичные данные (нет имени оператора) — не пишем в БД и не
	// затираем последнего успеха: ретраим.
	if op.Key == "" && op.Name == "" && op.ISP == "" {
		m.log("isp", fmt.Sprintf("detect incomplete (ip=%s asn=%s) — keeping previous, retry scheduled", op.IP, op.ASN))
		m.setLastOK(false)
		return false
	}

	if op.Key == "unknown" {
		m.mu.Lock()
		haveCache := m.lastGood != nil
		m.mu.Unlock()
		if haveCache {
			m.log("isp", "detect failed (all providers) — keeping cached operator, retry scheduled")
			m.setLastOK(false)
			return false
		}
		m.log("isp", "detect failed: no data from providers")
	}

	// Одна физическая сеть у разных geo-IP источников зовётся по-разному
	// («PJSC Rostelecom» / «Rostelecom networks» / «Ivtelecom» — у всех один
	// ASN): без привязки к ASN ключ оператора прыгал между вариантами, теряя
	// историю автотестов и серверные рейтинги. Каноничный ключ — самый старый
	// с этим ASN; имя тоже остаётся прежним, чтобы футер не мигал. Свежий
	// вариант имени никуда не теряется — оператор_info хранит историю.
	if op.ASN != "" {
		if canon, aliases, err := database.ResolveOperatorByASN(op.ASN); err == nil && canon != "" {
			if canon != op.Key {
				if info, _ := database.GetOperatorInfo(canon); info != nil && info.Name != "" {
					op.Key = canon
					op.Name = info.Name
				}
			}
			if len(aliases) > 0 {
				if n, err := database.MergeOperatorStrategies(aliases, canon); err == nil && n > 0 {
					m.log("isp", fmt.Sprintf("merged %d autoselect rows from %d alias keys into %s", n, len(aliases), canon))
				}
			}
		}
	}

	now := time.Now()
	err = database.UpsertOperatorInfo(&database.OperatorInfo{
		Key:       op.Key,
		Name:      op.Name,
		ISP:       op.ISP,
		ASN:       op.ASN,
		City:      op.City,
		Org:       op.Org,
		FirstSeen: now,
		LastSeen:  now,
	})
	if err != nil {
		m.log("isp", "upsert operator info failed: "+err.Error())
	}

	cur, _ := database.GetCurrentOperator()
	oldKey := ""
	oldName := ""
	if cur != nil {
		oldKey = cur.OperatorKey
		oldName = cur.OperatorName
	}

	if n, err := database.MigrateUnknownOperatorStrategies(op.Key); err == nil && n > 0 {
		m.log("isp", fmt.Sprintf("migrated %d autoselect rows from unknown operator", n))
	}

	// Стратегия оператора (ручной выбор или «лучшая» из подбора, см.
	// SetCurrentOperator в zapret/strategy.go) переживает фоновый детект:
	// пустая строка здесь затирала её каждые 60 секунд. Сбрасываем только
	// при реальной смене оператора — новая сеть начинается с чистого листа.
	keepStrategy := ""
	if oldKey != "" && oldKey == op.Key {
		keepStrategy = cur.Strategy
	}
	database.SetCurrentOperator(op.Key, op.Name, keepStrategy)

	if oldKey != "" && oldKey != op.Key {
		m.log("isp", fmt.Sprintf("operator changed: %s -> %s", oldName, op.Name))
		if m.onChange != nil {
			m.onChange(OperatorChange{
				OldKey:  oldKey,
				OldName: oldName,
				NewKey:  op.Key,
				NewName: op.Name,
				Op:      op,
			})
		}
	} else if oldKey == "" {
		m.log("isp", "operator detected: "+op.Name)
		if m.onChange != nil {
			m.onChange(OperatorChange{
				OldKey:  "",
				OldName: "",
				NewKey:  op.Key,
				NewName: op.Name,
				Op:      op,
			})
		}
	}

	m.mu.Lock()
	m.currentKey = op.Key
	m.lastGood = op
	m.lastOK = op.Key != "unknown"
	m.mu.Unlock()
	return op.Key != "unknown"
}

func (m *Monitor) setLastOK(ok bool) {
	m.mu.Lock()
	m.lastOK = ok
	m.mu.Unlock()
}

func (m *Monitor) GetCurrentKey() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.currentKey
}

func DetectOnce() (*Operator, error) {
	return DetectOperator()
}
