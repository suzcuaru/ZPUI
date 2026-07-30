package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/blockcheck"
	"zpui/internal/config"
	"zpui/internal/database"
	"zpui/internal/executil"
	"zpui/internal/isp"
	"zpui/internal/logger"
	"zpui/internal/monitor"
	"zpui/internal/notify"
	"zpui/internal/proxy"
	"zpui/internal/updater"
	"zpui/internal/xboxdns"
	"zpui/internal/zapret"
)

// App — главный контекст Wails-приложения.
// Все экспортируемые методы автоматически привязываются к frontend через Wails bindings.
type App struct {
	ctx     context.Context
	cfg     *config.Config
	log     *logger.Logger
	zapret  *zapret.Manager
	proxy   *proxy.SOCKS5Server
	monitor *monitor.TrafficMonitor
	xboxDns *xboxdns.Manager
	version string
	exeDir  string

	// Канал готовности контекста (для tray, который ждёт пока Wails запустится)
	ctxReady chan struct{}

	// stopCh закрывается в shutdown() — сигнал горутинам завершиться
	stopCh chan struct{}

	// shutdownDone закрывается после выполнения shutdown() — для Quit()
	shutdownDone chan struct{}

	// once защищает shutdown()/Quit() от повторного выполнения (panic на close)
	shutdownOnce sync.Once
	quitOnce     sync.Once

	// Кэш доступности ресурсов (для tray)
	resourceCache     *blockcheck.BulkReport
	resourceCacheTime time.Time
	resourceCacheMu   sync.Mutex

	// checkingNow = true when a resource check is in progress (auto or manual).
	// Frontend uses this to disable button and show "updating...".
	checkingNow   bool
	checkingNowMu sync.Mutex

	// prevResourceState tracks previous OK/!OK state per resource name.
	// Used to log only state CHANGES (not every failed check).
	prevResourceState   map[string]bool
	prevResourceStateMu sync.Mutex

	// Эталон: какие ресурсы заблокированы без запрета (для wizard)
	controlBaseline map[string]bool

	// Видимость окна (для tray toggle)
	windowVisible bool
	windowMu      sync.Mutex

	// startHidden — окно запускается скрытым (start_minimized или флаг --hidden)
	startHidden bool

	// ispMonitor следит за текущим оператором связи
	ispMonitor *isp.Monitor
}

// NewApp создаёт новый экземпляр приложения.
func NewApp(
	cfg *config.Config,
	logMgr *logger.Logger,
	zapretMgr *zapret.Manager,
	proxySrv *proxy.SOCKS5Server,
	trafficMon *monitor.TrafficMonitor,
	xboxDnsMgr *xboxdns.Manager,
	version string,
	exeDir string,
) *App {
	notify.SetLogger(logMgr)
	return &App{
		cfg:          cfg,
		log:          logMgr,
		zapret:       zapretMgr,
		proxy:        proxySrv,
		monitor:       trafficMon,
		xboxDns:       xboxDnsMgr,
		version:       version,
		exeDir:       exeDir,
		ctxReady:     make(chan struct{}),
		stopCh:       make(chan struct{}),
		shutdownDone: make(chan struct{}),
	}
}

// SetStartHidden управляет скрытым запуском окна (вызывается из main.go).
func (a *App) SetStartHidden(v bool) { a.startHidden = v }

