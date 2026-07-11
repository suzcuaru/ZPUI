package app

import (
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/zapret2"
)

func (a *App) GetEngineStatus() map[string]interface{} {
	engine := a.cfg.GetEngineVersion()

	v1Status := a.zapret.GetServiceStatus()
	v2Status := a.zapret2.GetServiceStatus()

	return map[string]interface{}{
		"engine": engine,
		"v1": map[string]interface{}{
			"installed":     v1Status.Installed,
			"running":       v1Status.Running,
			"strategy":      v1Status.Strategy,
			"version":       a.zapret.GetVersion(),
			"pid":           v1Status.PID,
			"files_present": a.zapret.VerifyFiles().AllPresent,
		},
		"v2": map[string]interface{}{
			"installed":     v2Status.Installed,
			"running":       v2Status.Running,
			"strategy":      v2Status.Strategy,
			"version":       a.zapret2.GetVersion(),
			"pid":           v2Status.PID,
			"files_present": a.zapret2.VerifyFiles().AllPresent,
		},
	}
}

func (a *App) SwitchEngine(engine string) map[string]interface{} {
	if engine != "v1" && engine != "v2" {
		return errResp("engine must be 'v1' or 'v2'")
	}

	current := a.cfg.GetEngineVersion()
	if current == engine {
		return okResp()
	}

	a.log.Info("engine", "Switching from "+current+" to "+engine)

	if current == "v1" {
		a.log.Info("engine", "Stopping v1 service/process...")
		a.zapret.Stop()
		a.zapret.RemoveService()
	} else {
		a.log.Info("engine", "Stopping v2 service/process...")
		a.zapret2.Stop()
		a.zapret2.RemoveService()
	}

	time.Sleep(2 * time.Second)

	a.cfg.SetEngineVersion(engine)

	if engine == "v1" {
		a.log.Info("engine", "Starting v1...")
		if err := a.zapret.Start(); err != nil {
			return errResp("v1 start failed: " + err.Error())
		}
	} else {
		vr := a.zapret2.VerifyFiles()
		if !vr.AllPresent {
			return map[string]interface{}{
				"status":           "need_download",
				"engine":           "v2",
				"files_present":    false,
				"missing_files":    vr.Files,
			}
		}
		a.log.Info("engine", "Starting v2...")
		if err := a.zapret2.Start(); err != nil {
			return errResp("v2 start failed: " + err.Error())
		}
	}

	return map[string]interface{}{"status": "ok", "engine": engine}
}

func (a *App) DownloadZapret2() map[string]interface{} {
	go func() {
		if err := a.zapret2.DownloadAndInstall(nil); err != nil {
			a.log.Error("engine", "Zapret2 download failed: "+err.Error())
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "zapret2:download_error", map[string]interface{}{
					"error": err.Error(),
				})
			}
			return
		}
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "zapret2:downloaded", map[string]interface{}{
				"version": a.zapret2.GetVersion(),
			})
		}
	}()

	return okResp()
}

func (a *App) GetZapret2Strategies() map[string]interface{} {
	return map[string]interface{}{"strategies": a.zapret2.ListStrategies()}
}

func (a *App) SetStrategyV2(filename string) map[string]interface{} {
	if filename == "" {
		return errResp("filename required")
	}
	if err := a.zapret2.SetStrategy(filename); err != nil {
		return errResp(err.Error())
	}
	return map[string]interface{}{"status": "ok", "strategy": filename}
}

func (a *App) InstallZapret2Service(strategy string) map[string]interface{} {
	if strategy == "" {
		strategy = a.zapret2.GetCurrentStrategy()
	}
	res, err := a.zapret2.InstallServiceLogged(strategy)
	if err != nil {
		return errResp(err.Error())
	}
	return map[string]interface{}{
		"success":  res.Success,
		"version":  res.Version,
		"strategy": res.Strategy,
		"running":  res.Running,
		"errors":   res.Errors,
	}
}

func (a *App) RemoveZapret2Service() map[string]interface{} {
	a.zapret2.RemoveService()
	return okResp()
}

func (a *App) CheckZapret2Updates() interface{} {
	info, err := a.zapret2.CheckForUpdates()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return info
}

func (a *App) ApplyZapret2Update() map[string]interface{} {
	progress := make(chan zapret2.UpdateProgress, 20)
	go a.zapret2.PerformUpdate(progress)
	return okResp()
}

func (a *App) VerifyZapret2Files() map[string]interface{} {
	vr := a.zapret2.VerifyFiles()
	return map[string]interface{}{
		"all_present": vr.AllPresent,
		"version":     vr.Version,
		"files":       vr.Files,
	}
}