package tray

import (
	"fmt"
	"time"

	"zpui/internal/config"
	"zpui/internal/logger"
	"zpui/internal/proxy"
	"zpui/internal/xboxdns"
	"zpui/internal/zapret"

	"fyne.io/systray"
)

// Controller — интерфейс для управления окном и приложением.
// Реализуется main.App (Wails runtime).
type Controller interface {
	GetCachedResourcePercent() int
	GetCachedUserPercent() int
	ToggleWindow()
	ShowWindow()
	Quit()
	Restart()
}

type App struct {
	cfg        *config.Config
	log        *logger.Logger
	zapret     *zapret.Manager
	proxy      *proxy.SOCKS5Server
	xboxDns    *xboxdns.Manager
	controller Controller
	version    string
	iconData   []byte

	mOpen         *systray.MenuItem
	mStrategyInfo *systray.MenuItem
	mStdPctInfo   *systray.MenuItem
	mUserPctInfo  *systray.MenuItem
	mRestart      *systray.MenuItem
	mQuit         *systray.MenuItem
}

func New(
	cfg *config.Config,
	log *logger.Logger,
	zapretMgr *zapret.Manager,
	proxySrv *proxy.SOCKS5Server,
	xboxDnsMgr *xboxdns.Manager,
	controller Controller,
	version string,
	iconData []byte,
) *App {
	return &App{
		cfg:        cfg,
		log:        log,
		zapret:     zapretMgr,
		proxy:      proxySrv,
		xboxDns:    xboxDnsMgr,
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
		systray.AddSeparator()

		a.mStrategyInfo = systray.AddMenuItem("Стратегия: ...", "")
		a.mStdPctInfo = systray.AddMenuItem("Доступность: ...", "")
		a.mUserPctInfo = systray.AddMenuItem("Пользовательские: ...", "")
		systray.AddSeparator()

		a.mRestart = systray.AddMenuItem("Перезапустить", "")
		a.mQuit = systray.AddMenuItem("Выход", "")

		systray.SetOnTapped(func() {
			a.controller.ToggleWindow()
		})

		go a.handleClicks()
		go a.updateInfoLoop()
	}

	systray.Run(onReady, func() {
		a.log.Info("tray", "Tray quit")
		a.proxy.Stop()
	})
	return nil
}

func (a *App) updateInfoLoop() {
	for {
		strategy := a.cfg.GetCurrentStrategy()
		stdPct := a.controller.GetCachedResourcePercent()
		userPct := a.controller.GetCachedUserPercent()

		if strategy == "" {
			strategy = "не выбрана"
		}
		a.mStrategyInfo.SetTitle(fmt.Sprintf("Стратегия: %s", strategy))

		if stdPct >= 0 {
			a.mStdPctInfo.SetTitle(fmt.Sprintf("Доступность: %d%%", stdPct))
		} else {
			a.mStdPctInfo.SetTitle("Доступность: —")
		}

		if userPct >= 0 {
			a.mUserPctInfo.SetTitle(fmt.Sprintf("Пользовательские: %d%%", userPct))
		} else {
			a.mUserPctInfo.SetTitle("Пользовательские: —")
		}

		time.Sleep(5 * time.Second)
	}
}

func (a *App) handleClicks() {
	for {
		select {
		case <-a.mOpen.ClickedCh:
			a.log.Info("tray", "Show window requested")
			a.controller.ShowWindow()
		case <-a.mRestart.ClickedCh:
			a.log.Info("tray", "Application restart requested")
			a.controller.Restart()
			return
		case <-a.mQuit.ClickedCh:
			a.log.Info("tray", "Quit requested from tray")
			go a.controller.Quit()
			return
		}
	}
}


