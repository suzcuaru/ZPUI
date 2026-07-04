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

type XboxDnsConfig struct {
	Enabled      bool   `json:"enabled"`
	PrimaryDNS   string `json:"primary_dns"`
	SecondaryDNS string `json:"secondary_dns"`
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
	ZapretPath      string        `json:"zapret_path"`
	CurrentStrategy string        `json:"current_strategy"`
	ModVersion      string        `json:"mod_version"`
	Proxy           ProxyConfig   `json:"proxy"`
	XboxDns         XboxDnsConfig `json:"xbox_dns"`
	Web             WebConfig     `json:"web"`
	AutoStart       bool          `json:"autostart"`
	Logs            LogConfig     `json:"logs"`
	AutoUpdateCheck bool          `json:"auto_update_check"`
	ZapretRepoURL   string        `json:"zapret_repo_url"`
	ModRepoURL      string        `json:"mod_repo_url"`

	Theme        string `json:"theme"`
	Language     string `json:"language"`
	FirstRunDone bool   `json:"first_run_done"`
	ZapretSkipped bool  `json:"zapret_skipped"`
	StartMinimized bool  `json:"start_minimized"`
	CloseToTray    bool  `json:"close_to_tray"`

	LastZapretState  bool `json:"last_zapret_state"`
	LastProxyState   bool `json:"last_proxy_state"`
	LastXboxDnsState bool `json:"last_xbox_dns_state"`

	AutoStartZapret  bool `json:"auto_start_zapret"`
	AutoStartProxy   bool `json:"auto_start_proxy"`
	AutoStartXboxDns bool `json:"auto_start_xbox_dns"`

	NotificationsEnabled bool `json:"notifications_enabled"`

	NotifyZPUIUpdates   bool `json:"notify_zpui_updates"`
	NotifyZapretUpdates bool `json:"notify_zapret_updates"`
	NotifyMissingFiles  bool `json:"notify_missing_files"`
	NotifyServiceStatus bool `json:"notify_service_status"`
	NotifyResourceDrop  bool `json:"notify_resource_drop"`
	ResourceDropPct     int  `json:"resource_drop_pct"`

	ShowStrategyColors bool `json:"show_strategy_colors"`
	ShowStrategyModal  bool `json:"show_strategy_modal"`
	NotifyStrategyTest bool `json:"notify_strategy_test"`

	DisabledMods []string `json:"disabled_mods"`

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
		XboxDns: XboxDnsConfig{
			Enabled:      false,
			PrimaryDNS:   "111.88.96.50",
			SecondaryDNS: "111.88.96.51",
		},
		Web: WebConfig{
			Port: 8080,
		},
		AutoStart:       false,
		Logs:            LogConfig{MaxFiles: 7, Level: "info"},
		AutoUpdateCheck: true,
		ZapretRepoURL:   "https://github.com/bol-van/zapret",
		ModRepoURL:      "https://github.com/bol-van/zapret",

		Theme:        "system",
		Language:     "ru",
		FirstRunDone: false,
		CloseToTray:  true,

		NotificationsEnabled: true,

		ShowStrategyColors: true,
		NotifyStrategyTest: false,

		NotifyZPUIUpdates:   true,
		NotifyZapretUpdates: true,
		NotifyMissingFiles:  true,
		NotifyServiceStatus: false,
		NotifyResourceDrop:  false,
		ResourceDropPct:     70,
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

	// Migrate: force fixed xbox-dns.ru servers
	cfg.XboxDns.PrimaryDNS = "111.88.96.50"
	cfg.XboxDns.SecondaryDNS = "111.88.96.51"

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

func (c *Config) GetZapretSkipped() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ZapretSkipped
}

func (c *Config) SetZapretSkipped(skipped bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ZapretSkipped = skipped
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

func (c *Config) GetXboxDnsConfig() XboxDnsConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.XboxDns
}

func (c *Config) SetXboxDnsConfig(x XboxDnsConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	x.PrimaryDNS = "111.88.96.50"
	x.SecondaryDNS = "111.88.96.51"
	c.XboxDns = x
	return c.save()
}

func (c *Config) GetTheme() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Theme
}

func (c *Config) GetNotificationsEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.NotificationsEnabled
}

func (c *Config) SetNotificationsEnabled(enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.NotificationsEnabled = enabled
	return c.save()
}

func (c *Config) ShouldNotify(event string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.NotificationsEnabled {
		return false
	}
	switch event {
	case "zpui_update":
		return c.NotifyZPUIUpdates
	case "zapret_update":
		return c.NotifyZapretUpdates
	case "missing_files":
		return c.NotifyMissingFiles
	case "service_status":
		return c.NotifyServiceStatus
	case "resource_drop":
		return c.NotifyResourceDrop
	default:
		return false
	}
}

func (c *Config) GetResourceDropPct() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.ResourceDropPct <= 0 {
		return 70
	}
	return c.ResourceDropPct
}

func (c *Config) SetResourceDropPct(pct int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if pct < 10 {
		pct = 10
	}
	if pct > 100 {
		pct = 100
	}
	c.ResourceDropPct = pct
	return c.save()
}

func (c *Config) SetNotifyFlags(flags map[string]bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := flags["notify_zpui_updates"]; ok {
		c.NotifyZPUIUpdates = v
	}
	if v, ok := flags["notify_zapret_updates"]; ok {
		c.NotifyZapretUpdates = v
	}
	if v, ok := flags["notify_missing_files"]; ok {
		c.NotifyMissingFiles = v
	}
	if v, ok := flags["notify_service_status"]; ok {
		c.NotifyServiceStatus = v
	}
	if v, ok := flags["notify_resource_drop"]; ok {
		c.NotifyResourceDrop = v
	}
	return c.save()
}

func (c *Config) GetLanguage() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Language == "" {
		return "ru"
	}
	return c.Language
}

func (c *Config) GetCloseToTray() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.CloseToTray
}

func (c *Config) SetTheme(theme string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Theme = theme
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
