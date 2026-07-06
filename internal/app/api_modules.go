package app

import (
	"fmt"
	"os/exec"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) GetModules() []map[string]interface{} {
	mods := a.mgr.Discover()
	out := make([]map[string]interface{}, 0, len(mods))
	for _, dm := range mods {
		s := a.mgr.Status(dm)
		s["disabled"] = a.cfg.IsModDisabled(dm.Manifest.ID)
		out = append(out, s)
	}
	return out
}

func (a *App) GetModuleStatus(id string) map[string]interface{} {
	for _, dm := range a.mgr.Discover() {
		if dm.Manifest.ID == id {
			s := a.mgr.Status(dm)
			s["disabled"] = a.cfg.IsModDisabled(id)
			return s
		}
	}
	return errResp(fmt.Sprintf("module %q not found", id))
}

func (a *App) StartModule(id string) map[string]interface{} {
	for _, dm := range a.mgr.Discover() {
		if dm.Manifest.ID == id {
			if !dm.EntryOK {
				return errResp("entry exe не найден")
			}
			if err := a.mgr.Start(dm.Manifest); err != nil {
				return errResp(err.Error())
			}
			return okResp(map[string]interface{}{"state": string(a.mgr.StateOf(id))})
		}
	}
	return errResp(fmt.Sprintf("module %q not found", id))
}

func (a *App) StopModule(id string) map[string]interface{} {
	if err := a.mgr.Stop(id); err != nil {
		return errResp(err.Error())
	}
	return okResp(map[string]interface{}{"state": string(a.mgr.StateOf(id))})
}

func (a *App) RestartModule(id string) map[string]interface{} {
	for _, dm := range a.mgr.Discover() {
		if dm.Manifest.ID == id {
			if err := a.mgr.Restart(dm.Manifest); err != nil {
				return errResp(err.Error())
			}
			return okResp(map[string]interface{}{"state": string(a.mgr.StateOf(id))})
		}
	}
	return errResp(fmt.Sprintf("module %q not found", id))
}

func (a *App) ReloadModules() []map[string]interface{} {
	return a.GetModules()
}

func (a *App) SetModuleEnabled(id string, enabled bool) map[string]interface{} {
	if err := a.cfg.SetModDisabled(id, !enabled); err != nil {
		return errResp(err.Error())
	}
	if !enabled {
		_ = a.mgr.Stop(id)
	}
	return okResp(map[string]interface{}{"id": id, "enabled": enabled})
}

func (a *App) OpenModulesFolder() map[string]interface{} {
	_ = exec.Command("explorer", a.mgr.RootDir()).Start()
	return okResp(nil)
}

func (a *App) OpenExternal(url string) map[string]interface{} {
	if a.ctx != nil {
		wailsruntime.BrowserOpenURL(a.ctx, url)
	}
	return okResp(nil)
}
