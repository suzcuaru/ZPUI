package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"zpui/internal/updater"
)

type ComponentVersion struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	File    string `json:"file"`
}

// GetVersions — версии для панели настроек: ZPUI + tgproxy.
// Модуль report снесён (волна 3A), строки компонентов больше не читаются
// из versions.json.
func (a *App) GetVersions() map[string]interface{} {
	components := []ComponentVersion{
		{Name: "ZPUI", Version: a.version, File: "ZPUI.exe"},
		{Name: "tgproxy", Version: a.tgproxy.Version(), File: "ZPUI-TgProxy.exe"},
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

	return map[string]interface{}{
		"components": components,
		"zpui":       a.version,
		"tgproxy":    a.tgproxy.Version(),
		"installed":  installed,
	}
}

// CheckZPUIUpdate — проверка обновления ZPUI для хука useUpdateCheck.
// Флаги выставлены стартовой проверкой; если проверки ещё не было
// (первые секунды после старта) — прямой запрос к Яндекс.Диску.
func (a *App) CheckZPUIUpdate() map[string]interface{} {
	updFlags.mu.Lock()
	checked, latest := updFlags.zpuiChecked, updFlags.zpuiLatest
	updFlags.mu.Unlock()

	if !checked {
		if info, err := updater.CheckZPUISetupUpdate(a.version); err == nil {
			latest = info.Version
			checked = true
		}
	}

	if !checked {
		return map[string]interface{}{
			"error":          "version check unavailable",
			"current":        a.version,
			"update_needed":  false,
			"repo_available": false,
		}
	}

	needed := latest != "" && updater.IsNewer(a.version, latest)
	return map[string]interface{}{
		"current":        a.version,
		"latest":         latest,
		"update_needed":  needed,
		"repo_available": latest != "",
		"source":         "yandex",
	}
}

// CheckVersionsPanel — данные для карточки ZPUI в настройках (без сети).
func (a *App) CheckVersionsPanel() map[string]interface{} {
	updFlags.mu.Lock()
	checked, latest := updFlags.zpuiChecked, updFlags.zpuiLatest
	updFlags.mu.Unlock()

	if !checked {
		return map[string]interface{}{
			"current": a.version,
			"latest":  "",
			"avail":   false,
			"source":  "",
		}
	}
	return map[string]interface{}{
		"current": a.version,
		"latest":  latest,
		"avail":   latest != "" && updater.IsNewer(a.version, latest),
		"source":  "yandex",
	}
}

// GetReleaseNotes — «что нового» версии ZPUI (пустая версия = последняя
// опубликованная). Источник — сервер управления (notes публикации);
// GitHub для приложения больше не используется.
func (a *App) GetReleaseNotes(version string) map[string]interface{} {
	info, err := a.serverZPUIReleaseNotes(version)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return info
}

// serverZPUIReleaseNotes берёт заметки из манифеста v1
// (GET /api/v1/updates/zpui): сервер хранит notes только у последней
// опубликованной версии.
func (a *App) serverZPUIReleaseNotes(version string) (map[string]interface{}, error) {
	if ServerURL == "" || !a.authorizationOn() {
		return nil, fmt.Errorf("заметки версий доступны на сервере")
	}
	m, reachable := a.fetchServerZPUIManifest()
	if !reachable {
		return nil, fmt.Errorf("сервер недоступен")
	}
	if m == nil {
		return nil, fmt.Errorf("на сервере нет опубликованных версий")
	}
	if version != "" && !strings.EqualFold(version, m.Version) {
		return nil, fmt.Errorf("версия %s на сервере не найдена", version)
	}
	return map[string]interface{}{
		"tag_name":     m.Version,
		"name":         "ZPUI " + m.Version,
		"body":         m.Notes,
		"published_at": "",
		"html_url":     "",
	}, nil
}

// GetZapretReleaseInfo — release notes версии запрета (пустая версия =
// последний релиз). Тела релизов Flowseal — фактический чейнджлог.
func (a *App) GetZapretReleaseInfo(version string) map[string]interface{} {
	var info *updater.ReleaseInfo
	var err error
	if version != "" {
		info, err = updater.GetZapretReleaseInfoByTag(version)
	} else {
		info, err = updater.GetZapretReleaseInfo()
	}
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

// CheckZapretVersionsPanel — данные для карточки запрета в настройках (без сети).
func (a *App) CheckZapretVersionsPanel() map[string]interface{} {
	updFlags.mu.Lock()
	checked, current, latest, source := updFlags.zapretChecked, updFlags.zapretCurrent, updFlags.zapretLatest, updFlags.zapretSource
	updFlags.mu.Unlock()

	if !checked {
		return map[string]interface{}{
			"current": "",
			"latest":  "",
			"avail":   false,
			"source":  "",
		}
	}
	return map[string]interface{}{
		"current": current,
		"latest":  latest,
		"avail":   latest != "" && updater.IsNewer(current, latest),
		"source":  source,
	}
}

func (a *App) getExeDir() string {
	exePath, _ := os.Executable()
	return filepath.Dir(exePath)
}