// startup вызывается Wails при запуске приложения.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.log.Info("app", "Wails application started")

	// Синхронизируем флаг видимости окна с реальным стартовым состоянием
	a.windowMu.Lock()
	a.windowVisible = !a.startHidden
	a.windowMu.Unlock()

	// Initialize resource state map for change tracking
	a.prevResourceState = make(map[string]bool)

	// Ensure skip-resources.txt exists (shipped with app, but auto-create if missing).
	a.ensureSkipResourcesFile()

	// Flush boot log into the log database (captures startup crashes).
	a.log.FlushBootLog()

	// Hook logger errors to desktop notifications (if enabled by user).
	a.log.SetOnError(func(code, msg string) {
		if a.cfg.ShouldNotify("errors") {
			notify.Show("ZPUI \xd0\xbe\xd1\x88\xd0\xb8\xd0\xb1\xd0\xba\xd0\xb0", msg)
		}
	})

	// Hook zapret crash: считаем краши, запускаем восстановление службы.
	a.zapret.OnCrash = func() {
		count := a.cfg.GetServiceCrashCount() + 1
		a.cfg.SetServiceCrashCount(count)
		a.cfg.SetServiceLastCrash(time.Now().Unix())
		a.log.Warn("zapret", fmt.Sprintf("winws.exe crashed (%d)", count))

		if count > 5 {
			a.log.Error("zapret", "Слишком много крашей, восстановление отключено")
			if a.cfg.ShouldNotify("service_crash") {
				notify.Show("Служба Запрета", "Превышен лимит аварийных завершений. Восстановление отключено.")
			}
			return
		}
		// Exponential backoff: 1s, 2s, 4s, 8s, 16s (cap at 60s)
		backoff := time.Duration(1<<uint(count-1)) * time.Second
		if backoff > 60*time.Second {
			backoff = 60 * time.Second
		}
		a.log.Warn("zapret", fmt.Sprintf("waiting %v before recovery (crash #%d)", backoff, count))
		time.Sleep(backoff)
		// Восстановление (с UI-тостом и полной диагностикой) запускается единым
		// механизмом startZapretRecovery — он перезапустит службу и при успехе
		// сбросит счётчик крашей.
		a.startZapretRecovery(fmt.Sprintf("process_crash_%d", count))
	}

	close(a.ctxReady)

	a.safeGo(a.runStartupCleanup)

	// Recovery: если прошлое обновление zapret прервалось (крах) — восстановить.
	needAutoStart := a.checkAndRecoverZapret()

	a.safeGo(func() {
		time.Sleep(200 * time.Millisecond)
		disableMaximizeButton("ZPUI")
	})

	a.safeGo(a.startDeviceTracker)
	a.safeGo(a.startTrafficSnapshots)
	a.safeGo(a.startDataRotation)
	a.safeGo(a.startResourceMonitor)
	a.safeGo(a.startZapretHealthMonitor)

	a.startISPMonitor()
	a.checkZapretUpdateFlag()
	a.syncAutostartRegistry()
	a.safeGo(a.syncStrategiesForOperator)

	startedViaAutostart := a.startHidden
	if startedViaAutostart && (a.cfg.AutoStartProxy || a.cfg.LastProxyState) {
		a.safeGo(func() {
			if err := a.proxy.Start(); err != nil {
				a.log.Error("proxy", "Auto-start proxy failed: "+err.Error())
			}
		})
	}

	if startedViaAutostart && a.cfg.LastZapretState && !needAutoStart && !a.cfg.GetZapretSkipped() {
		a.safeGo(func() {
			time.Sleep(1 * time.Second)
			// Автозапуск тоже верифицирует живость и при неудаче запускает
			// восстановление (с логированием и десктоп-уведомлением).
			if err := a.zapretStartAndVerify(); err != nil {
				a.log.Warn("zapret", "Auto-start zapret failed, recovering: "+err.Error())
				a.startZapretRecovery("autostart_failed: " + err.Error())
			}
		})
	}

	if startedViaAutostart && a.cfg.AutoStartXboxDns {
		a.safeGo(func() {
			if err := a.applyDnsEnabled(true); err != nil {
				a.log.Error("dns", "Auto-start DNS resolver failed: "+err.Error())
			}
		})
	} else if a.cfg.XboxDns.Enabled {
		// Резолвер был включён ранее, но автозапуск выключен — восстанавливаем
		// DHCP (netsh-настройки персистентны) и сбрасываем флаг.
		if err := a.cfg.SetXboxDnsEnabled(false); err != nil {
			a.log.Warn("dns", "SetXboxDnsEnabled(false): "+err.Error())
		}
		a.safeGo(func() {
			if err := a.xboxDns.RestoreDHCP(); err != nil {
				a.log.Warn("dns", "RestoreDHCP on startup: "+err.Error())
			}
		})
	}

	if a.cfg.AutoUpdateCheck {
		a.safeGo(a.checkUpdatesOnStartup)
		a.safeGo(a.startUpdatePanelPolling)
	}
}

