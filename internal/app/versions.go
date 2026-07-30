package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"zpui/internal/executil"
	"zpui/internal/updater"
)

func latestFromState(s *selfUpdateState) string {
	if s == nil {
		return ""
	}
	if s.ZPUI.GitHub.Avail && s.ZPUI.GitHub.Version != "" {
		return s.ZPUI.GitHub.Version
	}
	if s.ZPUI.Cloud.Avail && s.ZPUI.Cloud.Version != "" {
		return s.ZPUI.Cloud.Version
	}
	return ""
}

type ComponentVersion struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	File    string `json:"file"`
}

type VersionsManifest struct {
	ZPUI       string `json:"zpui"`
	SelfUpdate string `json:"selfupdate"`
	Report     string `json:"report"`
}

// findModulePath ищет модуль сначала рядом с exe (каноническая плоская раскладка
// установки), затем в components/<name>/ (легаси). Папка components/ теперь
// относится к распределению Яндекс.Диска и в установку не попадает.
func (a *App) findModulePath(name string) string {
	// 1. Каноническая позиция: рядом с exe.
	flatPath := filepath.Join(a.exeDir, name+".exe")
	if _, err := os.Stat(flatPath); err == nil {
		return flatPath
	}
	// 2. Легаси-фолбек: components/<name>/<name>.exe (старые установки).
	compPath := filepath.Join(a.exeDir, "components", name, name+".exe")
	if _, err := os.Stat(compPath); err == nil {
		return compPath
	}
	return ""
}

func (a *App) GetVersions() map[string]interface{} {
	manifest := a.loadVersionsManifest()

	components := []ComponentVersion{
		{Name: "ZPUI", Version: a.version, File: "ZPUI.exe"},
		{Name: "SelfUpdate", Version: manifest.SelfUpdate, File: "selfupdate.exe"},
	}

	if a.findModulePath("report") != "" {
		components = append(components, ComponentVersion{
			Name: "Report", Version: manifest.Report, File: "report.exe",
		})
	}

	installed := map[string]bool{}
	for _, c := range components {
		path := filepath.Join(a.exeDir, c.File)
		if _, err := os.Stat(path); err != nil {
			// Проверяем также в components/
			path = filepath.Join(a.exeDir, "components", strings.ToLower(c.Name), c.File)
		}
		_, err := os.Stat(path)
		installed[strings.ToLower(c.Name)] = err == nil
	}

	verMap := map[string]string{}
	for _, c := range components {
		key := strings.ToLower(c.Name)
		verMap[key] = c.Version
	}

	return map[string]interface{}{
		"components": components,
		"zpui":       a.version,
		"branch":     updater.VersionBranch(a.version),
		"installed":  installed,
	}
}

func (a *App) loadVersionsManifest() VersionsManifest {
	manifest := VersionsManifest{
		SelfUpdate: "0.0.0",
		Report:     "0.0.0",
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
		if m.SelfUpdate != "" {
			manifest.SelfUpdate = m.SelfUpdate
		}
		if m.Report != "" {
			manifest.Report = m.Report
		}
	}

	return manifest
}

func (a *App) CheckZPUIUpdate() map[string]interface{} {
	state := a.getCachedUpdateState()
	if state == nil {
		return map[string]interface{}{
			"error":          "cache not ready",
			"current":        a.version,
			"update_needed":  false,
			"repo_available": false,
		}
	}

	latest := latestFromState(state)
	source := "yandex-fallback"
	if state.ZPUI.GitHub.Avail {
		source = "github"
	}

	return map[string]interface{}{
		"current":        a.version,
		"latest":         latest,
		"update_needed":  state.ZPUI.UpdateNeeded,
		"repo_available": state.ZPUI.GitHub.Avail || state.ZPUI.Cloud.Avail,
		"source":         source,
	}
}

func (a *App) CheckVersionsPanel() map[string]interface{} {
	state := a.getCachedUpdateState()
	if state == nil {
		return map[string]interface{}{
			"current": a.version,
			"branch":  updater.VersionBranch(a.version),
			"github":  map[string]interface{}{"version": "", "avail": false},
			"cloud":   map[string]interface{}{"version": "", "avail": false},
		}
	}

	return map[string]interface{}{
		"current": a.version,
		"branch":  updater.VersionBranch(a.version),
		"github":  map[string]interface{}{"version": state.ZPUI.GitHub.Version, "avail": state.ZPUI.GitHub.Avail},
		"cloud":   map[string]interface{}{"version": state.ZPUI.Cloud.Version, "avail": state.ZPUI.Cloud.Avail},
	}
}

