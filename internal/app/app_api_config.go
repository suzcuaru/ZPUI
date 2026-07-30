package app

import (
	"fmt"
	"os"
	"path/filepath"
	"zpui/internal/executil"
)

// ============================================================
// AUTOSTART
// ============================================================

func (a *App) GetAutostartStatus() map[string]interface{} {
	return map[string]interface{}{"enabled": a.cfg.AutoStart}
}

func (a *App) EnableAutostart() map[string]interface{} {
	a.log.Info("autostart", "EnableAutostart called")
	a.cfg.AutoStart = true
	if err := a.cfg.Save(); err != nil {
		a.log.Error("autostart", "Save error: "+err.Error())
	} else {
		a.log.Info("autostart", "Autostart config saved")
	}

	exePath := getExePath()
	if exePath == "" {
		a.log.Error("autostart", "Cannot get exe path")
	} else {
		executil.HiddenCmd("schtasks", "/create", "/tn", "ZPUI",
			"/tr", fmt.Sprintf(`"%s" --hidden`, exePath),
			"/sc", "ONLOGON", "/rl", "HIGHEST", "/f").Run()
		a.log.Info("autostart", "Scheduled task created (ONLOGON, HIGHEST)")

		executil.HiddenCmd("reg", "delete",
			`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
			"/v", "ZPUI", "/f").Output()
	}

	return map[string]interface{}{"status": "enabled"}
}

func (a *App) DisableAutostart() map[string]interface{} {
	a.log.Info("autostart", "DisableAutostart called")
	a.cfg.AutoStart = false
	if err := a.cfg.Save(); err != nil {
		a.log.Error("autostart", "Save error: "+err.Error())
	} else {
		a.log.Info("autostart", "Autostart disabled, config saved")
	}

	executil.HiddenCmd("reg", "delete",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
		"/v", "ZPUI", "/f").Run()
	a.log.Info("autostart", "Registry Run key removed")

	startupPath := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "ZPUI.lnk")
	os.Remove(startupPath)

	executil.HiddenCmd("schtasks", "/delete", "/tn", "ZPUI", "/f").Run()
	a.log.Info("autostart", "Legacy startup artifacts cleaned")

	return map[string]interface{}{"status": "disabled"}
}

// ============================================================
// CONFIG
// ============================================================

func (a *App) GetConfig() map[string]interface{} {
	return map[string]interface{}{
		"current_strategy":        a.cfg.GetCurrentStrategy(),
		"web_port":                a.cfg.Web.Port,
		"proxy":                   a.cfg.GetProxyConfig(),
		"xbox_dns":                a.cfg.GetXboxDnsConfig(),
		"autostart":               a.cfg.AutoStart,
		"auto_update_check":       a.cfg.AutoUpdateCheck,
		"theme":                   a.cfg.Theme,
		"language":                a.cfg.GetLanguage(),
		"first_run_done":          a.cfg.FirstRunDone,
		"zapret_skipped":          a.cfg.GetZapretSkipped(),
		"start_minimized":         a.cfg.StartMinimized,
		"close_to_tray":           a.cfg.GetCloseToTray(),
		"last_zapret_state":       a.cfg.LastZapretState,
		"last_proxy_state":        a.cfg.LastProxyState,
		"last_xbox_dns_state":     a.cfg.LastXboxDnsState,
		"auto_start_zapret":       a.cfg.AutoStartZapret,
		"auto_start_proxy":        a.cfg.AutoStartProxy,
		"auto_start_xbox_dns":     a.cfg.AutoStartXboxDns,
		"notifications_enabled":   a.cfg.GetNotificationsEnabled(),
		"notify_zpui_updates":     a.cfg.NotifyZPUIUpdates,
		"notify_zapret_updates":   a.cfg.NotifyZapretUpdates,
		"notify_missing_files":    a.cfg.NotifyMissingFiles,
		"notify_service_crash":    a.cfg.NotifyServiceCrash,
		"notify_resource_drop":    a.cfg.NotifyResourceDrop,
		"resource_drop_pct":       a.cfg.GetResourceDropPct(),
		"resource_check_interval": a.cfg.GetResourceCheckInterval(),
		"show_strategy_modal":     a.cfg.ShowStrategyModal,
		"notify_strategy_test":    a.cfg.NotifyStrategyTest,
		"logs":                    a.cfg.Logs,
	}
}

func (a *App) SetConfig(opts map[string]interface{}) map[string]interface{} {
	a.log.Info("config", "SetConfig called")
	if port, ok := opts["web_port"].(float64); ok {
		a.cfg.Web.Port = int(port)
	}
	if theme, ok := opts["theme"].(string); ok {
		a.cfg.SetTheme(theme)
	}
	if v, ok := opts["start_minimized"].(bool); ok {
		a.cfg.StartMinimized = v
	}
	if v, ok := opts["close_to_tray"].(bool); ok {
		a.cfg.CloseToTray = v
	}
	if v, ok := opts["auto_start_zapret"].(bool); ok {
		a.cfg.AutoStartZapret = v
	}
	if v, ok := opts["auto_start_proxy"].(bool); ok {
		a.cfg.AutoStartProxy = v
	}
	if v, ok := opts["auto_start_xbox_dns"].(bool); ok {
		a.cfg.AutoStartXboxDns = v
	}
	if v, ok := opts["language"].(string); ok {
		a.cfg.Language = v
	}
	if v, ok := opts["zapret_skipped"].(bool); ok {
		a.cfg.SetZapretSkipped(v)
	}
	if v, ok := opts["first_run_done"].(bool); ok {
		a.cfg.FirstRunDone = v
	}
	if v, ok := opts["notifications_enabled"].(bool); ok {
		a.cfg.SetNotificationsEnabled(v)
	}
	notifyFlags := map[string]bool{}
	for _, key := range []string{"notify_zpui_updates", "notify_zapret_updates", "notify_missing_files", "notify_service_crash", "notify_resource_drop"} {
		if v, ok := opts[key].(bool); ok {
			notifyFlags[key] = v
		}
	}
	if len(notifyFlags) > 0 {
		a.cfg.SetNotifyFlags(notifyFlags)
	}
	if v, ok := opts["resource_drop_pct"].(float64); ok {
		a.cfg.SetResourceDropPct(int(v))
	}
	if v, ok := opts["resource_check_interval"].(float64); ok {
		a.cfg.SetResourceCheckInterval(int(v))
	}
	if err := a.cfg.Save(); err != nil {
		a.log.Error("config", "Save error: "+err.Error())
	} else {
		a.log.Info("config", "Config saved to JSON")
	}
	return map[string]interface{}{"status": "ok"}
}

// syncAutostartRegistry reconciles config.json autostart with Windows Registry.
// Config.json is the source of truth. Registry is corrected to match.
func (a *App) syncAutostartRegistry() {
	_, err := executil.HiddenCmd("schtasks", "/query", "/tn", "ZPUI", "/fo", "LIST").Output()
	taskExists := err == nil

	executil.HiddenCmd("reg", "delete",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
		"/v", "ZPUI", "/f").Output()

	if a.cfg.AutoStart && !taskExists {
		a.log.Info("autostart", "Config says ON but task missing — creating")
		exePath := getExePath()
		if exePath != "" {
			executil.HiddenCmd("schtasks", "/create", "/tn", "ZPUI",
				"/tr", fmt.Sprintf(`"%s" --hidden`, exePath),
				"/sc", "ONLOGON", "/rl", "HIGHEST", "/f").Run()
			a.log.Info("autostart", "Scheduled task created")
		}
	} else if !a.cfg.AutoStart && taskExists {
		a.log.Info("autostart", "Config says OFF but task exists — removing")
		executil.HiddenCmd("schtasks", "/delete", "/tn", "ZPUI", "/f").Run()
		a.log.Info("autostart", "Scheduled task removed")
	}
}