// checkUpdatesOnStartup проверяет обновления ZPUI и zapret через 10с после старта.
// Эмитит Wails event "update:available" если найдено обновление.
// Уведомления дедуплицируются: тост показывается один раз на каждую новую версию.
func (a *App) checkUpdatesOnStartup() {
	time.Sleep(10 * time.Second)

	// --- ZPUI ---
	remote, err := updater.FetchRemoteVersions()
	if err != nil {
		a.log.Warn("updater", "ZPUI update check failed: "+err.Error())
	} else if remote.ZPUI != "" && updater.IsNewer(a.version, remote.ZPUI) {
		last := a.cfg.GetLastNotifiedVersion("ZPUI")
		runtime.EventsEmit(a.ctx, "update:available", map[string]interface{}{
			"component": "ZPUI",
			"current":   a.version,
			"latest":    remote.ZPUI,
		})
		a.log.Info("updater", fmt.Sprintf("ZPUI update available: %s -> %s", a.version, remote.ZPUI))
		if last == remote.ZPUI {
			a.log.Info("updater", "ZPUI toast skipped: already notified about "+remote.ZPUI)
		} else if a.cfg.ShouldNotify("zpui_update") {
			lang := a.cfg.GetLanguage()
			notify.Show("ZPUI", tr(lang, "zpui_update", a.version, remote.ZPUI))
			a.cfg.SetLastNotifiedVersion("ZPUI", remote.ZPUI)
			a.log.Info("updater", "ZPUI update toast sent")
		} else {
			a.log.Info("updater", "ZPUI toast skipped: notifications disabled (notify_zpui_updates)")
		}
	}

	// --- Zapret ---
	if !a.cfg.GetZapretSkipped() {
		info, err := a.zapret.CheckForUpdates()
		if err != nil {
			a.log.Warn("updater", "Zapret update check failed: "+err.Error())
		} else if info != nil && info.UpdateNeeded {
			last := a.cfg.GetLastNotifiedVersion("zapret")
			runtime.EventsEmit(a.ctx, "update:available", map[string]interface{}{
				"component": "zapret",
				"current":   info.CurrentVersion,
				"latest":    info.LatestVersion,
			})
			a.log.Info("updater", fmt.Sprintf("Zapret update available: %s -> %s", info.CurrentVersion, info.LatestVersion))
			if last == info.LatestVersion {
				a.log.Info("updater", "Zapret toast skipped: already notified about "+info.LatestVersion)
			} else if a.cfg.ShouldNotify("zapret_update") {
				lang := a.cfg.GetLanguage()
				notify.Show("Zapret", tr(lang, "zapret_update", info.LatestVersion))
				a.cfg.SetLastNotifiedVersion("zapret", info.LatestVersion)
				a.log.Info("updater", "Zapret update toast sent")
			} else {
				a.log.Info("updater", "Zapret toast skipped: notifications disabled (notify_zapret_updates)")
			}
		}

		vr := a.zapret.VerifyFiles()
		if !vr.AllPresent {
			missing := []string{}
			for _, f := range vr.Files {
				if !f.Exists {
					missing = append(missing, f.Path)
				}
			}
			a.log.Warn("zapret", fmt.Sprintf("Missing files: %v — auto-recovering", missing))
			runtime.EventsEmit(a.ctx, "zapret:files-missing", map[string]interface{}{
				"missing": missing,
			})

			a.log.Info("zapret", "Auto-downloading missing zapret files...")
			runtime.EventsEmit(a.ctx, "zapret:recovering", map[string]interface{}{
				"missing_count": len(missing),
			})
			if err := a.zapret.DownloadAndInstall(nil); err != nil {
				a.log.Error("zapret", "Auto-recovery failed: "+err.Error())
				runtime.EventsEmit(a.ctx, "zapret:recovery_failed", map[string]interface{}{
					"error": err.Error(),
				})
			} else {
				a.log.Info("zapret", "Auto-recovery completed successfully")
				runtime.EventsEmit(a.ctx, "zapret:recovery_complete", nil)
			}
		}
	}
}

