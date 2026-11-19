package app

// Супервизор фоновых модулей: паника в горутине фонового цикла не должна
// убивать цикл навсегда (как было с safeGo) — модуль логируется, рестартует
// с нарастающей паузой, а при устойчивом crash-loop (5 рестартов за 10 минут)
// переводится в failed до перезапуска приложения. Уведомления пользователю —
// только через onError-хук логгера (log.Error уже показывает тост и кладёт
// ошибку в очередь на сервер), прямых notify из супервизора нет — иначе
// задваивание.

import (
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

const (
	moduleRestartWindow = 10 * time.Minute // окно подсчёта рестартов для лимита
	moduleRestartLimit  = 5                // рестартов в окне — модуль failed
	moduleStableAfter   = 5 * time.Minute  // прожил дольше — счётчик подряд сбрасывается
	moduleBriefMax      = 200              // размер краткого описания паники в реестре
)

const (
	modRunning    = "running"
	modRestarting = "restarting"
	modFailed     = "failed"
	modStopped    = "stopped"
)

// moduleState — запись реестра по одному фоновому модулю.
type moduleState struct {
	Name          string
	Status        string
	Restarts      int         // рестартов подряд (сбрасывается после moduleStableAfter)
	TotalRestarts int         // рестартов за всю жизнь приложения
	Recent        []time.Time // метки рестартов (для лимита в окне)
	LastRestart   time.Time
	LastError     string
}

// moduleRegistry — реестр всех супервизируемых модулей. Все методы
// потокобезопасны и не зависят от App (тестируются напрямую).
type moduleRegistry struct {
	mu      sync.Mutex
	modules map[string]*moduleState
}

func newModuleRegistry() *moduleRegistry {
	return &moduleRegistry{modules: make(map[string]*moduleState)}
}

func (r *moduleRegistry) getOrCreate(name string) *moduleState {
	m, ok := r.modules[name]
	if !ok {
		m = &moduleState{Name: name, Status: modRunning}
		r.modules[name] = m
	}
	return m
}

// begin отмечает запуск очередного прогона fn.
func (r *moduleRegistry) begin(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.getOrCreate(name)
	m.Status = modRunning
}

// end отмечает штатное завершение fn (одноразовая задача или выход по stopCh).
func (r *moduleRegistry) end(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.getOrCreate(name)
	if m.Status != modFailed {
		m.Status = modStopped
	}
}

// failOnce регистрирует панику и возвращает паузу перед рестартом;
// giveUp=true — лимит исчерпан, рестартов больше не будет.
func (r *moduleRegistry) failOnce(name string, at time.Time, uptime time.Duration, brief string) (backoff time.Duration, giveUp bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.getOrCreate(name)
	if uptime >= moduleStableAfter {
		m.Restarts = 0
	}
	// Лимит — по фактически случившимся рестартам в окне: набрали 5, дальше не рестартим.
	if moduleRestartsInWindow(m.Recent, at, moduleRestartWindow) >= moduleRestartLimit {
		m.Status = modFailed
		return 0, true
	}
	// Редкая, но долгоживущая череда сбоев: не даём истории расти неограниченно.
	if len(m.Recent) > 64 {
		kept := m.Recent[:0]
		for _, ts := range m.Recent {
			if at.Sub(ts) < moduleRestartWindow {
				kept = append(kept, ts)
			}
		}
		m.Recent = kept
	}
	m.Restarts++
	m.TotalRestarts++
	m.Recent = append(m.Recent, at)
	m.LastRestart = at
	m.LastError = brief
	m.Status = modRestarting
	return moduleBackoff(m.Restarts), false
}

// statusOf — снимок состояния модуля (или nil, если модуль не супервизируется).
func (r *moduleRegistry) statusOf(name string) (moduleState, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.modules[name]
	if !ok {
		return moduleState{}, false
	}
	return *m, true
}

// supervisedGo запускает fn в горутине под супервизором: паника → лог +
// рестарт с backoff; штатное завершение → статус stopped.
func (a *App) supervisedGo(name string, fn func()) {
	go a.supRun(name, fn)
}

// supRegistry возвращает реестр модулей, создавая его при первом обращении.
func (a *App) supRegistry() *moduleRegistry {
	a.supMu.Lock()
	defer a.supMu.Unlock()
	if a.sup == nil {
		a.sup = newModuleRegistry()
	}
	return a.sup
}

func (a *App) supRun(name string, fn func()) {
	for {
		reg := a.supRegistry()
		reg.begin(name)
		started := time.Now()
		panicMsg := runSupervised(fn)
		if panicMsg == "" {
			reg.end(name)
			return
		}
		a.log.Error(name, "PANIC: "+panicMsg)
		backoff, giveUp := reg.failOnce(name, time.Now(), time.Since(started), panicBrief(panicMsg))
		if giveUp {
			a.log.Error(name, fmt.Sprintf("module failed: %d restarts in %s — no more restarts until app restart", moduleRestartLimit, moduleRestartWindow))
			return
		}
		select {
		case <-a.stopCh:
			reg.end(name)
			return
		case <-time.After(backoff):
		}
	}
}

// runSupervised выполняет fn и возвращает полный текст паники со стеком
// ("" — штатное завершение).
func runSupervised(fn func()) (panicMsg string) {
	defer func() {
		if r := recover(); r != nil {
			panicMsg = fmt.Sprintf("%v\n%s", r, debug.Stack())
		}
	}()
	fn()
	return ""
}

// panicBrief — первая строка паники, укороченная для реестра/отчёта.
func panicBrief(msg string) string {
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	msg = strings.TrimSpace(msg)
	if len(msg) > moduleBriefMax {
		msg = msg[:moduleBriefMax]
	}
	return msg
}

// moduleBackoff — пауза перед рестартом по его номеру (подряд): 1с, 5с, 15с, далее 60с cap.
func moduleBackoff(restartNum int) time.Duration {
	switch {
	case restartNum <= 1:
		return time.Second
	case restartNum == 2:
		return 5 * time.Second
	case restartNum == 3:
		return 15 * time.Second
	default:
		return 60 * time.Second
	}
}

// moduleRestartsInWindow — сколько меток из recent попадают в окно window от момента now.
func moduleRestartsInWindow(recent []time.Time, now time.Time, window time.Duration) int {
	n := 0
	for _, ts := range recent {
		if now.Sub(ts) < window {
			n++
		}
	}
	return n
}
