package app

import (
	"encoding/json"
	"fmt"
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

// DownloadUpdate скачивает обновление для указанного компонента с прогрессом.
// Прогресс эмитится через Wails Events "update:download-progress".
func (a *App) DownloadUpdate(name string) map[string]interface{} {
	exeDir := a.getExeDir()
	tmpDir := filepath.Join(exeDir, ".update-tmp")
	os.MkdirAll(tmpDir, 0755)

	switch name {
	case "ZPUI":
		remote, err := updater.FetchRemoteVersions()
		if err != nil {
			return map[string]interface{}{"error": "Failed to fetch versions: " + err.Error()}
		}
		if remote.ZPUI == "" || !updater.IsNewer(a.version, remote.ZPUI) {
			return map[string]interface{}{"error": "No update available"}
		}

		// Build download URLs: GitHub first, Yandex fallback
		arch := "win64"
		ghURL := fmt.Sprintf("https://github.com/suzcuaru/ZPUI/releases/latest/download/zpui-%s.zip", arch)
		yaURL, _ := updater.YandexDownloadURL(updater.YandexPublicURL, "", "zpui.exe")
		urls := updater.ChooseBestSource(ghURL, yaURL, remote.ZPUI, remote.ZPUI)

		dest := filepath.Join(tmpDir, "zpui-update.zip")
		go func() {
			err := updater.DownloadFromBestSource(urls, dest, func(downloaded, total int64) {
				pct := 0
				if total > 0 {
					pct = int(downloaded * 100 / total)
				}
				a.emitUpdateProgress("downloading", pct)
			})
			if err != nil {
				a.emitUpdateProgress("error", 0)
				a.log.Error("updater", "Download failed: "+err.Error())
				return
			}
			a.emitUpdateProgress("downloaded", 100)
			a.log.Info("updater", "ZPUI update downloaded to "+dest)
		}()
		return map[string]interface{}{"status": "download_started", "dest": dest}

	case "Zapret":
		dest := filepath.Join(tmpDir, "zapret-update.zip")
		go func() {
			info, err := a.zapret.CheckForUpdates()
			if err != nil || !info.UpdateNeeded {
				a.emitUpdateProgress("error", 0)
				return
			}
			urls := []string{info.DownloadURL}
			urls = append(urls, info.FallbackURLs...)
			if yaURL, _, yErr := updater.YandexFindZapretZip(updater.YandexPublicURL); yErr == nil && yaURL != "" {
				urls = append(urls, yaURL)
			}
			err = updater.DownloadFromBestSource(urls, dest, func(downloaded, total int64) {
				pct := 0
				if total > 0 {
					pct = int(downloaded * 100 / total)
				}
				a.emitUpdateProgress("downloading", pct)
			})
			if err != nil {
				a.emitUpdateProgress("error", 0)
				a.log.Error("updater", "Zapret download failed: "+err.Error())
				return
			}
			a.emitUpdateProgress("downloaded", 100)
			a.log.Info("updater", "Zapret update downloaded to "+dest)
		}()
		return map[string]interface{}{"status": "download_started", "dest": dest}

	default:
		return map[string]interface{}{"error": "unknown component: " + name}
	}
}

// VerifyUpdate проверяет скачанный файл обновления.
func (a *App) VerifyUpdate(name string) map[string]interface{} {
	exeDir := a.getExeDir()
	tmpDir := filepath.Join(exeDir, ".update-tmp")

	var path string
	switch name {
	case "ZPUI":
		path = filepath.Join(tmpDir, "zpui-update.zip")
	case "Zapret":
		path = filepath.Join(tmpDir, "zapret-update.zip")
	default:
		return map[string]interface{}{"error": "unknown component: " + name}
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return map[string]interface{}{"error": "Downloaded file not found"}
	}

	// Verify file size
	if err := updater.VerifyFileSize(path, 1024, 200<<20); err != nil {
		a.log.Error("updater", "Verification failed: "+err.Error())
		return map[string]interface{}{"error": err.Error()}
	}

	a.log.Info("updater", "Update verified: "+path)
	return map[string]interface{}{"status": "verified", "path": path}
}

// InstallUpdate устанавливает ранее скачанное обновление.
func (a *App) InstallUpdate(name string) map[string]interface{} {
	exeDir := a.getExeDir()
	tmpDir := filepath.Join(exeDir, ".update-tmp")

	switch name {
	case "ZPUI":
		zipPath := filepath.Join(tmpDir, "zpui-update.zip")
		if _, err := os.Stat(zipPath); os.IsNotExist(err) {
			return map[string]interface{}{"error": "Downloaded file not found. Download first."}
		}
		go func() {
			a.emitUpdateProgress("installing", 0)
			selfUpdate := filepath.Join(exeDir, "selfupdate.exe")
			if _, err := os.Stat(selfUpdate); err == nil {
				// Use selfupdate.exe for cold replacement
				cmd := executil.DetachedCmd(selfUpdate)
				if err := cmd.Start(); err != nil {
					a.emitUpdateProgress("error", 0)
					a.log.Error("updater", "Self-update start failed: "+err.Error())
					return
				}
				a.emitUpdateProgress("installed", 100)
			} else {
				a.emitUpdateProgress("error", 0)
				a.log.Error("updater", "selfupdate.exe not found")
			}
		}()
		return map[string]interface{}{"status": "install_started"}

	case "Zapret":
		zipPath := filepath.Join(tmpDir, "zapret-update.zip")
		if _, err := os.Stat(zipPath); os.IsNotExist(err) {
			return map[string]interface{}{"error": "Downloaded file not found. Download first."}
		}
		go func() {
			a.emitUpdateProgress("installing", 0)
			progress := make(chan struct{})
			go func() {
				if err := a.zapret.PerformUpdate(nil); err != nil {
					a.log.Error("updater", "Zapret install failed: "+err.Error())
					a.emitUpdateProgress("error", 0)
				} else {
					a.zapret.RefreshVersion()
					a.emitUpdateProgress("installed", 100)
				}
				close(progress)
			}()
			<-progress
		}()
		return map[string]interface{}{"status": "install_started"}

	default:
		return map[string]interface{}{"error": "unknown component: " + name}
	}
}

// GetReleaseInfo получает информацию о последнем релизе с GitHub.
func (a *App) GetReleaseInfo(component string) map[string]interface{} {
	switch component {
	case "ZPUI":
		rel, err := updater.FetchReleaseInfo()
		if err != nil {
			return map[string]interface{}{"error": err.Error()}
		}
		return map[string]interface{}{
			"tag_name": rel.TagName,
			"assets":   rel.Assets,
		}
	case "Zapret":
		info, err := a.zapret.CheckForUpdates()
		if err != nil {
			return map[string]interface{}{"error": err.Error()}
		}
		return map[string]interface{}{
			"current_version": info.CurrentVersion,
			"latest_version":  info.LatestVersion,
			"release_page":    info.ReleasePage,
			"download_url":    info.DownloadURL,
		}
	default:
		return map[string]interface{}{"error": "unknown component: " + component}
	}
}

// emitUpdateProgress отправляет прогресс обновления на фронтенд.
func (a *App) emitUpdateProgress(step string, percent int) {
	a.log.Info("updater", fmt.Sprintf("Update progress: %s %d%%", step, percent))
}
