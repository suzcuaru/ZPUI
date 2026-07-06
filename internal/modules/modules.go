package modules

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type Manifest struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Author      string   `json:"author,omitempty"`
	Description string   `json:"description,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	Entry       string   `json:"entry"`
	Args        []string `json:"args,omitempty"`
	AutoStart   bool     `json:"auto_start,omitempty"`
	Placements  []string `json:"placements,omitempty"`
	MinApp      string   `json:"min_app,omitempty"`

	dir string `json:"-"`
}

func (m *Manifest) Dir() string      { return m.dir }
func (m *Manifest) EntryPath() string { return filepath.Join(m.dir, m.Entry) }
func (m *Manifest) IconPath() string {
	if m.Icon == "" {
		return ""
	}
	return filepath.Join(m.dir, m.Icon)
}
func (m *Manifest) HasEntry() bool {
	if m.Entry == "" {
		return false
	}
	_, err := os.Stat(m.EntryPath())
	return err == nil
}

type DiscoveredModule struct {
	Manifest  *Manifest `json:"manifest"`
	Dir       string    `json:"dir"`
	EntryOK   bool      `json:"entry_ok"`
	EntryName string    `json:"entry_name"`
}

func Discover(rootDir string) []*DiscoveredModule {
	var found []*DiscoveredModule
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return found
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		manifestPath := filepath.Join(rootDir, e.Name(), "module.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		data = stripBOM(data)
		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			found = append(found, &DiscoveredModule{
				Manifest: &Manifest{Name: e.Name(), ID: e.Name()},
				Dir:      filepath.Join(rootDir, e.Name()),
				EntryOK:  false,
			})
			continue
		}
		m.dir = filepath.Join(rootDir, e.Name())
		if m.ID == "" {
			m.ID = e.Name()
		}
		if m.Name == "" {
			m.Name = m.ID
		}
		found = append(found, &DiscoveredModule{
			Manifest:  &m,
			Dir:       m.dir,
			EntryOK:   m.HasEntry(),
			EntryName: m.Entry,
		})
	}
	sort.Slice(found, func(i, j int) bool {
		return found[i].Manifest.Name < found[j].Manifest.Name
	})
	return found
}

func ReadStatusFile(modDir string) map[string]interface{} {
	data, err := os.ReadFile(filepath.Join(modDir, "status.json"))
	if err != nil {
		return nil
	}
	var s map[string]interface{}
	if err := json.Unmarshal(data, &s); err != nil {
		return nil
	}
	return s
}

func WriteStatusFile(modDir string, status map[string]interface{}) error {
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(modDir, "status.json"), data, 0644)
}

func stripBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

func EnsureModulesDir(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create modules dir: %w", err)
	}
	return nil
}