func (a *App) startResourceMonitor() {
	time.Sleep(10 * time.Second)

	// Первая проверка сразу при запуске
	a.doResourceCheckAndSave()

	for {
		interval := time.Duration(a.cfg.GetResourceCheckInterval()) * time.Minute
		next := time.Now().Truncate(interval).Add(interval)
		wait := time.Until(next)
		if wait <= 0 {
			wait = interval
		}

		select {
		case <-a.stopCh:
			return
		case <-time.After(wait):
			a.doResourceCheckAndSave()
		}
	}
}

// doResourceCheckAndSave performs a full resource check (bypassing cache),
// saves snapshots to DB, and logs results. Used by both auto-monitor
// and manual refresh.
func (a *App) doResourceCheckAndSave() {
	a.checkingNowMu.Lock()
	a.checkingNow = true
	a.checkingNowMu.Unlock()
	defer func() {
		a.checkingNowMu.Lock()
		a.checkingNow = false
		a.checkingNowMu.Unlock()
	}()

	report := a.getResourceStatusForced()
	if report == nil {
		return
	}

	all := append(report.Default, report.User...)
	if len(all) == 0 {
		return
	}

	var operatorKey string
	if op, _ := database.GetCurrentOperator(); op != nil {
		operatorKey = op.OperatorKey
	}
	now := time.Now()

	saveSet := func(typ string, res []blockcheck.BulkResult) {
		if len(res) == 0 {
			return
		}
		oks := 0
		var newlyFailed, recovered []string
		failedCount := 0
		for _, r := range res {
			key := typ + ":" + r.Name
			a.prevResourceStateMu.Lock()
			prevOK, wasKnown := a.prevResourceState[key]
			a.prevResourceStateMu.Unlock()

			if r.OK {
				oks++
				if wasKnown && !prevOK {
					recovered = append(recovered, r.Name)
				}
			} else {
				failedCount++
				if !wasKnown || prevOK {
					newlyFailed = append(newlyFailed, r.Name)
				}
			}

			a.prevResourceStateMu.Lock()
			a.prevResourceState[key] = r.OK
			a.prevResourceStateMu.Unlock()

			if err := database.InsertResourceAvailability(&database.ResourceAvailability{
				Timestamp:   now,
				OperatorKey: operatorKey,
				Host:        r.Name,
				Type:        typ,
				Ok:          r.OK,
				Verdict:     r.Verdict,
				LatencyMs:   int(r.LatencyMs),
			}); err != nil {
				a.log.Warn("availability", "resource insert failed: "+err.Error())
			}
		}
		pct := 0
		if len(res) > 0 {
			pct = oks * 100 / len(res)
		}
		database.InsertAvailabilitySnapshot(&database.AvailabilityRecord{
			Timestamp:      time.Now(),
			Type:           typ,
			TotalResources: len(res),
			OKResources:    oks,
			Pct:            float64(pct),
		})
		// Only log summary of state changes (not per-resource)
		if len(newlyFailed) > 0 || len(recovered) > 0 {
			a.log.Info("availability", fmt.Sprintf("[%s] check: %d/%d ok (%d%%) — changed: -%d/+%d", typ, oks, len(res), pct, len(newlyFailed), len(recovered)))
		}
	}
	saveSet("standard", report.Default)
	saveSet("user", report.User)
}

