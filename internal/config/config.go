package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Config struct {
	mu             sync.RWMutex
	configPath     string
	AppVersion     string   `json:"app_version"`
	Theme          string   `json:"theme"`
	Language       string   `json:"language"`
	StartMinimized bool     `json:"start_minimized"`
	CloseToTray    bool     `json:"close_to_tray"`
	AutoStartMods  bool     `json:"auto_start_mods"`
	DisabledMods   []string `json:"disabled_mods"`
	Verbose        bool     `json:"verbose,omitempty"`
	DisableUpdates bool     `json:"disable_updates,omitempty"`
}

func defaultConfig() *Config {
	return &Config{
		Theme:          "system",
		Language:       "ru",
		StartMinimized: false,
		CloseToTray:    true,
		AutoStartMods:  false,
		DisabledMods:   []string{},
		Verbose:        false,
		DisableUpdates: false,
	}
}

func Load(configPath string) *Config {
	cfg := defaultConfig()
	cfg.configPath = configPath

	if data, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			cfg = defaultConfig()
			cfg.configPath = configPath
		}
	}
	cfg.Save()
	return cfg
}

func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.save()
}

func (c *Config) save() error {
	if c.configPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.configPath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.configPath, data, 0644)
}

func (c *Config) GetTheme() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Theme
}

func (c *Config) GetLanguage() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Language == "" {
		return "ru"
	}
	return c.Language
}

func (c *Config) LogsDir() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.configPath == "" {
		return "logs"
	}
	return filepath.Join(filepath.Dir(c.configPath), "logs")
}

func (c *Config) IsModDisabled(id string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, d := range c.DisabledMods {
		if d == id {
			return true
		}
	}
	return false
}

func (c *Config) SetModDisabled(id string, disabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	set := make(map[string]bool)
	for _, d := range c.DisabledMods {
		set[d] = true
	}
	if disabled {
		set[id] = true
	} else {
		delete(set, id)
	}
	c.DisabledMods = c.DisabledMods[:0]
	for k := range set {
		c.DisabledMods = append(c.DisabledMods, k)
	}
	return c.save()
}

func (c *Config) GetVerbose() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Verbose
}

func (c *Config) SetVerbose(v bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Verbose = v
	return c.save()
}

func (c *Config) GetDisableUpdates() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.DisableUpdates
}

func (c *Config) SetDisableUpdates(v bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.DisableUpdates = v
	return c.save()
}

func (c *Config) Apply(patch map[string]interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := patch["theme"].(string); ok {
		c.Theme = v
	}
	if v, ok := patch["language"].(string); ok {
		c.Language = v
	}
	if v, ok := patch["start_minimized"].(bool); ok {
		c.StartMinimized = v
	}
	if v, ok := patch["close_to_tray"].(bool); ok {
		c.CloseToTray = v
	}
	if v, ok := patch["auto_start_mods"].(bool); ok {
		c.AutoStartMods = v
	}
	if v, ok := patch["verbose"].(bool); ok {
		c.Verbose = v
	}
	if v, ok := patch["disable_updates"].(bool); ok {
		c.DisableUpdates = v
	}
	return c.save()
}
