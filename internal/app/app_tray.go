package app

import (

	"github.com/wailsapp/wails/v2/pkg/runtime"

)

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

func (a *App) StartTrayUpdate(component string) {
	a.log.Info("updater", "Tray update requested: "+component)
	a.ShowWindow()
	runtime.EventsEmit(a.ctx, "update:open-modal", map[string]interface{}{
		"component": component,
	})
}

func (a *App) CloseApp() {
	a.keepEngineOnQuit.Store(true)
	a.Quit()
}