func (a *App) GetReleaseNotes() map[string]interface{} {
	info, err := updater.GetReleaseInfo()
	if err != nil {
		return map[string]interface{}{
			"error": err.Error(),
		}
	}
	return map[string]interface{}{
		"tag_name":     info.TagName,
		"name":         info.Name,
		"body":         info.Body,
		"published_at": info.PublishedAt,
		"html_url":     info.HTMLURL,
	}
}

func (a *App) GetZapretReleaseInfo() map[string]interface{} {
	info, err := updater.GetZapretReleaseInfo()
	if err != nil {
		return map[string]interface{}{
			"error": err.Error(),
		}
	}
	return map[string]interface{}{
		"tag_name":     info.TagName,
		"name":         info.Name,
		"body":         info.Body,
		"published_at": info.PublishedAt,
		"html_url":     info.HTMLURL,
	}
}

func (a *App) CheckZapretVersionsPanel() map[string]interface{} {
	state := a.getCachedUpdateState()
	if state == nil {
		return map[string]interface{}{
			"current": "",
			"github":  map[string]interface{}{"version": "", "avail": false},
			"cloud":   map[string]interface{}{"version": "", "avail": false},
		}
	}

	return map[string]interface{}{
		"current": state.Zapret.Current,
		"github":  map[string]interface{}{"version": state.Zapret.GitHub.Version, "avail": state.Zapret.GitHub.Avail},
		"cloud":   map[string]interface{}{"version": state.Zapret.Cloud.Version, "avail": state.Zapret.Cloud.Avail},
	}
}

func (a *App) PrepareZPUIUpdate() map[string]interface{} {
	selfUpdate := a.findModulePath("selfupdate")
	if selfUpdate == "" {
		return errResp("selfupdate.exe не найден")
	}

	state := a.getCachedUpdateState()
	if state == nil {
		return errResp("не удалось проверить версию: кэш не готов")
	}

	latest := latestFromState(state)

	return okRespWithData(map[string]interface{}{
		"current":       a.version,
		"latest":        latest,
		"update_needed": state.ZPUI.UpdateNeeded,
		"updater_path":  selfUpdate,
	})
}

func (a *App) LaunchZPUIUpdater() map[string]interface{} {
	selfUpdate := a.findModulePath("selfupdate")
	if selfUpdate == "" {
		return errResp("selfupdate.exe не найден")
	}

	if err := executil.GuiCmd(selfUpdate, "--from-app").Start(); err != nil {
		return errResp("не удалось запустить обновление: " + err.Error())
	}

	return okRespWithData(map[string]interface{}{
		"status":  "updater_launched",
		"updater": "selfupdate.exe",
	})
}

func (a *App) getExeDir() string {
	exePath, _ := os.Executable()
	return filepath.Dir(exePath)
}

func (a *App) CheckComponentUpdates() map[string]interface{} {
	manifest := a.loadVersionsManifest()
	localVersions := map[string]string{
		"zpui":       a.version,
		"selfupdate": manifest.SelfUpdate,
		"report":     manifest.Report,
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
	list := []map[string]string{
		{"name": "ZPUI", "current": a.version, "latest": a.version, "file": "ZPUI.exe"},
		{"name": "SelfUpdate", "current": manifest.SelfUpdate, "latest": manifest.SelfUpdate, "file": "selfupdate.exe"},
	}
	if manifest.Report != "0.0.0" {
		list = append(list, map[string]string{"name": "Report", "current": manifest.Report, "latest": manifest.Report, "file": "report.exe"})
	}
	return list
}

func (a *App) UpdateComponent(name string) map[string]interface{} {
	exeDir := a.getExeDir()

	switch name {
	case "ZPUI", "Zapret":
		return a.LaunchZPUIUpdater()

	case "selfupdate", "report":
		a.safeGo(func() {
			if err := updater.ReplaceModule(exeDir, name); err != nil {
				a.log.Error("updater", "Module update failed for "+name+": "+err.Error())
			} else {
				a.log.Info("updater", "Module "+name+" updated successfully")
			}
		})
		return map[string]interface{}{"status": "module_update_started", "component": name}

	default:
		return map[string]interface{}{"error": "unknown component: " + name}
	}
}
