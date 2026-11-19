package zsvc

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"zpui/internal/executil"
)

const (
	appImageName  = "ZPUI.exe"
	engineImgName = "winws.exe"
	engineSvcName = "zapret"

	tickInterval    = 15 * time.Second
	restartWaitKill = 10 * time.Second

	// Мониторинг движка: 2 подряд мёртвых тика (30с) → попытка подъёма,
	// между попытками — cooldown.
	engineDeadTicksToRevive = 2
	engineReviveCooldown    = 90 * time.Second

	// svcStartDisabled — Start службы «zapret» = SERVICE_DISABLED
	// (автозапуск движка отключён, watchdog его не поднимает).
	svcStartDisabled = 4
)

// Backoff перезапуска после краша: 1с → 5с → 15с.
func backoffFor(crashNum int) time.Duration {
	switch {
	case crashNum <= 1:
		return 1 * time.Second
	case crashNum == 2:
		return 5 * time.Second
	default:
		return 15 * time.Second
	}
}

func crashLimitReached(count int) bool { return count >= crashLimit }

// nextCrashCount — счётчик крашей в скользящем окне 1 час (чистая функция).
func nextCrashCount(count int, windowStart, now time.Time) (int, time.Time) {
	if windowStart.IsZero() || now.Sub(windowStart) > crashWindow {
		return 1, now
	}
	return count + 1, windowStart
}

const (
	exitFlagNone = iota
	exitFlagClean
	exitFlagRestart
)

// Watchdog — цикл надзора за процессом приложения и движком (тик 15с).
type Watchdog struct {
	log *SvcLog

	mu           sync.Mutex
	hasHello     bool
	helloPid     uint32
	helloVersion string
	helloHidden  bool
	exitFlag     int
	updateUntil  time.Time
	logoffSeen   bool
	cycleStop    bool
	nextAttempt  time.Time

	// Состояние мониторинга движка (zapret): наблюдаемая картина прошлого
	// тика + счётчики подряд-тиков и подъёмов. Только в памяти.
	engineInstalled bool
	engineAutostart bool
	engineRunning   bool
	engineDeadTicks int
	engineNextTrial time.Time
	engineRevives   int
	engineLastTry   time.Time
}

func NewWatchdog(log *SvcLog) *Watchdog { return &Watchdog{log: log} }

// saveConfig — запись конфига службы с журналированием провала: тихо
// пропавшее состояние (порт, версия, счётчик крашей) не поддаётся диагностике.
func (w *Watchdog) saveConfig(cfg *Config) {
	if err := SaveConfig(*cfg); err != nil {
		w.log.Error("config save: " + err.Error())
	}
}

func (w *Watchdog) Run(ctx context.Context) {
	t := time.NewTicker(tickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.tickOnce()
		}
	}
}

// SetHello — приложение сообщило о запуске: сброс счётчика крашей и CycleStop.
// autostart — состояние пользовательского автозапуска (Run-ключ): гард
// краш-восстановления, службе HKCU пользователя не виден.
func (w *Watchdog) SetHello(pid uint32, version string, hidden, autostart bool) {
	w.mu.Lock()
	w.hasHello, w.helloPid, w.helloVersion, w.helloHidden = true, pid, version, hidden
	w.exitFlag, w.updateUntil, w.cycleStop, w.nextAttempt, w.logoffSeen = exitFlagNone, time.Time{}, false, time.Time{}, false
	w.mu.Unlock()
	w.log.Info(fmt.Sprintf("app-hello pid=%d version=%s hidden=%v autostart=%v", pid, version, hidden, autostart))
	cfg := LoadConfig()
	cfg.Status = StatusWatching
	cfg.LastAppStart = time.Now()
	cfg.CrashCount = 0
	cfg.CrashWindowStart = time.Time{}
	cfg.WatchdogPausedUntil = time.Time{}
	cfg.Autostart = autostart
	cfg.Hidden = hidden
	w.saveConfig(&cfg)
}

// SetExiting — приложение завершается штатно.
func (w *Watchdog) SetExiting() {
	w.markIntentionalExit("app-exiting")
}

// SetRestarting — приложение завершается для перезапуска (обновление и т.п.).
func (w *Watchdog) SetRestarting() {
	w.markIntentionalExit("app-restarting")
}

func (w *Watchdog) markIntentionalExit(cmd string) {
	w.mu.Lock()
	w.exitFlag = exitFlagClean
	w.mu.Unlock()
	w.log.Info(cmd)
	cfg := LoadConfig()
	cfg.Status = StatusIdle
	cfg.LastExitKind = ExitClean
	cfg.LastExitAt = time.Now()
	w.saveConfig(&cfg)
}

