package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type ProxyConfig struct {
	Enabled   bool   `json:"enabled"`
	AutoStart bool   `json:"auto_start"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	Password  string `json:"password"`
}

type WebConfig struct {
	Port int `json:"port"`
}

type LogConfig struct {
	MaxFiles int    `json:"max_files"`
	Level    string `json:"level"`
}

type Config struct {
	mu              sync.RWMutex
	ZapretPath      string      `json:"zapret_path"`
	CurrentStrategy string      `json:"current_strategy"`
	ModVersion      string      `json:"mod_version"`
	Proxy           ProxyConfig `json:"proxy"`
	Web             WebConfig   `json:"web"`
	AutoStart       bool        `json:"autostart"`
	Logs            LogConfig   `json:"logs"`
	AutoUpdateCheck bool        `json:"auto_update_check"`
	ZapretRepoURL   string      `json:"zapret_repo_url"`
	ModRepoURL      string      `json:"mod_repo_url"`

	configPath string
}

func defaultConfig(zapretDir string) *Config {
	return &Config{
		ZapretPath:      zapretDir,
		CurrentStrategy: "general.bat",
		Proxy: ProxyConfig{
			Enabled:   false,
			AutoStart: false,
			Port:      1080,
			Username:  "",
			Password:  "",
		},
		Web: WebConfig{
			Port: 8080,
		},
		AutoStart:       false,
		Logs:            LogConfig{MaxFiles: 7, Level: "info"},
		AutoUpdateCheck: true,
		ZapretRepoURL:   "https://github.com/bol-van/zapret",
		ModRepoURL:      "https://github.com/bol-van/zapret",
	}
}

func Load(configPath, zapretDir string) *Config {
	cfg := defaultConfig(zapretDir)
	cfg.configPath = configPath

	data, err := os.ReadFile(configPath)
	if err != nil {
		cfg.Save()
		return cfg
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		cfg.Save()
		return cfg
	}

	cfg.configPath = configPath
	return cfg
}

func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.save()
}

func (c *Config) save() error {
	dir := filepath.Dir(c.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return os.WriteFile(c.configPath, data, 0644)
}

func (c *Config) GetZapretPath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ZapretPath
}

func (c *Config) SetZapretPath(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ZapretPath = path
}

func (c *Config) GetCurrentStrategy() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.CurrentStrategy
}

func (c *Config) SetCurrentStrategy(strategy string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.CurrentStrategy = strategy
	return c.save()
}

func (c *Config) GetProxyConfig() ProxyConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Proxy
}

func (c *Config) SetProxyConfig(p ProxyConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Proxy = p
	return c.save()
}

func (c *Config) ListsDir() string {
	return filepath.Join(c.GetZapretPath(), "lists")
}

func (c *Config) BinDir() string {
	return filepath.Join(c.GetZapretPath(), "bin")
}

func (c *Config) LogsDir() string {
	return filepath.Join(filepath.Dir(c.configPath), "logs")
}

func (c *Config) StrategyPath(name string) string {
	if !strings.HasSuffix(name, ".bat") {
		name += ".bat"
	}
	return filepath.Join(c.GetZapretPath(), name)
}
