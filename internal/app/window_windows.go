package app

import (
	"context"
	"os"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) ShowWindow() {
	if a.ctx == nil {
		return
	}
	a.hidden = false
	wailsruntime.WindowShow(a.ctx)
}

func (a *App) ToggleWindow() {
	if a.ctx == nil {
		return
	}
	if a.hidden {
		a.ShowWindow()
	} else {
		a.hideWindow()
	}
}

func (a *App) hideWindow() {
	if a.ctx == nil {
		return
	}
	a.hidden = true
	wailsruntime.WindowHide(a.ctx)
}

func (a *App) Quit() {
	a.log.Info("app", "Force quit requested")
	a.mgr.StopAll()
	os.Remove(a.pidPath)
	os.Exit(0)
}

func (a *App) BeforeClose(ctx context.Context) bool {
	if a.cfg.CloseToTray {
		a.hideWindow()
		return true
	}
	return false
}
