package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/blockcheck"
	"zpui/internal/config"
	"zpui/internal/database"
	"zpui/internal/executil"
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
	return &App{
		cfg:           cfg,
		log:           logMgr,
		zapret:        zapretMgr,
		proxy:         proxySrv,
		monitor:       trafficMon,
		xboxDns:       xboxDnsMgr,
		version:       version,
		exeDir:        exeDir,
		ctxReady:      make(chan struct{}),
		stopCh:        make(chan struct{}),
		shutdownDone:  make(chan struct{}),
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

	// Hook logger errors to desktop notifications (if enabled by user).
	a.log.SetOnError(func(category, msg string) {
		if a.cfg.ShouldNotify("errors") {
			notify.Show("ZPUI \xd0\xbe\xd1\x88\xd0\xb8\xd0\xb1\xd0\xba\xd0\xb0", "["+category+"] "+msg)
		}
	})

	// Hook zapret crash to desktop notifications.
	a.zapret.OnCrash = func() {
		if a.cfg.ShouldNotify("service_crash") {
			notify.Show("Служба Запрета остановлена", "Процесс winws.exe завершился неожиданно. Проверьте стратегию и логи.")
		}
	}

	close(a.ctxReady)

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

	if a.cfg.AutoStartProxy || a.cfg.LastProxyState {
		a.safeGo(func() {
			if err := a.proxy.Start(); err != nil {
				a.log.Error("proxy", "Auto-start proxy failed: "+err.Error())
			}
		})
	}

	if a.cfg.LastZapretState && !needAutoStart && !a.cfg.GetZapretSkipped() {
		a.safeGo(func() {
			time.Sleep(1 * time.Second)
			if err := a.zapret.Start(); err != nil {
				a.log.Error("zapret", "Auto-start zapret failed: "+err.Error())
			}
		})
	}

	if a.cfg.AutoStartXboxDns {
		a.safeGo(func() {
			xd := a.cfg.GetXboxDnsConfig()
			a.xboxDns.Configure(xd.PrimaryDNS, xd.SecondaryDNS)
			if err := a.xboxDns.Enable(); err != nil {
				a.log.Error("xboxdns", "Auto-start xbox DNS failed: "+err.Error())
			}
		})
	} else if a.cfg.XboxDns.Enabled {
		// Автозапуск выключен, но DNS был включён ранее — восстанавливаем DHCP
		// и сбрасываем флаг, чтобы статус отражал реальное выключенное состояние.
		xd := a.cfg.GetXboxDnsConfig()
		xd.Enabled = false
		a.cfg.SetXboxDnsConfig(xd)
		a.safeGo(func() {
			if err := a.xboxDns.RestoreDHCP(); err != nil {
				a.log.Warn("xboxdns", "RestoreDHCP on startup: "+err.Error())
			}
		})
	}

	if a.cfg.AutoUpdateCheck {
		a.safeGo(a.checkUpdatesOnStartup)
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
		if last != remote.ZPUI {
			a.cfg.SetLastNotifiedVersion("ZPUI", remote.ZPUI)
			if a.cfg.ShouldNotify("zpui_update") {
				lang := a.cfg.GetLanguage()
				notify.Show("ZPUI", tr(lang, "zpui_update", a.version, remote.ZPUI))
			}
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
			if last != info.LatestVersion {
				a.cfg.SetLastNotifiedVersion("zapret", info.LatestVersion)
				if a.cfg.ShouldNotify("zapret_update") {
					lang := a.cfg.GetLanguage()
					notify.Show("Zapret", tr(lang, "zapret_update", info.CurrentVersion, info.LatestVersion))
				}
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
			a.log.Warn("zapret", fmt.Sprintf("Missing files: %v", missing))
			runtime.EventsEmit(a.ctx, "zapret:files-missing", map[string]interface{}{
				"missing": missing,
			})
			if a.cfg.ShouldNotify("missing_files") {
				lang := a.cfg.GetLanguage()
				notify.Show("Zapret", tr(lang, "missing_files", len(missing)))
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

	all := make([]blockcheck.BulkResult, 0, len(report.Default)+len(report.User))
	all = append(all, report.Default...)
	all = append(all, report.User...)
	if len(all) == 0 {
		return
	}

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

			// Update state
			a.prevResourceStateMu.Lock()
			a.prevResourceState[key] = r.OK
			a.prevResourceStateMu.Unlock()
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
		// Summary line (always)
		a.log.Info("availability", fmt.Sprintf("[%s] %d%% (%d/%d)", typ, pct, oks, len(res)))
		// Only log state CHANGES (not every failed resource)
		for _, name := range newlyFailed {
			a.log.Warn("availability", fmt.Sprintf("[%s] ✗ %s — now unavailable", typ, name))
		}
		for _, name := range recovered {
			a.log.Info("availability", fmt.Sprintf("[%s] ✓ %s — recovered", typ, name))
		}
	}
	saveSet("standard", report.Default)
	saveSet("user", report.User)

	oks := 0
	for _, r := range all {
		if r.OK {
			oks++
		}
	}
	pct := 0
	if len(all) > 0 {
		pct = oks * 100 / len(all)
	}
	a.log.Info("availability", fmt.Sprintf("[total] %d%% (%d/%d) — standard %d/%d, user %d/%d",
		pct, oks, len(all),
		countOK(report.Default), len(report.Default),
		countOK(report.User), len(report.User)))
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
				a.log.Error("app", fmt.Sprintf("PANIC (goroutine): %v", r))
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

		// Сигнал всем фоновым горутинам завершиться
		close(a.stopCh)

		// Быстрая остановка — не блокируем shutdown ожиданием netsh и taskkill
		a.proxy.Stop()
		a.monitor.Stop()

		// Xbox DNS (netsh) может быть медленным — запускаем в фоне с таймаутом
		if a.xboxDns.IsEnabled() {
			go func() {
				done := make(chan struct{})
				go func() {
					a.xboxDns.Disable()
					close(done)
				}()
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					a.log.Warn("app", "xboxDns.Disable timeout, skipped")
				}
			}()
		}

		// Гасим фоновые процессы — fire-and-forget
		go executil.HiddenCmd("taskkill", "/IM", "wizard.exe", "/F").Run()
		go executil.HiddenCmd("taskkill", "/IM", "autoselect.exe", "/F").Run()
		go executil.HiddenCmd("taskkill", "/IM", "selfupdate.exe", "/F").Run()
		go executil.HiddenCmd("taskkill", "/IM", "zapretupdate.exe", "/F").Run()
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
	// FIX: Include User resources in tray percentage
	for _, r := range report.User {
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
	// Очистка сразу при старте, затем каждый час
	cleanOldSnapshots(24 * time.Hour)
	cleanOldConnections(7 * 24 * time.Hour)
	database.CleanOldAvailability(24 * time.Hour)

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cleanOldSnapshots(24 * time.Hour)
			cleanOldConnections(7 * 24 * time.Hour)
			database.CleanOldAvailability(24 * time.Hour)
		case <-a.stopCh:
			return
		}
	}
}
// countOK returns number of OK results in a bulk result slice.
func countOK(res []blockcheck.BulkResult) int {
	n := 0
	for _, r := range res {
		if r.OK {
			n++
		}
	}
	return n
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
