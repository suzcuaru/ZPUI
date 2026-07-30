package app

import (
	"encoding/json"
	"os/exec"
	"sync"
	"time"

	"zpui/internal/database"
	"zpui/internal/executil"
	"zpui/internal/updater"
)

// extractJSON возвращает подстроку от первой '{' до последней '}'.
// Защищает парсинг от мусора в stdout (например, лог-строк вида
// «[2026-07-24 ...] [INFO] ...» от старых сборок selfupdate, которые
// писали логи в консоль). Если JSON-объекта нет — возвращает исходные данные.
func extractJSON(b []byte) []byte {
	s := string(b)
	first := -1
	last := -1
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			if first < 0 {
				first = i
			}
		case '}':
			last = i
		}
	}
	if first >= 0 && last > first {
		return []byte(s[first : last+1])
	}
	return b
}

type srcVer struct {
	Version string `json:"version"`
	Avail   bool   `json:"avail"`
}

type selfTargetState struct {
	Current      string `json:"current"`
	UpdateNeeded bool   `json:"update_needed"`
	GitHub       srcVer `json:"github"`
	Cloud        srcVer `json:"cloud"`
}

type selfModuleState struct {
	Name        string `json:"name"`
	Current     string `json:"current"`
	Latest      string `json:"latest"`
	NeedsUpdate bool   `json:"needs_update"`
}

type selfUpdateState struct {
	Mode    string            `json:"mode"`
	Version string            `json:"version"`
	ZPUI    selfTargetState   `json:"zpui"`
	Zapret  selfTargetState   `json:"zapret"`
	Modules []selfModuleState `json:"modules"`
}

type updateCache struct {
	mu        sync.RWMutex
	state     *selfUpdateState
	lastCheck time.Time
}

var updCache updateCache

func (a *App) runSelfUpdateCheck() {
	selfUpdate := a.findModulePath("selfupdate")
	if selfUpdate == "" {
		a.log.Error("updater", "selfupdate.exe не найден рядом с exe")
		return
	}

	cmd := executil.HiddenCmd(selfUpdate, "--check")
	out, err := cmd.Output()
	if err != nil {
		a.log.Error("updater", "selfupdate --check failed: "+err.Error())
		if ee, ok := err.(*exec.ExitError); ok {
			a.log.Error("updater", "stderr: "+string(ee.Stderr))
		}
		return
	}

	var state selfUpdateState
	if err := json.Unmarshal(extractJSON(out), &state); err != nil {
		a.log.Error("updater", "failed to parse selfupdate output: "+err.Error())
		return
	}

	updCache.mu.Lock()
	updCache.state = &state
	updCache.lastCheck = time.Now()
	updCache.mu.Unlock()

	a.log.Info("updater", "selfupdate check complete: zpui="+state.ZPUI.Current+" zapret="+state.Zapret.Current)
}

func (a *App) getCachedUpdateState() *selfUpdateState {
	updCache.mu.RLock()
	defer updCache.mu.RUnlock()
	if updCache.state == nil {
		return nil
	}
	s := *updCache.state
	return &s
}

// syncVersionsToDB проверяет удалённые версии всех компонентов и записывает
// актуальные данные в БД. Вызывается при старте и каждый час.
func (a *App) syncVersionsToDB() {
	remote, err := updater.FetchRemoteVersions()
	if err != nil {
		a.log.Warn("updater", "syncVersionsToDB: fetch remote failed: "+err.Error())
	} else {
		now := time.Now()
		if remote.ZPUI != "" {
			database.SaveComponentVersion(&database.ComponentVersion{
				ID:               "zpui",
				InstalledVersion: a.version,
				RemoteVersion:    remote.ZPUI,
				RemoteSource:     "github",
				RemoteUpdatedAt:  &now,
			})
		}
		if remote.SelfUpdate != "" {
			manifest := a.loadVersionsManifest()
			database.SaveComponentVersion(&database.ComponentVersion{
				ID:               "selfupdate",
				InstalledVersion: manifest.SelfUpdate,
				RemoteVersion:    remote.SelfUpdate,
				RemoteSource:     "github",
				RemoteUpdatedAt:  &now,
			})
		}
		if remote.Report != "" {
			manifest := a.loadVersionsManifest()
			database.SaveComponentVersion(&database.ComponentVersion{
				ID:               "report",
				InstalledVersion: manifest.Report,
				RemoteVersion:    remote.Report,
				RemoteSource:     "github",
				RemoteUpdatedAt:  &now,
			})
		}
		a.log.Info("updater", "syncVersionsToDB: versions synced to DB")
	}

	if !a.cfg.GetZapretSkipped() {
		info, err := a.zapret.CheckForUpdates()
		if err == nil && info != nil {
			now := time.Now()
			database.SaveComponentVersion(&database.ComponentVersion{
				ID:               "zapret",
				InstalledVersion: info.CurrentVersion,
				RemoteVersion:    info.LatestVersion,
				RemoteSource:     "github",
				RemoteUpdatedAt:  &now,
			})
		}
	}
}

// startUpdatePanelPolling синхронизирует версии в БД.
// Первый запуск — спустя 12с после старта, далее — каждый час.
func (a *App) startUpdatePanelPolling() {
	time.Sleep(12 * time.Second)
	a.syncVersionsToDB()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.syncVersionsToDB()
		}
	}
}
