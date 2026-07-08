package app

import (
	"fmt"
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

	go func() {
		exePath := getExePath()
		a.log.Info("autostart", "Creating scheduled task...")
		executil.HiddenCmd("schtasks", "/delete", "/tn", "ZPUI", "/f").Run()
		cmd := executil.HiddenCmd("schtasks", "/create",
			"/tn", "ZPUI",
			"/tr", fmt.Sprintf(`"%s"`, exePath),
			"/sc", "onlogon",
			"/rl", "highest",
			"/f")
		output, err := cmd.CombinedOutput()
		if err != nil {
			a.log.Error("autostart", fmt.Sprintf("Task Scheduler error: %v: %s", err, string(output)))
			executil.HiddenCmd("reg", "add",
				`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
				"/v", "ZPUI", "/t", "REG_SZ", "/d",
				fmt.Sprintf(`"%s"`, exePath), "/f").Run()
		} else {
			a.log.Info("autostart", "Scheduled task created")
		}
	}()

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

	go func() {
		executil.HiddenCmd("schtasks", "/delete", "/tn", "ZPUI", "/f").Run()
		executil.HiddenCmd("reg", "delete",
			`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
			"/v", "ZPUI", "/f").Run()
	}()

	return map[string]interface{}{"status": "disabled"}
}

// ============================================================
// CONFIG
// ============================================================

func (a *App) GetConfig() map[string]interface{} {
	return map[string]interface{}{
		"current_strategy":    a.cfg.GetCurrentStrategy(),
		"web_port":            a.cfg.Web.Port,
		"proxy":               a.cfg.GetProxyConfig(),
		"xbox_dns":            a.cfg.GetXboxDnsConfig(),
		"autostart":           a.cfg.AutoStart,
		"auto_update_check":   a.cfg.AutoUpdateCheck,
		"theme":               a.cfg.Theme,
		"language":            a.cfg.GetLanguage(),
		"first_run_done":      a.cfg.FirstRunDone,
		"zapret_skipped":      a.cfg.GetZapretSkipped(),
		"start_minimized":     a.cfg.StartMinimized,
		"close_to_tray":       a.cfg.GetCloseToTray(),
		"last_zapret_state":   a.cfg.LastZapretState,
		"last_proxy_state":    a.cfg.LastProxyState,
		"last_xbox_dns_state": a.cfg.LastXboxDnsState,
		"auto_start_zapret":   a.cfg.AutoStartZapret,
		"auto_start_proxy":    a.cfg.AutoStartProxy,
		"auto_start_xbox_dns": a.cfg.AutoStartXboxDns,
		"notifications_enabled": a.cfg.GetNotificationsEnabled(),
		"notify_zpui_updates":   a.cfg.NotifyZPUIUpdates,
		"notify_zapret_updates": a.cfg.NotifyZapretUpdates,
		"notify_missing_files":  a.cfg.NotifyMissingFiles,
		"notify_service_crash": a.cfg.NotifyServiceCrash,
		"notify_resource_drop":  a.cfg.NotifyResourceDrop,
		"resource_drop_pct":     a.cfg.GetResourceDropPct(),
		"show_strategy_modal":   a.cfg.ShowStrategyModal,
		"notify_strategy_test":  a.cfg.NotifyStrategyTest,
		"logs":                a.cfg.Logs,
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
	if err := a.cfg.Save(); err != nil {
		a.log.Error("config", "Save error: "+err.Error())
	} else {
		a.log.Info("config", "Config saved to JSON")
	}
	return map[string]interface{}{"status": "ok"}
}