// getResourceStatusForced does a force check (bypasses cache) and returns the raw report.
func (a *App) getResourceStatusForced() *blockcheck.BulkReport {
	defaultTargets, _ := blockcheck.ReadTargets(blockcheck.DefaultTargetsPath(a.cfg.GetZapretPath()))

	var userTargets []blockcheck.BulkTarget
	if body, err := os.ReadFile(filepath.Join(a.cfg.ListsDir(), "list-general-user.txt")); err == nil {
		userTargets = blockcheck.ParseTargets(string(body))
	}

	defaultTargets = a.filterSkipped(defaultTargets)
	userTargets = a.filterSkipped(userTargets)

	bc := a.cfg.GetBlockCheckConfig()
	checker := blockcheck.NewChecker(bc.CheckTCP, bc.CheckTLS, bc.CheckHTTP, bc.TimeoutSec)
	report := checker.BulkCheck(defaultTargets, userTargets)

	now := time.Now()
	a.resourceCacheMu.Lock()
	a.resourceCache = report
	a.resourceCacheTime = now
	a.resourceCacheMu.Unlock()

	return report
}

// IsCheckingNow returns whether a resource check is currently in progress.
func (a *App) IsCheckingNow() bool {
	a.checkingNowMu.Lock()
	defer a.checkingNowMu.Unlock()
	return a.checkingNow
}

// safeGo запускает функцию в горутине с защитой от panic.
// Panic логируется через Error и попадает в errors/ срез.
func (a *App) safeGo(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				a.log.Error("app", fmt.Sprintf("PANIC (goroutine): %v\n%s", r, debug.Stack()))
			}
		}()
		fn()
	}()
}

// checkAndRecoverZapret проверяет, есть ли в базе невостановленный слепок
// состояния zapret (признак прерванного обновления). Если zapret не работает —
// запускает восстановление в фоновой горутине и возвращает true (автозапуск не нужен).
func (a *App) checkAndRecoverZapret() bool {
	data, err := database.GetZapretBackup()
	if err != nil || data == "" {
		return false
	}

	svc := a.zapret.GetServiceStatus()
	if svc.Running {
		// zapret работает — прошлое обновление завершилось нормально
		database.DeleteZapretBackup()
		return false
	}

	a.log.Warn("app", "Обнаружено прерванное обновление zapret — восстановление состояния")
	var snap zapret.BackupSnapshot
	if err := json.Unmarshal([]byte(data), &snap); err != nil {
		a.log.Error("app", "Чтение backup не удалось: "+err.Error())
		database.DeleteZapretBackup()
		return false
	}

	a.safeGo(func() {
		time.Sleep(1 * time.Second)
		a.zapret.RestoreState(&snap)
		database.DeleteZapretBackup()
		a.log.Info("app", "Состояние zapret восстановлено после прерванного обновления")
	})
	return true
}

// shutdown вызывается Wails при завершении приложения.
func (a *App) Shutdown(ctx context.Context) {
	a.shutdownOnce.Do(func() {
		a.log.Info("app", "Shutting down...")

		close(a.stopCh)

		if a.ispMonitor != nil {
			a.ispMonitor.Stop()
		}

		a.proxy.Stop()
		a.monitor.Stop()

		// Сначала восстанавливаем системный DNS через netsh (БЛОКИРУЮЩИЙ).
		// netsh-настройки персистентны — если не восстановить, система
		// останется с кастомным DNS после выхода.
		if a.xboxDns.IsEnabled() {
			a.log.Info("app", "Restoring system DNS before shutdown...")
			done := make(chan struct{}, 1)
			go func() {
				if err := a.xboxDns.Disable(); err != nil {
					a.log.Error("app", "xboxDns.Disable error: "+err.Error())
				}
				close(done)
			}()
			select {
			case <-done:
				a.log.Info("app", "System DNS restored")
			case <-time.After(10 * time.Second):
				a.log.Error("app", "xboxDns.Disable timed out after 10s — network may be broken")
			}
		}

		go executil.HiddenCmd("taskkill", "/IM", "selfupdate.exe", "/F").Run()
		go executil.HiddenCmd("taskkill", "/IM", "report.exe", "/F").Run()
		go executil.HiddenCmd("taskkill", "/IM", "winws.exe", "/F").Run()

		a.log.Info("app", "Shutdown complete")
		close(a.shutdownDone)
	})
}

