package tray

import (
	"fmt"
	"time"

	"zpui/internal/config"
	"zpui/internal/logger"
	"zpui/internal/proxy"
	"zpui/internal/zapret"

	"fyne.io/systray"
)

// Controller — интерфейс для управления окном и приложением.
// Реализуется main.App (Wails runtime).
type Controller interface {
	GetCachedResourcePercent() int
	ToggleWindow()
	ShowWindow()
	Quit()
}

type App struct {
	cfg        *config.Config
	log        *logger.Logger
	zapret     *zapret.Manager
	proxy      *proxy.SOCKS5Server
	controller Controller
	version    string
	iconData   []byte

	mOpen     *systray.MenuItem
	mRestart  *systray.MenuItem
	mQuit     *systray.MenuItem
}

func New(
	cfg *config.Config,
	log *logger.Logger,
	zapretMgr *zapret.Manager,
	proxySrv *proxy.SOCKS5Server,
	controller Controller,
	version string,
	iconData []byte,
) *App {
	return &App{
		cfg:        cfg,
		log:        log,
		zapret:     zapretMgr,
		proxy:      proxySrv,
		controller: controller,
		version:    version,
		iconData:   iconData,
	}
}

func (a *App) Run() error {
	onReady := func() {
		systray.SetIcon(a.iconData)
		systray.SetTooltip(fmt.Sprintf("ZPUI v%s", a.version))

		a.mOpen = systray.AddMenuItem("Открыть ZPUI", "")
		a.mRestart = systray.AddMenuItem("Перезапустить", "")
		systray.AddSeparator()
		a.mQuit = systray.AddMenuItem("Выход", "")

		systray.SetOnTapped(func() {
			a.controller.ToggleWindow()
		})

		go a.handleClicks()
	}

	systray.Run(onReady, func() {
		a.log.Info("tray", "Tray quit")
		a.proxy.Stop()
	})
	return nil
}

func (a *App) handleClicks() {
	for {
		select {
		case <-a.mOpen.ClickedCh:
			a.log.Info("tray", "Show window requested")
			a.controller.ShowWindow()
		case <-a.mRestart.ClickedCh:
			a.log.Info("tray", "Restart requested")
			go func() {
				restoreProxy := a.cfg.LastProxyState
				restoreZapret := a.cfg.LastZapretState

				a.proxy.Stop()
				a.zapret.Stop()
				for i := 0; i < 15; i++ {
					if a.zapret.GetStatus() != "running" && !a.proxy.IsRunning() { break }
					time.Sleep(500 * time.Millisecond)
				}
				if restoreProxy {
					a.proxy.Start()
				}
				if restoreZapret {
					a.zapret.Start()
				}
			}()
		case <-a.mQuit.ClickedCh:
			a.log.Info("tray", "Quit requested from tray")
			go a.controller.Quit()
			return
		}
	}
}


