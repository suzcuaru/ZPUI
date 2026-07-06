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
	Stage      StartupStage      `json:"stage"`
	Sub        string            `json:"sub,omitempty"`
	Progress   float64           `json:"progress"`
	Error      string            `json:"error,omitempty"`
	SelfUpdate *UpdateInfo       `json:"self_update,omitempty"`
	ModUpdates []ModUpdateInfo   `json:"mod_updates,omitempty"`
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
}

func (a *App) runStartupSequence() {
	a.startup.set(a.startup.info) // ensure initial state

	time.Sleep(1500 * time.Millisecond)

	release, err := a.updater.CheckLatest(context.Background())
	if err != nil {
		a.log.Warn("startup", fmt.Sprintf("self-update check: %v", err))
		a.startup.update(func(s *StartupInfo) {
			s.Stage = StageModCheck
			s.Progress = 0.35
			s.Sub = ""
		})
	} else if release != nil && a.updater.NeedsUpdate(release) {
		a.startup.update(func(s *StartupInfo) {
			s.Stage = StageSelfDL
			s.Progress = 0.2
			s.SelfUpdate = &UpdateInfo{Version: release.TagName, Body: truncate(release.Body, 200)}
		})
		if asset := a.updater.FindAsset(release, ".exe"); asset != nil {
			dlPath := filepath.Join(a.exeDir, "zpui.exe.new")
			if err := a.updater.Download(context.Background(), asset.URL, dlPath); err != nil {
				a.log.Error("startup", fmt.Sprintf("self-update download: %v", err))
				a.startup.update(func(s *StartupInfo) { s.Error = "Ошибка загрузки обновления" })
			} else {
				batPath, err := a.updater.PrepareSelfUpdate(filepath.Join(a.exeDir, "zpui.exe"), dlPath)
				if err != nil {
					a.log.Error("startup", fmt.Sprintf("prepare update: %v", err))
				} else {
					a.startup.update(func(s *StartupInfo) {
						s.Stage = StageInstall
						s.Progress = 0.65
						s.Sub = "Установка обновления..."
					})
					a.startup.restartBat = batPath
					a.startup.selfUpdated = true
				}
			}
		}
	} else {
		a.startup.update(func(s *StartupInfo) {
			s.Stage = StageModCheck
			s.Progress = 0.35
		})
	}

	if !a.startup.selfUpdated {
		a.runModuleCheck()
	}

	if a.startup.selfUpdated {
		a.startup.update(func(s *StartupInfo) {
			s.Stage = StageRestart
			s.Progress = 1.0
			s.Sub = "Перезапуск..."
		})
		time.Sleep(500 * time.Millisecond)
		if a.startup.restartBat != "" {
			updater.RunBat(a.startup.restartBat)
			os.Exit(0)
		}
	}

	a.startup.mu.Lock()
	a.startup.completed = true
	a.startup.mu.Unlock()
	a.startup.update(func(s *StartupInfo) {
		s.Stage = StageDone
		s.Progress = 1.0
		s.Sub = ""
	})
}

func (a *App) runModuleCheck() {
	discovered := modules.Discover(a.mgr.RootDir())
	var updates []ModUpdateInfo

	for _, dm := range discovered {
		if !dm.EntryOK {
			continue
		}
		if dm.Manifest.UpdateURL == "" {
			continue
		}
		rel, err := a.updater.CheckLatest(context.Background())
		if err != nil {
			a.log.Warn("startup", fmt.Sprintf("module %s update check: %v", dm.Manifest.ID, err))
			continue
		}
		if rel != nil && a.updater.NeedsUpdate(rel) {
			updates = append(updates, ModUpdateInfo{
				ID:      dm.Manifest.ID,
				Name:    dm.Manifest.Name,
				Version: rel.TagName,
			})
		}
	}

	if len(updates) > 0 {
		a.startup.update(func(s *StartupInfo) {
			s.Stage = StageModDL
			s.Progress = 0.65
			s.ModUpdates = updates
			s.Sub = fmt.Sprintf("Обновление модулей (%d)...", len(updates))
		})

		for _, upd := range updates {
			for _, dm := range discovered {
				if dm.Manifest.ID == upd.ID {
					a.downloadModuleUpdate(dm)
					break
				}
			}
		}
	}

	a.startup.update(func(s *StartupInfo) {
		s.Stage = StageInstall
		s.Progress = 0.85
		s.Sub = ""
	})

	if a.cfg.AutoStartMods {
		discovered = modules.Discover(a.mgr.RootDir())
		a.mgr.AutoStartAll(discovered)
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

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
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