// SetUpdating — приложение обновляется: пауза надзора на ttl.
func (w *Watchdog) SetUpdating(ttl time.Duration) {
	until := time.Now().Add(ttl)
	w.mu.Lock()
	w.updateUntil = until
	w.mu.Unlock()
	w.log.Info(fmt.Sprintf("app-updating pause=%s", ttl))
	cfg := LoadConfig()
	cfg.Status = StatusPaused
	cfg.WatchdogPausedUntil = until
	w.saveConfig(&cfg)
}

// SetLogoff — событие завершения сессии пользователя (SESSIONCHANGE).
func (w *Watchdog) SetLogoff() {
	w.mu.Lock()
	w.logoffSeen = true
	w.mu.Unlock()
}

// Hidden из команды set-hidden (само значение пишет вызывающий в реестр).
func (w *Watchdog) HelloPid() uint32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.helloPid
}

// Snapshot — живое состояние надзора для IPC status.
func (w *Watchdog) Snapshot() (pid uint32, version string, active bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.helloPid, w.helloVersion, w.hasHello
}

// EngineSnapshot — наблюдаемое состояние движка (картина прошлого тика,
// свежесть — до tickInterval) для IPC status.
func (w *Watchdog) EngineSnapshot() (installed, autostart, running bool, revives int, lastRevive time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.engineInstalled, w.engineAutostart, w.engineRunning, w.engineRevives, w.engineLastTry
}

