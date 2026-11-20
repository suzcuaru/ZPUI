package app

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"zpui/internal/database"
	"zpui/internal/updater"
)

// updateFlags — кэш результатов стартовой проверки обновлений. Читается треем
// (каждые 5с в его цикле) и панелью версий без сетевых вызовов: проверки
// выполняются только при запуске приложения и при ручном скачивании.
type updateFlags struct {
	mu              sync.Mutex
	zpuiChecked     bool
	zpuiAvailable   bool
	zpuiLatest      string
	zpuiSrvVersion  string
	zpuiSrvURL      string
	zpuiSrvSHA      string
	zapretChecked   bool
	zapretAvailable bool
	zapretCurrent   string
	zapretLatest    string
	zapretSource    string
}

var updFlags updateFlags

func (a *App) setZPUIUpdateAvailable(latest string) {
	updFlags.mu.Lock()
	updFlags.zpuiChecked = true
	updFlags.zpuiAvailable = latest != ""
	updFlags.zpuiLatest = latest
	updFlags.mu.Unlock()
}

// setZPUIServerManifest — кэш манифеста обновления ZPUI с сервера из последней
// проверки: ручное скачивание достраивает из него план, когда живые источники
// недоступны или противоречивы. Пустая версия очищает запись целиком.
func (a *App) setZPUIServerManifest(version, url, sha string) {
	if version == "" {
		url, sha = "", ""
	}
	updFlags.mu.Lock()
	updFlags.zpuiSrvVersion, updFlags.zpuiSrvURL, updFlags.zpuiSrvSHA = version, url, sha
	updFlags.mu.Unlock()
}

func (a *App) zpuiServerManifestCached() (version, url, sha string) {
	updFlags.mu.Lock()
	defer updFlags.mu.Unlock()
	return updFlags.zpuiSrvVersion, updFlags.zpuiSrvURL, updFlags.zpuiSrvSHA
}

// zpuiManifestCacheNewer — в кэше есть манифест сервера и он новее current.
func (a *App) zpuiManifestCacheNewer(current string) bool {
	v, _, _ := a.zpuiServerManifestCached()
	return v != "" && updater.IsNewer(current, v)
}

func (a *App) setZapretUpdateAvailable(current, latest, source string) {
	updFlags.mu.Lock()
	updFlags.zapretChecked = true
	updFlags.zapretAvailable = latest != "" && latest != current
	updFlags.zapretCurrent = current
	updFlags.zapretLatest = latest
	updFlags.zapretSource = source
	updFlags.mu.Unlock()
}

func (a *App) HasZPUIUpdateAvailable() bool {
	updFlags.mu.Lock()
	defer updFlags.mu.Unlock()
	return updFlags.zpuiAvailable
}

func (a *App) HasZapretUpdateAvailable() bool {
	updFlags.mu.Lock()
	defer updFlags.mu.Unlock()
	return updFlags.zapretAvailable
}

// zapretUpdateSource — источник, с которого будет качаться запрет.
func (a *App) zapretUpdateSource() string {
	updFlags.mu.Lock()
	defer updFlags.mu.Unlock()
	return updFlags.zapretSource
}

// syncVersionsToDB записывает удалённые версии компонентов в БД
// (история версий на мониторе). Вызывается один раз при старте.
// При ошибке проверки строка НЕ замораживается со старым remote: удалённая
// версия сбрасывается, а установленная — обновляется, иначе таблица версий
// вечно показывает протухшие значения (например, zpui 1.6.2 при 1.7.0).
func (a *App) syncVersionsToDB() {
	now := time.Now()

	a.pruneComponentVersions()

	// ZPUI: приоритет источника — сервер управления, фолбэк Яндекс.Диск.
	handled := false
	if m, reachable := a.fetchServerZPUIManifest(); reachable {
		handled = true
		if m != nil {
			database.SaveComponentVersion(&database.ComponentVersion{
				ID:               "zpui",
				InstalledVersion: a.version,
				RemoteVersion:    m.Version,
				RemoteSource:     "server",
				RemoteUpdatedAt:  &now,
			})
		} else {
			database.UpsertComponentInstalled("zpui", a.version)
			database.InvalidateComponentRemote("zpui")
		}
	}
	if !handled {
		if info, err := updater.CheckZPUISetupUpdate(a.version); err == nil && info.Version != "" {
			database.SaveComponentVersion(&database.ComponentVersion{
				ID:               "zpui",
				InstalledVersion: a.version,
				RemoteVersion:    info.Version,
				RemoteSource:     "yandex",
				RemoteUpdatedAt:  &now,
			})
		} else {
			if err != nil {
				a.log.Warn("updater", "ZPUI version sync failed: "+err.Error())
			}
			database.UpsertComponentInstalled("zpui", a.version)
			database.InvalidateComponentRemote("zpui")
		}
	}

	if !a.cfg.GetZapretSkipped() {
		if info, err := a.zapret.CheckForUpdates(); err == nil && info != nil {
			database.SaveComponentVersion(&database.ComponentVersion{
				ID:               "zapret",
				InstalledVersion: info.CurrentVersion,
				RemoteVersion:    info.LatestVersion,
				RemoteSource:     info.Source,
				RemoteUpdatedAt:  &now,
			})
		} else {
			if err != nil {
				a.log.Warn("updater", "zapret version sync failed: "+err.Error())
			}
			database.UpsertComponentInstalled("zapret", a.zapret.GetVersion())
			database.InvalidateComponentRemote("zapret")
		}
	}

	if sv := readSelfupdateVersion(a.exeDir); sv != "" {
		if rv, err := updater.FetchComponentVersion("selfupdate"); err == nil && rv != "" {
			database.SaveComponentVersion(&database.ComponentVersion{
				ID:               "selfupdate",
				InstalledVersion: sv,
				RemoteVersion:    rv,
				RemoteSource:     "yandex",
				RemoteUpdatedAt:  &now,
			})
		} else {
			database.UpsertComponentInstalled("selfupdate", sv)
		}
	}

	a.log.Info("updater", "syncVersionsToDB: versions synced to DB")
}

// readSelfupdateVersion читает версию хелпера самообновления из
// components/selfupdate/version.txt рядом с exe ("" если файла нет).
func readSelfupdateVersion(exeDir string) string {
	data, err := os.ReadFile(filepath.Join(exeDir, "components", "selfupdate", "version.txt"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// pruneComponentVersions удаляет строки удалённых компонентов: их «версии»
// больше ничем не поддерживаются и только путают отчёты (например, давно
// удалённый модуль security продолжал вечно числиться в таблице).
func (a *App) pruneComponentVersions() {
	known := map[string]bool{"zpui": true, "zapret": true, "selfupdate": true, "tgproxy": true}
	rows, err := database.GetAllComponentVersions()
	if err != nil {
		return
	}
	for _, cv := range rows {
		if !known[cv.ID] {
			if err := database.DeleteComponentVersion(cv.ID); err == nil {
				a.log.Info("updater", "pruned stale component version row: "+cv.ID)
			}
		}
	}
}
