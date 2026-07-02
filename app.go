package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/config"
	"zpui/internal/database"
	"zpui/internal/executil"
	"zpui/internal/logger"
	"zpui/internal/monitor"
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
	resourceCache     *resCacheData
	resourceCacheTime time.Time
	resourceCacheMu   sync.Mutex

	// Видимость окна (для tray toggle)
	windowVisible bool
	windowMu      sync.Mutex
}

// resCacheData — структура кэша доступности ресурсов.
type resCacheData struct {
	Default []map[string]interface{}
	User    []map[string]interface{}
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

// startup вызывается Wails при запуске приложения.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.log.Info("app", "Wails application started")

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

	if a.cfg.AutoStartProxy || a.cfg.LastProxyState {
		a.safeGo(func() {
			if err := a.proxy.Start(); err != nil {
				a.log.Error("proxy", "Auto-start proxy failed: "+err.Error())
			}
		})
	}

	if a.cfg.LastZapretState && !needAutoStart {
		a.safeGo(func() {
			time.Sleep(1 * time.Second)
			if err := a.zapret.Start(); err != nil {
				a.log.Error("zapret", "Auto-start zapret failed: "+err.Error())
			}
		})
	}

	if a.cfg.LastXboxDnsState {
		a.safeGo(func() {
			xd := a.cfg.GetXboxDnsConfig()
			a.xboxDns.Configure(xd.PrimaryDNS, xd.SecondaryDNS)
			if err := a.xboxDns.Enable(); err != nil {
				a.log.Error("xboxdns", "Auto-start xbox DNS failed: "+err.Error())
			}
		})
	}

	if a.cfg.AutoUpdateCheck {
		a.safeGo(a.checkUpdatesOnStartup)
	}
}

// checkUpdatesOnStartup проверяет обновления ZPUI и zapret через 10с после старта.
// Эмитит Wails event "update:available" если найдено обновление.
func (a *App) checkUpdatesOnStartup() {
	time.Sleep(10 * time.Second)

	if remote, err := updater.FetchRemoteVersions(); err == nil {
		if remote.ZPUI != "" && remote.ZPUI != a.version {
			runtime.EventsEmit(a.ctx, "update:available", map[string]interface{}{
				"component": "ZPUI",
				"current":   a.version,
				"latest":    remote.ZPUI,
			})
			a.log.Info("updater", fmt.Sprintf("ZPUI update available: %s -> %s", a.version, remote.ZPUI))
		}
	}

	if info, err := a.zapret.CheckForUpdates(); err == nil && info != nil && info.UpdateNeeded {
		runtime.EventsEmit(a.ctx, "update:available", map[string]interface{}{
			"component": "zapret",
			"current":   info.CurrentVersion,
			"latest":    info.LatestVersion,
		})
		a.log.Info("updater", fmt.Sprintf("Zapret update available: %s -> %s", info.CurrentVersion, info.LatestVersion))
	}
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
func (a *App) shutdown(ctx context.Context) {
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
func (a *App) beforeClose(ctx context.Context) bool {
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
			case <-time.After(1 * time.Second):
				a.log.Warn("app", "Shutdown timeout reached, forcing exit")
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
	data := a.resourceCache
	a.resourceCacheMu.Unlock()

	total := 0
	ok := 0
	for _, r := range data.Default {
		total++
		if isOk, _ := r["ok"].(bool); isOk {
			ok++
		}
	}
	if total == 0 {
		return -1
	}
	return ok * 100 / total
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
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cleanOldSnapshots(24 * time.Hour)
			cleanOldConnections(7 * 24 * time.Hour)
		case <-a.stopCh:
			return
		}
	}
}
