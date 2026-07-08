package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"zpui/internal/executil"
	"zpui/internal/updater"
)

type ComponentVersion struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	File    string `json:"file"`
}

type VersionsManifest struct {
	ZPUI         string `json:"zpui"`
	Wizard       string `json:"wizard"`
	AutoSelect   string `json:"autoselect"`
	SelfUpdate   string `json:"selfupdate"`
	ZapretUpdate string `json:"zapretupdate"`
}

type modManifest struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (a *App) GetVersions() map[string]interface{} {
	manifest := a.loadVersionsManifest()
	exeDir := a.getExeDir()

	components := []ComponentVersion{
		{Name: "ZPUI", Version: a.version, File: "zpui.exe"},
		{Name: "Wizard", Version: manifest.Wizard, File: "wizard.exe"},
		{Name: "AutoSelect", Version: manifest.AutoSelect, File: "autoselect.exe"},
		{Name: "SelfUpdate", Version: manifest.SelfUpdate, File: "selfupdate.exe"},
		{Name: "ZapretUpdate", Version: manifest.ZapretUpdate, File: "zapretupdate.exe"},
	}

	installed := map[string]bool{}
	for _, c := range components {
		_, err := os.Stat(filepath.Join(exeDir, c.File))
		installed[strings.ToLower(c.Name)] = err == nil
	}

	modsDir := filepath.Join(exeDir, "mods")
	if entries, err := os.ReadDir(modsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			manifestPath := filepath.Join(modsDir, e.Name(), "mod.json")
			data, err := os.ReadFile(manifestPath)
			if err != nil {
				continue
			}
			var m modManifest
			if err := json.Unmarshal(data, &m); err != nil {
				continue
			}
			if m.Version == "" {
				m.Version = "0.0.0"
			}
			id := m.ID
			if id == "" {
				id = e.Name()
			}
			displayName := m.Name
			if displayName == "" {
				displayName = id
			}
			components = append(components, ComponentVersion{
				Name: displayName, Version: m.Version, File: "mods/" + id + "/",
			})
			installed[id] = true
		}
	}

	verMap := map[string]string{}
	for _, c := range components {
		key := strings.ToLower(c.Name)
		verMap[key] = c.Version
	}

	return map[string]interface{}{
		"components": components,
		"zpui":       a.version,
		"wizard":     manifest.Wizard,
		"installed":  installed,
	}
}

func (a *App) loadVersionsManifest() VersionsManifest {
	manifest := VersionsManifest{
		Wizard:       "0.0.0",
		AutoSelect:   "0.0.0",
		SelfUpdate:   "0.0.0",
		ZapretUpdate: "0.0.0",
	}

	exePath, err := os.Executable()
	if err != nil {
		return manifest
	}
	versionsPath := filepath.Join(filepath.Dir(exePath), "versions.json")

	data, err := os.ReadFile(versionsPath)
	if err != nil {
		return manifest
	}

	var m VersionsManifest
	if err := json.Unmarshal(data, &m); err == nil {
		if m.Wizard != "" {
			manifest.Wizard = m.Wizard
		}
		if m.AutoSelect != "" {
			manifest.AutoSelect = m.AutoSelect
		}
		if m.SelfUpdate != "" {
			manifest.SelfUpdate = m.SelfUpdate
		}
		if m.ZapretUpdate != "" {
			manifest.ZapretUpdate = m.ZapretUpdate
		}
	}

	return manifest
}

func (a *App) CheckZPUIUpdate() map[string]interface{} {
	remote, err := updater.FetchRemoteVersions()
	if err != nil {
		return map[string]interface{}{
			"error":          err.Error(),
			"current":        a.version,
			"update_needed":  false,
			"repo_available": false,
		}
	}
	needed := remote.ZPUI != "" && updater.IsNewer(a.version, remote.ZPUI)
	return map[string]interface{}{
		"current":        a.version,
		"latest":         remote.ZPUI,
		"update_needed":  needed,
		"repo_available": true,
		"source":         "github+yandex-fallback",
	}
}

func (a *App) getExeDir() string {
	exePath, _ := os.Executable()
	return filepath.Dir(exePath)
}

func (a *App) CheckComponentUpdates() map[string]interface{} {
	manifest := a.loadVersionsManifest()
	localVersions := map[string]string{
		"zpui":         a.version,
		"wizard":       manifest.Wizard,
		"autoselect":   manifest.AutoSelect,
		"selfupdate":   manifest.SelfUpdate,
		"zapretupdate": manifest.ZapretUpdate,
	}

	components, err := updater.CheckAllComponents(localVersions, a.getExeDir())
	if err != nil {
		return map[string]interface{}{
			"error":      err.Error(),
			"components": a.fallbackComponentList(manifest),
		}
	}

	anyUpdate := false
	for _, c := range components {
		if c.NeedsUpdate {
			anyUpdate = true
			break
		}
	}

	return map[string]interface{}{
		"components": components,
		"any_update": anyUpdate,
	}
}

func (a *App) fallbackComponentList(manifest VersionsManifest) []map[string]string {
	return []map[string]string{
		{"name": "ZPUI", "current": a.version, "latest": a.version, "file": "zpui.exe"},
		{"name": "Wizard", "current": manifest.Wizard, "latest": manifest.Wizard, "file": "wizard.exe"},
		{"name": "AutoSelect", "current": manifest.AutoSelect, "latest": manifest.AutoSelect, "file": "autoselect.exe"},
		{"name": "SelfUpdate", "current": manifest.SelfUpdate, "latest": manifest.SelfUpdate, "file": "selfupdate.exe"},
		{"name": "ZapretUpdate", "current": manifest.ZapretUpdate, "latest": manifest.ZapretUpdate, "file": "zapretupdate.exe"},
	}
}

func (a *App) UpdateComponent(name string) map[string]interface{} {
	exeDir := a.getExeDir()

	switch name {
	case "ZPUI":
		selfUpdate := filepath.Join(exeDir, "selfupdate.exe")
		if _, err := os.Stat(selfUpdate); err != nil {
			return map[string]interface{}{"error": "selfupdate.exe не найден"}
		}
		if err := executil.DetachedCmd(selfUpdate).Start(); err != nil {
			return map[string]interface{}{"error": "не удалось запустить selfupdate.exe: " + err.Error()}
		}
		return map[string]interface{}{"status": "selfupdate_started"}

	case "Zapret":
		go func() {
			zapretUpdate := filepath.Join(exeDir, "zapretupdate.exe")
			if _, err := os.Stat(zapretUpdate); err == nil {
				cmd := executil.HiddenCmd(zapretUpdate)
				cmd.Start()
				cmd.Wait()
			}
			a.zapret.RefreshVersion()
			a.log.Info("updater", "Zapret update process finished, version="+a.zapret.GetVersion())
		}()
		return map[string]interface{}{"status": "zapretupdate_started"}

	case "wizard", "autoselect", "selfupdate", "zapretupdate":
		go func() {
			if err := updater.ReplaceModule(exeDir, name); err != nil {
				a.log.Error("updater", "Module update failed for "+name+": "+err.Error())
			} else {
				a.log.Info("updater", "Module "+name+" updated successfully")
			}
		}()
		return map[string]interface{}{"status": "module_update_started", "component": name}

	default:
		return map[string]interface{}{"error": "unknown component: " + name}
	}
}
