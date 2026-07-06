package app

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

func okResp(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		data = map[string]interface{}{}
	}
	data["status"] = "ok"
	return data
}

func errResp(msg string) map[string]interface{} {
	return map[string]interface{}{"status": "error", "error": msg}
}

func (a *App) GetStatus() map[string]interface{} {
	mods := a.mgr.Discover()
	running := 0
	for _, dm := range mods {
		if a.mgr.StateOf(dm.Manifest.ID) == "running" {
			running++
		}
	}
	return okResp(map[string]interface{}{
		"app": map[string]interface{}{
			"version":       a.version,
			"modules_dir":   a.mgr.RootDir(),
			"modules_count": len(mods),
			"running_count": running,
		},
		"mod": map[string]interface{}{
			"version":     a.version,
			"theme":       a.cfg.GetTheme(),
			"language":    a.cfg.GetLanguage(),
			"closeToTray": a.cfg.CloseToTray,
		},
	})
}

func (a *App) GetVersion() string {
	return a.version
}

func (a *App) GetConfig() map[string]interface{} {
	return okResp(map[string]interface{}{
		"theme":          a.cfg.GetTheme(),
		"language":       a.cfg.GetLanguage(),
		"start_minimized": a.cfg.StartMinimized,
		"close_to_tray":  a.cfg.CloseToTray,
		"auto_start_mods": a.cfg.AutoStartMods,
		"disabled_mods":  a.cfg.DisabledMods,
	})
}

func (a *App) SetConfig(patch map[string]interface{}) map[string]interface{} {
	if err := a.cfg.Apply(patch); err != nil {
		return errResp(err.Error())
	}
	return okResp(nil)
}

func (a *App) GetSystemTheme() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return "dark"
	}
	defer k.Close()
	val, _, err := k.GetIntegerValue("AppsUseLightTheme")
	if err != nil {
		return "dark"
	}
	if val == 1 {
		return "light"
	}
	return "dark"
}

func (a *App) SetLanguage(lang string) map[string]interface{} {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang != "ru" && lang != "en" {
		lang = "ru"
	}
	a.cfg.Apply(map[string]interface{}{"language": lang})
	return okResp(map[string]interface{}{"language": lang})
}

func (a *App) GetLogs(category string, lines int) map[string]interface{} {
	if lines <= 0 || lines > 1000 {
		lines = 200
	}
	entries := a.log.Recent(category, lines)
	return okResp(map[string]interface{}{"entries": entries})
}
