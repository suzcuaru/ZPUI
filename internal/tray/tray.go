package tray

import (
	"fmt"

	"zpui/internal/config"
	"zpui/internal/logger"

	"fyne.io/systray"
)

type Controller interface {
	ShowWindow()
	ToggleWindow()
	Quit()
}

type App struct {
	cfg        *config.Config
	log        *logger.Logger
	controller Controller
	version    string
	iconData   []byte

	mOpen  *systray.MenuItem
	mQuit  *systray.MenuItem
}

func New(cfg *config.Config, log *logger.Logger, controller Controller, version string, iconData []byte) *App {
	return &App{
		cfg:        cfg,
		log:        log,
		controller: controller,
		version:    version,
		iconData:   iconData,
	}
}

func (a *App) Run() {
	onReady := func() {
		systray.SetIcon(a.iconData)
		systray.SetTooltip(fmt.Sprintf("ZPUI v%s", a.version))
		systray.SetTitle("")

		a.mOpen = systray.AddMenuItem("Открыть ZPUI", "")
		systray.AddSeparator()
		a.mQuit = systray.AddMenuItem("Выход", "")

		systray.SetOnTapped(func() {
			a.controller.ToggleWindow()
		})

		go a.handleClicks()

		go func() {
			if !a.cfg.StartMinimized {
				a.controller.ShowWindow()
			}
		}()
	}

	systray.Run(onReady, func() {
		a.log.Info("tray", "Tray quit")
	})
}

func (a *App) handleClicks() {
	for {
		select {
		case <-a.mOpen.ClickedCh:
			a.log.Info("tray", "Show requested")
			a.controller.ShowWindow()
		case <-a.mQuit.ClickedCh:
			a.log.Info("tray", "Quit requested")
			a.controller.Quit()
			systray.Quit()
			return
		}
	}
}
