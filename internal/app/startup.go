package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"zpui/internal/modules"
	"zpui/internal/updater"
)

type StartupStage string

const (
	StageWelcome   StartupStage = "welcome"
	StageSelfCheck StartupStage = "self_check"
	StageSelfDL    StartupStage = "self_download"
	StageModCheck  StartupStage = "mod_check"
	StageModDL     StartupStage = "mod_download"
	StageInstall   StartupStage = "install"
	StageRestart   StartupStage = "restart"
	StageDone      StartupStage = "done"
)

type StartupInfo struct {
	Stage      StartupStage    `json:"stage"`
	Sub        string          `json:"sub,omitempty"`
	Progress   float64         `json:"progress"`
	Error      string          `json:"error,omitempty"`
	SelfUpdate *UpdateInfo     `json:"self_update,omitempty"`
	ModUpdates []ModUpdateInfo `json:"mod_updates,omitempty"`
}

type UpdateInfo struct {
	Version string `json:"version"`
	Body    string `json:"body"`
}

type ModUpdateInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type startupState struct {
	mu          sync.Mutex
	info        StartupInfo
	completed   bool
	restartBat  string
	selfUpdated bool
	t0          time.Time
}

func (a *App) runStartupSequence() {
	a.startup.set(a.startup.info)
	a.startup.mu.Lock()
	a.startup.t0 = time.Now()
	a.startup.mu.Unlock()

	a.setStage(StageWelcome, "", 0)
	time.Sleep(2800 * time.Millisecond)

	if !a.doUpdateCheck() {
		a.setStage(StageModCheck, "", 0.35)
		time.Sleep(2200 * time.Millisecond)
		a.doModuleCheck()
	}

	if a.startup.selfUpdated {
		a.setStage(StageRestart, "Перезапуск...", 1.0)
		time.Sleep(800 * time.Millisecond)
		if a.startup.restartBat != "" {
			updater.RunBat(a.startup.restartBat)
			os.Exit(0)
		}
	}

	a.setStage(StageInstall, "", 0.9)
	time.Sleep(1200 * time.Millisecond)

	a.ensureMinTime()

	a.startup.mu.Lock()
	a.startup.completed = true
	a.startup.mu.Unlock()
	a.setStage(StageDone, "", 1.0)
}

func (a *App) ensureMinTime() {
	a.startup.mu.Lock()
	t0 := a.startup.t0
	a.startup.mu.Unlock()
	elapsed := time.Since(t0)
	minTotal := 10 * time.Second
	if elapsed < minTotal {
		time.Sleep(minTotal - elapsed)
	}
}

func (a *App) setStage(stage StartupStage, sub string, progress float64) {
	a.startup.update(func(s *StartupInfo) {
		s.Stage = stage
		s.Sub = sub
		s.Progress = progress
	})
}

func (a *App) doUpdateCheck() (updated bool) {
	a.setStage(StageSelfCheck, "", 0.15)
	t0 := time.Now()

	release, err := a.updater.CheckLatest(context.Background())
	elapsed := time.Since(t0)
	minDuration := 2200 * time.Millisecond
	if elapsed < minDuration {
		time.Sleep(minDuration - elapsed)
	}

	if err != nil {
		a.log.Warn("startup", fmt.Sprintf("self-update check: %v", err))
		return false
	}

	if release == nil || !a.updater.NeedsUpdate(release) {
		return false
	}

	a.setStage(StageSelfDL, release.TagName, 0.35)
	asset := a.updater.FindAsset(release, ".exe")
	if asset == nil {
		return false
	}

	dlPath := filepath.Join(a.exeDir, "zpui.exe.new")
	if err := a.updater.Download(context.Background(), asset.URL, dlPath); err != nil {
		a.log.Error("startup", fmt.Sprintf("self-update download: %v", err))
		a.setStage(StageSelfCheck, "Ошибка загрузки", 0.35)
		time.Sleep(1500 * time.Millisecond)
		return false
	}

	batPath, err := a.updater.PrepareSelfUpdate(filepath.Join(a.exeDir, "zpui.exe"), dlPath)
	if err != nil {
		a.log.Error("startup", fmt.Sprintf("prepare update: %v", err))
		return false
	}

	a.startup.restartBat = batPath
	a.startup.selfUpdated = true
	a.setStage(StageInstall, "Установка обновления...", 0.7)
	time.Sleep(1200 * time.Millisecond)
	return true
}

