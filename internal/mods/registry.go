package mods

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"zpui/internal/updater"
)

type Placement string

const (
	PlacementSidebar   Placement = "sidebar"
	PlacementDashboard Placement = "dashboard"
	PlacementSettings  Placement = "settings"
)

type ModManifest struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Description string                 `json:"description"`
	Author      string                 `json:"author"`
	Icon        string                 `json:"icon"`
	Color       string                 `json:"color"`
	MinCore     string                 `json:"min_core"`
	Entry       string                 `json:"entry"`
	Placements  []Placement            `json:"placements"`
	Backend     *ModBackend            `json:"backend,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
	Repository  string                 `json:"repository,omitempty"`
	Type        string                 `json:"type,omitempty"`
	dir         string                 `json:"-"`
}

type ModBackend struct {
	Exe      string `json:"exe"`
	Autostart bool   `json:"autostart"`
}

type ModHealth struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	Status   string `json:"status"`
	Exists   bool   `json:"exists"`
	Manifest bool   `json:"manifest"`
	Entry    bool   `json:"entry"`
	Backend  bool   `json:"backend"`
	Message  string `json:"message,omitempty"`
}

type Registry struct {
	modsDir string
	mods    []*ModManifest
	loaded  bool
}

func NewRegistry(modsDir string) *Registry {
	return &Registry{modsDir: modsDir}
}

func (r *Registry) Scan() error {
	r.mods = nil

	entries, err := os.ReadDir(r.modsDir)
	if err != nil {
		if os.IsNotExist(err) {
			os.MkdirAll(r.modsDir, 0755)
			r.loaded = true
			return nil
		}
		return err
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		modDir := filepath.Join(r.modsDir, e.Name())
		manifestPath := filepath.Join(modDir, "mod.json")

		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}

		var m ModManifest
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		if m.ID == "" {
			m.ID = e.Name()
		}
		if m.Entry == "" {
			m.Entry = "index.js"
		}
		m.dir = modDir
		r.mods = append(r.mods, &m)
	}

	sort.Slice(r.mods, func(i, j int) bool {
		return r.mods[i].ID < r.mods[j].ID
	})

	r.loaded = true
	return nil
}

func (r *Registry) Mods() []*ModManifest {
	return r.mods
}

func (r *Registry) GetMod(id string) *ModManifest {
	for _, m := range r.mods {
		if m.ID == id {
			return m
		}
	}
	return nil
}

func (r *Registry) GetEntrySource(id string) (string, error) {
	m := r.GetMod(id)
	if m == nil {
		return "", fmt.Errorf("mod not found: %s", id)
	}
	entryPath := filepath.Join(m.dir, m.Entry)
	data, err := os.ReadFile(entryPath)
	if err != nil {
		return "", fmt.Errorf("entry file not found: %s", m.Entry)
	}
	return string(data), nil
}

func (r *Registry) GetFile(id, filename string) ([]byte, error) {
	m := r.GetMod(id)
	if m == nil {
		return nil, fmt.Errorf("mod not found: %s", id)
	}
	return os.ReadFile(filepath.Join(m.dir, filename))
}

func (r *Registry) HealthCheck() []ModHealth {
	if !r.loaded {
		r.Scan()
	}

	var results []ModHealth

	for _, m := range r.mods {
		h := ModHealth{
			ID:       m.ID,
			Name:     m.Name,
			Version:  m.Version,
			Exists:   true,
			Manifest: true,
		}

		entryPath := filepath.Join(m.dir, m.Entry)
		if _, err := os.Stat(entryPath); err != nil {
			h.Entry = false
			h.Status = "broken"
			h.Message = fmt.Sprintf("Entry file missing: %s", m.Entry)
		} else {
			h.Entry = true
		}

		if m.Backend != nil && m.Backend.Exe != "" {
			exePath := filepath.Join(m.dir, m.Backend.Exe)
			if _, err := os.Stat(exePath); err != nil {
				h.Backend = false
				if h.Status == "" {
					h.Status = "degraded"
					h.Message = fmt.Sprintf("Backend exe missing: %s", m.Backend.Exe)
				}
			} else {
				h.Backend = true
			}
		} else {
			h.Backend = true
		}

		if h.Status == "" {
			h.Status = "healthy"
		}

		results = append(results, h)
	}

	return results
}

func (r *Registry) ModDir(id string) string {
	m := r.GetMod(id)
	if m == nil {
		return ""
	}
	return m.dir
}

func (r *Registry) LastModified(id string) time.Time {
	m := r.GetMod(id)
	if m == nil {
		return time.Time{}
	}
	entryPath := filepath.Join(m.dir, m.Entry)
	info, err := os.Stat(entryPath)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

type ModUpdateResult struct {
	ModManifest
	NeedsUpdate bool   `json:"needs_update"`
	Latest      string `json:"latest,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
	Error       string `json:"error,omitempty"`
}

func (r *Registry) CheckUpdates() []ModUpdateResult {
	var results []ModUpdateResult
	for _, m := range r.mods {
		res := ModUpdateResult{ModManifest: *m}
		info, err := updater.CheckModUpdate(m.ID, m.Name, m.Version, m.Repository)
		if err != nil {
			res.Error = err.Error()
		} else if info != nil {
			res.NeedsUpdate = info.NeedsUpdate
			res.Latest = info.Latest
			res.DownloadURL = info.DownloadURL
		}
		results = append(results, res)
	}
	return results
}