// beforeClose вызывается при закрытии окна (X).
// Если включён close-to-tray — сворачивает окно в трей вместо выхода.
// Для выхода используйте tray → Выход.
func (a *App) BeforeClose(ctx context.Context) bool {
	if a.cfg.GetCloseToTray() {
		a.log.Info("app", "Window close requested — hiding to tray")
		a.windowMu.Lock()
		a.windowVisible = false
		a.windowMu.Unlock()
		runtime.WindowHide(ctx)
		return true
	}
	a.log.Info("app", "Window close requested — quitting")
	go a.Quit()
	return true
}

// Quit — принудительное завершение приложения (вызывается из tray и при закрытии окна).
func (a *App) Quit() {
	a.quitOnce.Do(func() {
		a.log.Info("app", "Quit requested — terminating process")
		if a.ctx != nil {
			runtime.Quit(a.ctx)
			// Ждём shutdown с таймаутом, чтобы не зависнуть, если Wails
			// не доходит до OnShutdown при скрытом в трей окне.
			select {
			case <-a.shutdownDone:
			case <-time.After(3 * time.Second):
				// Normal when window hidden in tray - Wails is slow to process quit.
				// Force exit is fine, not an error.
			}
		}
		// Гарантированно завершаем процесс (убивает горутину трея и фоновые задачи)
		os.Exit(0)
	})
}

// ShowWindow — показать окно (из tray).
func (a *App) ShowWindow() {
	a.windowMu.Lock()
	a.windowVisible = true
	a.windowMu.Unlock()
	if a.ctx != nil {
		runtime.WindowShow(a.ctx)
	}
}

// HideWindow — скрыть окно (из tray).
func (a *App) HideWindow() {
	a.windowMu.Lock()
	a.windowVisible = false
	a.windowMu.Unlock()
	if a.ctx != nil {
		runtime.WindowHide(a.ctx)
	}
}

// ToggleWindow — переключить видимость окна (из tray).
func (a *App) ToggleWindow() {
	a.windowMu.Lock()
	visible := a.windowVisible
	a.windowMu.Unlock()
	if visible {
		a.HideWindow()
	} else {
		a.ShowWindow()
	}
}

// GetCachedResourcePercent — процент доступности ресурсов для tray.
func (a *App) GetCachedResourcePercent() int {
	a.resourceCacheMu.Lock()
	if a.resourceCache == nil {
		a.resourceCacheMu.Unlock()
		return -1
	}
	report := a.resourceCache
	a.resourceCacheMu.Unlock()

	total := 0
	oks := 0
	for _, r := range report.Default {
		total++
		if r.OK {
			oks++
		}
	}
	if total == 0 {
		return -1
	}
	return oks * 100 / total
}

// Restart перезапускает приложение (новый процесс, завершение текущего).
func (a *App) Restart() {
	a.log.Info("app", "Restart requested — spawning new process")
	exe, err := os.Executable()
	if err != nil {
		a.log.Error("app", "Restart: os.Executable() failed: "+err.Error())
		return
	}
	psCmd := fmt.Sprintf("Start-Sleep -Seconds 2; Start-Process '%s'", exe)
	if err := exec.Command("powershell", "-NoProfile", "-Command", psCmd).Start(); err != nil {
		a.log.Error("app", "Restart: failed to start new process: "+err.Error())
		return
	}
	a.Quit()
}

// GetCachedUserPercent — процент доступности пользовательских ресурсов для tray.
func (a *App) GetCachedUserPercent() int {
	a.resourceCacheMu.Lock()
	if a.resourceCache == nil {
		a.resourceCacheMu.Unlock()
		return -1
	}
	report := a.resourceCache
	a.resourceCacheMu.Unlock()
	total := len(report.User)
	if total == 0 {
		return -1
	}
	oks := 0
	for _, r := range report.User {
		if r.OK {
			oks++
		}
	}
	return oks * 100 / total
}

