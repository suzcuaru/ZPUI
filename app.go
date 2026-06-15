package main

import (
	"context"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/config"
	"zpui/internal/logger"
	"zpui/internal/monitor"
	"zpui/internal/proxy"
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
	version string

	// Канал готовности контекста (для tray, который ждёт пока Wails запустится)
	ctxReady chan struct{}

	// stopCh закрывается в shutdown() — сигнал горутинам завершиться
	stopCh chan struct{}

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
	version string,
) *App {
	return &App{
		cfg:      cfg,
		log:      logMgr,
		zapret:   zapretMgr,
		proxy:    proxySrv,
		monitor:  trafficMon,
		version:  version,
		ctxReady: make(chan struct{}),
		stopCh:   make(chan struct{}),
	}
}

// startup вызывается Wails при запуске приложения.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.log.Info("app", "Wails application started")

	// Сигнализируем tray о готовности контекста
	close(a.ctxReady)

	// Запускаем фоновые воркеры (трекинг устройств, снапшоты, ротация)
	go a.startDeviceTracker()
	go a.startTrafficSnapshots()
	go a.startDataRotation()

	// Автозапуск прокси
	if a.cfg.Proxy.AutoStart {
		go func() {
			if err := a.proxy.Start(); err != nil {
				a.log.Error("proxy", "Auto-start proxy failed: "+err.Error())
			}
		}()
	}
}

// shutdown вызывается Wails при завершении приложения.
func (a *App) shutdown(ctx context.Context) {
	a.log.Info("app", "Shutting down...")

	// Сигнал всем фоновым горутинам завершиться (чтобы не писали в БД после database.Close())
	close(a.stopCh)

	a.proxy.Stop()
	a.monitor.Stop()
	a.log.Info("app", "Shutdown complete")
}

// beforeClose вызывается при закрытии окна (X).
// Сворачивает окно в трей вместо завершения приложения.
// Для выхода используйте tray → Выход (вызывает runtime.Quit()).
func (a *App) beforeClose(ctx context.Context) bool {
	a.log.Info("app", "Window close requested — hiding to tray")
	a.windowMu.Lock()
	a.windowVisible = false
	a.windowMu.Unlock()
	runtime.WindowHide(ctx)
	return true
}

// Quit — принудительное завершение приложения (вызывается из tray).
func (a *App) Quit() {
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
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