// zapretSvcStart читает Start службы «zapret» из HKLM; ok=false — служба
// не установлена (движка нет вовсе).
func zapretSvcStart() (start uint32, ok bool) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\`+engineSvcName, registry.QUERY_VALUE)
	if err != nil {
		return 0, false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("Start")
	if err != nil {
		return 0, false
	}
	return uint32(v), true
}

// engineProcessAlive — процесс с именем образа name присутствует в системе.
func engineProcessAlive(name string) bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	entry := windows.ProcessEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), name) {
			return true
		}
	}
	return false
}

// engineTick — решение тика мониторинга движка (чистая функция, тестируемо
// без системы): сброс счётчика при живом движке/выключенном автозапуске/отсутствии
// службы, инкремент при мёртвом, попытка подъёма после порога — вне паузы
// установки и cooldown-окна.
func engineTick(installed, autostart, alive, updating bool, deadTicks int, nextTrial, now time.Time) (revive bool, deadTicksOut int) {
	if !installed || !autostart || alive {
		return false, 0
	}
	deadTicks++
	if updating || deadTicks < engineDeadTicksToRevive {
		return false, deadTicks
	}
	if !nextTrial.IsZero() && now.Before(nextTrial) {
		return false, deadTicks
	}
	return true, deadTicks
}

// engineWatch — тик мониторинга zapret: служба «zapret» установлена, её
// автозапуск включён, а процесса winws нет — подъём через net start (служба
// работает как SYSTEM, прав хватает). cycleStop (он про краш-цикл приложения)
// на движок не влияет; во время установки движка приложением (svcUpdating)
// подъём откладывается.
func (w *Watchdog) engineWatch() {
	start, installed := zapretSvcStart()
	alive := engineProcessAlive(engineImgName)

	now := time.Now()
	w.mu.Lock()
	w.engineInstalled = installed
	w.engineAutostart = installed && start != svcStartDisabled
	w.engineRunning = alive
	updating := now.Before(w.updateUntil)
	revive, deadTicks := engineTick(installed, w.engineAutostart, alive, updating, w.engineDeadTicks, w.engineNextTrial, now)
	w.engineDeadTicks = deadTicks
	w.mu.Unlock()

	if revive {
		w.reviveEngine(now)
	}
}

func (w *Watchdog) reviveEngine(now time.Time) {
	w.mu.Lock()
	w.engineNextTrial = now.Add(engineReviveCooldown)
	w.engineRevives++
	w.engineLastTry = now
	w.mu.Unlock()
	w.log.Info("engine dead — reviving via net start " + engineSvcName)
	if err := executil.HiddenCmd("net", "start", engineSvcName).Run(); err != nil {
		w.log.Error("engine revive failed: " + err.Error())
		return
	}
	w.log.Info("engine revived: net start " + engineSvcName + " ok")
}

func (w *Watchdog) tickOnce() {
	// Движок мониторится независимо от приложения: hello-гард действует
	// только на надзор за ZPUI.exe.
	w.engineWatch()
	w.appWatch()
}

func (w *Watchdog) appWatch() {
	w.mu.Lock()
	if !w.hasHello {
		w.mu.Unlock()
		return
	}
	if !w.nextAttempt.IsZero() && time.Now().Before(w.nextAttempt) {
		w.mu.Unlock()
		return
	}
	pid := w.helloPid
	if appProcessAlive(pid) {
		w.mu.Unlock()
		return
	}
	now := time.Now()
	switch {
	case w.exitFlag != exitFlagNone:
		w.clearHelloLocked()
		w.mu.Unlock()
		w.log.Info("app process gone after intentional exit")
		return
	case now.Before(w.updateUntil):
		w.mu.Unlock()
		return
	case w.logoffSeen:
		w.logoffSeen = false
		w.clearHelloLocked()
		w.mu.Unlock()
		cfg := LoadConfig()
		cfg.Status = StatusIdle
		cfg.LastExitKind = ExitLogoff
		cfg.LastExitAt = now
		w.saveConfig(&cfg)
		w.log.Info("session logoff — app exit recorded")
		return
	default:
		w.clearHelloLocked()
		w.mu.Unlock()
		w.handleCrash(pid, now)
	}
}

func (w *Watchdog) clearHelloLocked() {
	w.hasHello, w.helloPid, w.helloVersion, w.helloHidden = false, 0, "", false
}

// handleCrash — краш приложения: чистим движок, пишем реестр, рестарт с backoff.
func (w *Watchdog) handleCrash(pid uint32, now time.Time) {
	w.mu.Lock()
	updating := now.Before(w.updateUntil)
	w.mu.Unlock()
	if updating {
		// Идёт установка обновления: ни winws (установщику файлы трогать
		// нельзя), ни рестарт приложения (гоночная замена exe) — решение
		// откладывается до конца паузы.
		w.log.Info("app disappeared during update — watchdog paused, re-check after pause")
		w.mu.Lock()
		w.nextAttempt = w.updateUntil
		w.mu.Unlock()
		return
	}

	w.killImage(engineImgName)
	cfg := LoadConfig()
	cfg.LastExitKind = ExitCrash
	cfg.LastExitAt = now
	cfg.LastCrashReason = fmt.Sprintf("процесс %s (pid %d) исчез без app-exiting", appImageName, pid)
	cnt, wstart := nextCrashCount(cfg.CrashCount, cfg.CrashWindowStart, now)
	cfg.CrashCount, cfg.CrashWindowStart = cnt, wstart

	if crashLimitReached(cnt) {
		cfg.Status = StatusFailed
		w.saveConfig(&cfg)
		w.mu.Lock()
		w.cycleStop = true
		w.mu.Unlock()
		w.log.Error(fmt.Sprintf("crash #%d за час — cycle stop до ручного запуска приложения", cnt))
		return
	}

	if !cfg.Autostart {
		w.saveConfig(&cfg)
		w.log.Info("crash recorded (autostart disabled — no restart): " + cfg.LastCrashReason)
		return
	}

	pause := backoffFor(cnt)
	cfg.WatchdogPausedUntil = now.Add(pause)
	w.saveConfig(&cfg)
	w.mu.Lock()
	w.nextAttempt = now.Add(pause)
	w.mu.Unlock()
	w.log.Info(fmt.Sprintf("crash #%d (%s) — restart via scheduled task in %s", cnt, cfg.LastCrashReason, pause))
	time.AfterFunc(pause, w.restartViaTask)
}

func (w *Watchdog) restartViaTask() {
	w.mu.Lock()
	if w.hasHello || w.cycleStop {
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()
	cfg := LoadConfig()
	if cfg.AppPath == "" {
		w.log.Error("restart via task failed: app path unknown")
		return
	}
	if !RestoreTaskExists() {
		if err := CreateRestoreTask(cfg.AppPath, cfg.Hidden); err != nil {
			w.log.Error("restart via task: create restore task: " + err.Error())
			return
		}
	}
	if err := RunRestoreTask(); err != nil {
		w.log.Error("restart via scheduled task failed: " + err.Error())
		return
	}
	w.log.Info("app restarted via scheduled task")
}

// appProcessAlive проверяет наличие процесса приложения: сначала по PID,
// затем по имени образа (приложение могло перезапуститься с новым PID).
func appProcessAlive(pid uint32) bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	entry := windows.ProcessEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	found := false
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		if !strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), appImageName) {
			continue
		}
		if pid != 0 && entry.ProcessID == pid {
			return true
		}
		found = true
	}
	return found
}

func (w *Watchdog) killImage(name string) {
	if err := executil.HiddenCmd("taskkill", "/IM", name, "/F").Run(); err != nil {
		w.log.Debug("taskkill " + name + ": " + err.Error())
	}
}