// startTrafficSnapshots — периодическое сохранение снапшотов трафика (каждые 5с).
func (a *App) startTrafficSnapshots() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			stats := a.monitor.GetCurrentStats()
			a.saveTrafficSnapshot(
				stats.DownloadSpeed,
				stats.UploadSpeed,
				int64(stats.DownloadBytes),
				int64(stats.UploadBytes),
				len(a.proxy.GetConnections()),
			)
		case <-a.stopCh:
			return
		}
	}
}

// startDataRotation — ротация старых данных (каждый час).
func (a *App) startDataRotation() {
	cleanOldSnapshots(24 * time.Hour)
	cleanOldConnections(7 * 24 * time.Hour)
	database.CleanOldAvailability(7 * 24 * time.Hour)
	database.CleanOldResourceAvailability(7 * 24 * time.Hour)

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	if err := database.RollupResourceDaily(yesterday); err != nil {
		a.log.Warn("availability", "daily rollup failed: "+err.Error())
	}

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cleanOldSnapshots(24 * time.Hour)
			cleanOldConnections(7 * 24 * time.Hour)
			database.CleanOldAvailability(7 * 24 * time.Hour)
			database.CleanOldResourceAvailability(7 * 24 * time.Hour)

			y := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
			if err := database.RollupResourceDaily(y); err != nil {
				a.log.Warn("availability", "daily rollup failed: "+err.Error())
			}
		case <-a.stopCh:
			return
		}
	}
}

// defaultSkipContent is the default content for skip-resources.txt.
// Used when the file is missing and needs to be created.
const defaultSkipContent = `# skip-resources.txt - resources excluded from availability checks.
#
# These domains are always down (dead CDN, retired subdomains, etc) so
# there is no point in checking them. Edit this file manually to add/remove.
# One host per line. Lines starting with # are comments. Blank lines ignored.
# Subdomains are matched automatically: "google.com" excludes "drive.google.com".

# Cloudflare service/test domains (always unavailable)
cloudflareapps.com
cloudflarebolt.com
cloudflareclient.com
cloudflarepartners.com
cloudflareresolve.com
cloudflaressl.com
cloudflarestatus.com
cloudflarestorage.com
cloudflaretest.com

# Cloudfront CDN
cloudfront.net

# Discord service subdomains
discord.dev
discord.media
discord.status
discord-activities.com
discordactivities.com
discordapp.net
discordpartygames.com

# Other service/unavailable
localizeapi.com
live-video.net

# PornHub CDN - always unavailable in RU
phncdn.com
pix-cdn77.phncdn.com
winhanced.com
`

// ensureSkipResourcesFile creates skip-resources.txt next to config.json
// if it does not exist yet. The file is pre-populated with a list of
// known always-down resources. User can edit it manually afterwards.
func (a *App) ensureSkipResourcesFile() {
	path := a.cfg.GetSkipResourcesFilePath()
	if _, err := os.Stat(path); err == nil {
		return // file exists, do not overwrite
	}
	if err := os.WriteFile(path, []byte(defaultSkipContent), 0644); err != nil {
		a.log.Warn("app", "Failed to create skip-resources.txt: "+err.Error())
		return
	}
	a.log.Info("app", "Created skip-resources.txt with default exclusions")
}