func (a *App) doModuleCheck() {
	discovered := modules.Discover(a.mgr.RootDir())
	var updates []ModUpdateInfo

	for _, dm := range discovered {
		if !dm.EntryOK || dm.Manifest.UpdateURL == "" {
			continue
		}
		rel, err := a.updater.CheckLatest(context.Background())
		if err != nil {
			a.log.Warn("startup", fmt.Sprintf("module %s update: %v", dm.Manifest.ID, err))
			continue
		}
		if rel != nil && a.updater.NeedsUpdate(rel) {
			updates = append(updates, ModUpdateInfo{ID: dm.Manifest.ID, Name: dm.Manifest.Name, Version: rel.TagName})
		}
	}

	if len(updates) > 0 {
		a.setStage(StageModDL, fmt.Sprintf("Обновление модулей (%d)...", len(updates)), 0.6)
		time.Sleep(1800 * time.Millisecond)
		for _, upd := range updates {
			for _, dm := range discovered {
				if dm.Manifest.ID == upd.ID {
					a.downloadModuleUpdate(dm)
					break
				}
			}
		}
	}

	a.setStage(StageInstall, "", 0.85)
	time.Sleep(1000 * time.Millisecond)

	if a.cfg.AutoStartMods {
		a.mgr.AutoStartAll(modules.Discover(a.mgr.RootDir()))
	}
}

func (a *App) downloadModuleUpdate(dm *modules.DiscoveredModule) {
	if dm.Manifest.UpdateURL == "" {
		return
	}

	a.log.Info("startup", fmt.Sprintf("downloading module update: %s", dm.Manifest.ID))
	dest := filepath.Join(dm.Dir, dm.Manifest.Entry+".new")
	if err := a.updater.Download(context.Background(), dm.Manifest.UpdateURL, dest); err != nil {
		a.log.Error("startup", fmt.Sprintf("module %s download: %v", dm.Manifest.ID, err))
		return
	}

	oldEntry := filepath.Join(dm.Dir, dm.Manifest.Entry)
	backup := oldEntry + ".bak"
	os.Remove(backup)
	if err := os.Rename(oldEntry, backup); err != nil {
		a.log.Error("startup", fmt.Sprintf("module %s backup: %v", dm.Manifest.ID, err))
		return
	}
	if err := os.Rename(dest, oldEntry); err != nil {
		os.Rename(backup, oldEntry)
		a.log.Error("startup", fmt.Sprintf("module %s install: %v", dm.Manifest.ID, err))
		return
	}
	os.Remove(backup)
}

type UIRegistration struct {
	ModuleID  string `json:"module_id"`
	Placement string `json:"placement"`
	CompID    string `json:"comp_id"`
	CompType  string `json:"comp_type"`
	Label     string `json:"label,omitempty"`
	Icon      string `json:"icon,omitempty"`
	Action    string `json:"action,omitempty"`
	Color     string `json:"color,omitempty"`
	Metadata  string `json:"metadata,omitempty"`
}

func (a *App) collectUIRegistrations() []UIRegistration {
	var out []UIRegistration
	for _, dm := range a.mgr.Discover() {
		ui := dm.Manifest.UI
		for _, c := range ui.StatusBar {
			out = append(out, UIRegistration{
				ModuleID:  dm.Manifest.ID,
				Placement: "statusbar",
				CompID:    c.ID,
				CompType:  c.Type,
				Label:     c.Label,
				Icon:      c.Icon,
				Action:    c.Action,
				Color:     c.Color,
				Metadata:  c.Metadata,
			})
		}
		for _, c := range ui.Sidebar {
			out = append(out, UIRegistration{
				ModuleID:  dm.Manifest.ID,
				Placement: "sidebar",
				CompID:    c.ID,
				CompType:  c.Type,
				Label:     c.Label,
				Icon:      c.Icon,
				Action:    c.Action,
				Color:     c.Color,
				Metadata:  c.Metadata,
			})
		}
		statusFile := filepath.Join(dm.Dir, "status.json")
		if data, err := os.ReadFile(statusFile); err == nil {
			var dynamic struct {
				UI struct {
					StatusBar map[string]struct {
						Label string `json:"label"`
						Color string `json:"color"`
					} `json:"statusbar"`
				} `json:"ui"`
			}
			if json.Unmarshal(data, &dynamic) == nil {
				for i := range out {
					if out[i].ModuleID == dm.Manifest.ID && out[i].Placement == "statusbar" {
						if d, ok := dynamic.UI.StatusBar[out[i].CompID]; ok {
							if d.Label != "" {
								out[i].Label = d.Label
							}
							if d.Color != "" {
								out[i].Color = d.Color
							}
						}
					}
				}
			}
		}
	}
	return out
}

func (s *startupState) update(fn func(*StartupInfo)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.info)
}

func (s *startupState) get() StartupInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.info
}

func (s *startupState) isCompleted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.completed
}

func (s *startupState) set(info StartupInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.info = info
}