// startZapretHealthMonitor периодически проверяет, жива ли служба zapret.
// OnCrash срабатывает только для дочернего процесса (режим прямого запуска);
// когда zapret работает как служба Windows и падает, OnCrash не вызывается.
// Этот монитор заполняет пробел: если служба была RUNNING, а стала STOPPED
// не через Stop() — запускает восстановление.
func (a *App) startZapretHealthMonitor() {
	time.Sleep(15 * time.Second)
	wasRunning := a.zapret.GetStatus() == zapret.StatusRunning

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			nowRunning := a.zapret.GetStatus() == zapret.StatusRunning
			if wasRunning && !nowRunning && !a.zapret.StopRequested() && !a.isRecoveryRunning() {
				a.log.Warn("zapret", "health monitor: service stopped unexpectedly — recovering")
				a.startZapretRecovery("service_stopped_health")
			}
			wasRunning = nowRunning
		}
	}
}

// startISPMonitor запускает фоновый мониторинг текущего оператора связи.
// При смене оператора пушит Wails-событие "operator:changed".
func (a *App) startISPMonitor() {
	a.ispMonitor = isp.NewMonitor(a.log.Info, 30*time.Second, func(change isp.OperatorChange) {
		if change.OldKey != "" {
			runtime.EventsEmit(a.ctx, "operator:changed", map[string]interface{}{
				"old_key":  change.OldKey,
				"old_name": change.OldName,
				"new_key":  change.NewKey,
				"new_name": change.NewName,
			})
		}

		saved, _ := database.GetOperatorStrategy(change.NewKey)
		if saved != "" {
			a.log.Info("isp", "auto-switching strategy for new operator: "+saved)
			if err := a.zapret.SetStrategy(saved); err != nil {
				a.log.Error("isp", "auto-switch strategy failed: "+err.Error())
			} else {
				database.SetCurrentOperator(change.NewKey, change.NewName, saved)
				runtime.EventsEmit(a.ctx, "strategy:changed", map[string]interface{}{
					"strategy": saved,
					"operator": change.NewName,
				})
			}
		} else if change.OldKey != "" {
			a.log.Info("isp", "no saved strategy for operator: "+change.NewName+", auto-select needed")
			runtime.EventsEmit(a.ctx, "operator:auto_select_needed", map[string]interface{}{
				"operator_key":  change.NewKey,
				"operator_name": change.NewName,
			})
		}

		a.syncStrategiesForOperator()
	})
	a.safeGo(a.ispMonitor.Start)
}

// syncStrategiesForOperator перечисляет все стратегии из папки zapret и
// синхронизирует их в БД для текущего оператора. Новые стратегии добавляются,
// существующие сохраняют свои данные. Текущая отмечается как активная.
func (a *App) syncStrategiesForOperator() {
	op, err := database.GetCurrentOperator()
	if err != nil || op == nil || op.OperatorKey == "" {
		a.log.Info("strategies", "operator not yet detected, skipping strategy sync")
		return
	}

	strategies := a.zapret.ListStrategies()
	if len(strategies) == 0 {
		a.log.Info("strategies", "no strategies found in folder, skipping sync")
		return
	}

	current := a.cfg.GetCurrentStrategy()
	for _, s := range strategies {
		if err := database.EnsureOperatorStrategy(op.OperatorKey, s.Filename, s.Name); err != nil {
			a.log.Error("strategies", "ensure strategy "+s.Filename+": "+err.Error())
		}
	}

	if current != "" {
		if err := database.MarkActiveStrategy(op.OperatorKey, current, "startup"); err != nil {
			a.log.Error("strategies", "mark active "+current+": "+err.Error())
		}
	}

	a.log.Info("strategies", fmt.Sprintf("synced %d strategies for operator %s", len(strategies), op.OperatorKey))
}

// checkZapretUpdateFlag проверяет флаг zapret_just_updated в БД и пушит
// событие для показа модалки автоподбора.
func (a *App) checkZapretUpdateFlag() {
	if database.GetZapretJustUpdated() {
		database.SetZapretJustUpdated(false)
		op, _ := database.GetCurrentOperator()
		opName := ""
		if op != nil {
			opName = op.OperatorName
		}
		a.log.Info("app", "zapret was just updated — triggering auto-select modal")
		runtime.EventsEmit(a.ctx, "zapret:needs_autoselect", map[string]interface{}{
			"operator_name": opName,
		})
	}
}
