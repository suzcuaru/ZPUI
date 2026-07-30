package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"zpui/internal/config"
)

func TestStrategyPath(t *testing.T) {
	cfg := &config.Config{ZapretPath: "zapret"}

	cases := []struct {
		name string
		want string
	}{
		{"general", filepath.Join("zapret", "general.bat")},
		{"general.bat", filepath.Join("zapret", "general.bat")},
		{"general (ALT)", filepath.Join("zapret", "general (ALT).bat")},
		{"general1.bat", filepath.Join("zapret", "general1.bat")},
	}
	for _, c := range cases {
		if got := cfg.StrategyPath(c.name); got != c.want {
			t.Errorf("StrategyPath(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestLoadFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "missing.json"), "zapret")

	if cfg.GetZapretPath() != "zapret" {
		t.Errorf("default zapret path = %q, want %q", cfg.GetZapretPath(), "zapret")
	}
	if pc := cfg.GetProxyConfig(); pc.Port != 1080 {
		t.Errorf("default proxy port = %d, want 1080", pc.Port)
	}
	if cfg.GetTheme() != "system" {
		t.Errorf("default theme = %q, want system", cfg.GetTheme())
	}
}

func TestDnsDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")
	xd := cfg.GetXboxDnsConfig()
	if xd.Enabled {
		t.Errorf("default resolver Enabled = true, want false")
	}
	if !xd.XboxEnabled {
		t.Errorf("default XboxEnabled = false, want true")
	}
	if xd.PrimaryDNS != "111.88.96.50" {
		t.Errorf("default PrimaryDNS = %q, want 111.88.96.50", xd.PrimaryDNS)
	}
}

func TestSetXboxDnsConfigNormalizes(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")

	if err := cfg.SetXboxDnsConfig(config.XboxDnsConfig{
		PrimaryDNS: "8.8.8.8",
	}); err != nil {
		t.Fatalf("SetXboxDnsConfig: %v", err)
	}
	xd := cfg.GetXboxDnsConfig()
	if xd.PrimaryDNS != "111.88.96.50" {
		t.Errorf("PrimaryDNS = %q, want 111.88.96.50", xd.PrimaryDNS)
	}
}

func TestSetXboxDnsEnabled(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")
	if err := cfg.SetXboxDnsEnabled(true); err != nil {
		t.Fatalf("SetXboxDnsEnabled: %v", err)
	}
	if !cfg.GetXboxDnsConfig().Enabled {
		t.Errorf("Enabled = false, want true")
	}
}

func TestShouldNotifyUpdatesDependOnAutoUpdateCheck(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")
	cfg.NotificationsEnabled = true

	cfg.AutoUpdateCheck = false
	if cfg.ShouldNotify("zpui_update") || cfg.ShouldNotify("zapret_update") {
		t.Errorf("update notifications should be off when AutoUpdateCheck is off")
	}

	cfg.AutoUpdateCheck = true
	if !cfg.ShouldNotify("zpui_update") || !cfg.ShouldNotify("zapret_update") {
		t.Errorf("update notifications should be on when AutoUpdateCheck is on")
	}
}

// --- Load / Save ---

func TestLoadValidJSONAndMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.json")
	content := `{
		"zapret_path": "C:/custom/zapret",
		"current_strategy": "custom.bat",
		"theme": "dark",
		"language": "en",
		"close_to_tray": false,
		"xbox_dns": {"enabled": true, "primary_dns": "8.8.8.8", "secondary_dns": "8.8.4.4"},
		"proxy": {"port": 9999, "bind_host": "127.0.0.1"},
		"resource_drop_pct": 50
	}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Load(path, "ignored")

	if cfg.GetZapretPath() != "C:/custom/zapret" {
		t.Errorf("zapret path not loaded: %q", cfg.GetZapretPath())
	}
	if cfg.GetCurrentStrategy() != "custom.bat" {
		t.Errorf("strategy not loaded: %q", cfg.GetCurrentStrategy())
	}
	if cfg.GetTheme() != "dark" {
		t.Errorf("theme not loaded: %q", cfg.GetTheme())
	}
	if cfg.GetLanguage() != "en" {
		t.Errorf("language not loaded: %q", cfg.GetLanguage())
	}
	if cfg.GetCloseToTray() {
		t.Errorf("close_to_tray should be loaded as false")
	}
	if cfg.GetProxyConfig().Port != 9999 {
		t.Errorf("proxy port not loaded: %d", cfg.GetProxyConfig().Port)
	}

	xd := cfg.GetXboxDnsConfig()
	if xd.PrimaryDNS != "111.88.96.50" || xd.SecondaryDNS != "111.88.96.51" {
		t.Errorf("DNS migration failed: primary=%s secondary=%s", xd.PrimaryDNS, xd.SecondaryDNS)
	}
	if !xd.Enabled {
		t.Errorf("enabled flag not loaded")
	}
	if cfg.GetResourceDropPct() != 50 {
		t.Errorf("resource_drop_pct not loaded: %d", cfg.GetResourceDropPct())
	}
}

func TestLoadInvalidJSONFallsBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Load(path, "zapret")
	if cfg.GetTheme() != "system" {
		t.Errorf("expected default theme on invalid JSON, got %q", cfg.GetTheme())
	}
	if cfg.GetCurrentStrategy() != "general.bat" {
		t.Errorf("expected default strategy on invalid JSON, got %q", cfg.GetCurrentStrategy())
	}
}

func TestSaveRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.json")
	cfg := config.Load(path, "zapret")

	if err := cfg.SetTheme("dark"); err != nil {
		t.Fatalf("SetTheme: %v", err)
	}
	if err := cfg.SetCurrentStrategy("mystream.bat"); err != nil {
		t.Fatalf("SetCurrentStrategy: %v", err)
	}
	if err := cfg.SetProxyConfig(config.ProxyConfig{Port: 31337, BindHost: "127.0.0.1"}); err != nil {
		t.Fatalf("SetProxyConfig: %v", err)
	}

	cfg2 := config.Load(path, "ignored")
	if cfg2.GetTheme() != "dark" {
		t.Errorf("theme roundtrip failed: %q", cfg2.GetTheme())
	}
	if cfg2.GetCurrentStrategy() != "mystream.bat" {
		t.Errorf("strategy roundtrip failed: %q", cfg2.GetCurrentStrategy())
	}
	if cfg2.GetProxyConfig().Port != 31337 {
		t.Errorf("proxy port roundtrip failed: %d", cfg2.GetProxyConfig().Port)
	}
}

// --- Path / Strategy getters-setters ---

func TestZapretPathAndStrategy(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")

	cfg.SetZapretPath("C:/new/path")
	if cfg.GetZapretPath() != "C:/new/path" {
		t.Errorf("SetZapretPath failed: %q", cfg.GetZapretPath())
	}

	if err := cfg.SetCurrentStrategy("stream.bat"); err != nil {
		t.Fatalf("SetCurrentStrategy: %v", err)
	}
	if cfg.GetCurrentStrategy() != "stream.bat" {
		t.Errorf("GetCurrentStrategy = %q", cfg.GetCurrentStrategy())
	}
}

func TestZapretSkipped(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")
	if cfg.GetZapretSkipped() {
		t.Errorf("default should be false")
	}
	if err := cfg.SetZapretSkipped(true); err != nil {
		t.Fatalf("SetZapretSkipped: %v", err)
	}
	if !cfg.GetZapretSkipped() {
		t.Errorf("should be true after set")
	}
}

func TestProxyConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")
	p := config.ProxyConfig{Enabled: true, AutoStart: true, Port: 2000, BindHost: "1.2.3.4", Username: "u", Password: "p"}
	if err := cfg.SetProxyConfig(p); err != nil {
		t.Fatalf("SetProxyConfig: %v", err)
	}
	got := cfg.GetProxyConfig()
	if !got.Enabled || !got.AutoStart || got.Port != 2000 || got.BindHost != "1.2.3.4" || got.Username != "u" || got.Password != "p" {
		t.Errorf("proxy roundtrip mismatch: %+v", got)
	}
}

// --- Theme / Language / CloseToTray ---

func TestThemeLanguageCloseToTray(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")
	if err := cfg.SetTheme("light"); err != nil {
		t.Fatalf("SetTheme: %v", err)
	}
	if cfg.GetTheme() != "light" {
		t.Errorf("theme set/get failed: %q", cfg.GetTheme())
	}

	// Empty language defaults to "ru".
	cfg.Language = ""
	if got := cfg.GetLanguage(); got != "ru" {
		t.Errorf("empty language should default to ru, got %q", got)
	}
	cfg.Language = "en"
	if got := cfg.GetLanguage(); got != "en" {
		t.Errorf("language get failed: %q", got)
	}

	// Default CloseToTray is true.
	cfg2 := config.Load(filepath.Join(dir, "c2.json"), "zapret")
	if !cfg2.GetCloseToTray() {
		t.Errorf("default CloseToTray should be true")
	}
}

// --- Notifications ---

func TestNotificationsEnabled(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")
	cfg.NotificationsEnabled = false
	if cfg.GetNotificationsEnabled() {
		t.Errorf("expected false")
	}
	if err := cfg.SetNotificationsEnabled(true); err != nil {
		t.Fatalf("SetNotificationsEnabled: %v", err)
	}
	if !cfg.GetNotificationsEnabled() {
		t.Errorf("expected true")
	}
}

func TestShouldNotifyAllEvents(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")

	cfg.NotificationsEnabled = false
	for _, ev := range []string{"zpui_update", "zapret_update", "service_crash", "resource_drop", "errors", "unknown"} {
		if cfg.ShouldNotify(ev) {
			t.Errorf("ShouldNotify(%q) should be false when notifications disabled", ev)
		}
	}

	cfg.NotificationsEnabled = true
	cfg.AutoUpdateCheck = false
	cfg.NotifyServiceCrash = true
	cfg.NotifyResourceDrop = true
	cfg.NotifyErrors = true

	if cfg.ShouldNotify("zpui_update") || cfg.ShouldNotify("zapret_update") {
		t.Errorf("update events should follow AutoUpdateCheck (off)")
	}
	cfg.AutoUpdateCheck = true
	if !cfg.ShouldNotify("zpui_update") || !cfg.ShouldNotify("zapret_update") {
		t.Errorf("update events should be on when AutoUpdateCheck on")
	}
	if !cfg.ShouldNotify("service_crash") {
		t.Errorf("service_crash should be on")
	}
	if !cfg.ShouldNotify("resource_drop") {
		t.Errorf("resource_drop should be on")
	}
	if !cfg.ShouldNotify("errors") {
		t.Errorf("errors should be on")
	}
	if cfg.ShouldNotify("totally_unknown") {
		t.Errorf("unknown event should be false")
	}

	// Per-flag toggling.
	cfg.NotifyServiceCrash = false
	if cfg.ShouldNotify("service_crash") {
		t.Errorf("service_crash should follow its flag (off)")
	}
	cfg.NotifyResourceDrop = false
	if cfg.ShouldNotify("resource_drop") {
		t.Errorf("resource_drop should follow its flag (off)")
	}
	cfg.NotifyErrors = false
	if cfg.ShouldNotify("errors") {
		t.Errorf("errors should follow its flag (off)")
	}
}

func TestSetNotifyFlags(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")
	cfg.NotifyZPUIUpdates = false
	cfg.NotifyZapretUpdates = false
	cfg.NotifyMissingFiles = false
	cfg.NotifyServiceCrash = false
	cfg.NotifyResourceDrop = false

	flags := map[string]bool{
		"notify_zpui_updates":   true,
		"notify_zapret_updates": true,
		"notify_missing_files":  true,
		"notify_service_crash":  true,
		"notify_resource_drop":  true,
	}
	if err := cfg.SetNotifyFlags(flags); err != nil {
		t.Fatalf("SetNotifyFlags: %v", err)
	}
	if !cfg.NotifyZPUIUpdates || !cfg.NotifyZapretUpdates || !cfg.NotifyMissingFiles || !cfg.NotifyServiceCrash || !cfg.NotifyResourceDrop {
		t.Errorf("flags not set: %+v", cfg)
	}

	// Partial update: only one flag flipped, others unchanged.
	if err := cfg.SetNotifyFlags(map[string]bool{"notify_service_crash": false}); err != nil {
		t.Fatalf("SetNotifyFlags partial: %v", err)
	}
	if cfg.NotifyServiceCrash {
		t.Errorf("notify_service_crash should be false after partial update")
	}
	if !cfg.NotifyZPUIUpdates {
		t.Errorf("notify_zpui_updates should remain true")
	}
}

// --- Clamping getters/setters ---

func TestResourceDropPct(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")

	cfg.ResourceDropPct = 0
	if got := cfg.GetResourceDropPct(); got != 70 {
		t.Errorf("clamped default = %d, want 70", got)
	}
	cfg.ResourceDropPct = 50
	if got := cfg.GetResourceDropPct(); got != 50 {
		t.Errorf("get = %d, want 50", got)
	}

	if err := cfg.SetResourceDropPct(5); err != nil {
		t.Fatalf("SetResourceDropPct: %v", err)
	}
	if cfg.ResourceDropPct != 10 {
		t.Errorf("clamp low = %d, want 10", cfg.ResourceDropPct)
	}
	if err := cfg.SetResourceDropPct(200); err != nil {
		t.Fatalf("SetResourceDropPct: %v", err)
	}
	if cfg.ResourceDropPct != 100 {
		t.Errorf("clamp high = %d, want 100", cfg.ResourceDropPct)
	}
	if err := cfg.SetResourceDropPct(42); err != nil {
		t.Fatalf("SetResourceDropPct: %v", err)
	}
	if cfg.ResourceDropPct != 42 {
		t.Errorf("normal = %d, want 42", cfg.ResourceDropPct)
	}
}

func TestResourceCheckInterval(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")

	cfg.ResourceCheckInterval = 0
	if got := cfg.GetResourceCheckInterval(); got != 10 {
		t.Errorf("getter low clamp = %d, want 10", got)
	}
	cfg.ResourceCheckInterval = 100
	if got := cfg.GetResourceCheckInterval(); got != 60 {
		t.Errorf("getter high clamp = %d, want 60", got)
	}
	cfg.ResourceCheckInterval = 30
	if got := cfg.GetResourceCheckInterval(); got != 30 {
		t.Errorf("getter normal = %d, want 30", got)
	}

	if err := cfg.SetResourceCheckInterval(1); err != nil {
		t.Fatalf("SetResourceCheckInterval: %v", err)
	}
	if cfg.ResourceCheckInterval != 5 {
		t.Errorf("setter low = %d, want 5", cfg.ResourceCheckInterval)
	}
	if err := cfg.SetResourceCheckInterval(100); err != nil {
		t.Fatalf("SetResourceCheckInterval: %v", err)
	}
	if cfg.ResourceCheckInterval != 60 {
		t.Errorf("setter high = %d, want 60", cfg.ResourceCheckInterval)
	}
	if err := cfg.SetResourceCheckInterval(15); err != nil {
		t.Fatalf("SetResourceCheckInterval: %v", err)
	}
	if cfg.ResourceCheckInterval != 15 {
		t.Errorf("setter normal = %d, want 15", cfg.ResourceCheckInterval)
	}
}

// --- LastNotifiedVersion ---

func TestLastNotifiedVersion(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")

	if cfg.GetLastNotifiedVersion("ZPUI") != "" {
		t.Errorf("default ZPUI should be empty")
	}
	if cfg.GetLastNotifiedVersion("zapret") != "" {
		t.Errorf("default zapret should be empty")
	}
	if cfg.GetLastNotifiedVersion("unknown") != "" {
		t.Errorf("unknown component should be empty")
	}

	if err := cfg.SetLastNotifiedVersion("ZPUI", "1.2.3"); err != nil {
		t.Fatalf("SetLastNotifiedVersion ZPUI: %v", err)
	}
	if got := cfg.GetLastNotifiedVersion("ZPUI"); got != "1.2.3" {
		t.Errorf("ZPUI = %q, want 1.2.3", got)
	}
	if err := cfg.SetLastNotifiedVersion("zapret", "9.9.9"); err != nil {
		t.Fatalf("SetLastNotifiedVersion zapret: %v", err)
	}
	if got := cfg.GetLastNotifiedVersion("zapret"); got != "9.9.9" {
		t.Errorf("zapret = %q, want 9.9.9", got)
	}
	// Unknown component: no error, no save.
	if err := cfg.SetLastNotifiedVersion("bogus", "1.0"); err != nil {
		t.Errorf("unknown component should not error: %v", err)
	}
	if cfg.GetLastNotifiedVersion("bogus") != "" {
		t.Errorf("bogus should remain empty")
	}
}

// --- Dirs ---

func TestDirs(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapretroot")

	if base := filepath.Base(cfg.ListsDir()); base != "lists" {
		t.Errorf("ListsDir base = %q, want lists", base)
	}
	if base := filepath.Base(cfg.BinDir()); base != "bin" {
		t.Errorf("BinDir base = %q, want bin", base)
	}
	if base := filepath.Base(cfg.LogsDir()); base != "logs" {
		t.Errorf("LogsDir base = %q, want logs", base)
	}
	// LogsDir lives next to config.json.
	if filepath.Dir(cfg.LogsDir()) != dir {
		t.Errorf("LogsDir parent = %q, want %q", filepath.Dir(cfg.LogsDir()), dir)
	}
}

// --- BlockCheckConfig ---

func TestBlockCheckConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")
	if cfg.GetBlockCheckConfig().TimeoutSec != 8 {
		t.Errorf("default timeout = %d, want 8", cfg.GetBlockCheckConfig().TimeoutSec)
	}

	// Invalid timeout → normalized to 8.
	if err := cfg.SetBlockCheckConfig(config.BlockCheckConfig{CheckTCP: true, CheckTLS: true, CheckHTTP: true, TimeoutSec: 0}); err != nil {
		t.Fatalf("SetBlockCheckConfig: %v", err)
	}
	got := cfg.GetBlockCheckConfig()
	if !got.CheckTCP || !got.CheckTLS || !got.CheckHTTP || got.TimeoutSec != 8 {
		t.Errorf("blockcheck mismatch: %+v", got)
	}

	// Valid timeout preserved.
	if err := cfg.SetBlockCheckConfig(config.BlockCheckConfig{TimeoutSec: 15}); err != nil {
		t.Fatalf("SetBlockCheckConfig: %v", err)
	}
	if cfg.GetBlockCheckConfig().TimeoutSec != 15 {
		t.Errorf("timeout = %d, want 15", cfg.GetBlockCheckConfig().TimeoutSec)
	}
}

// --- Skip resources ---

func TestGetSkipResources(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")

	if got := cfg.GetSkipResources(); len(got) != 0 {
		t.Errorf("expected empty when no file, got %v", got)
	}
	if base := filepath.Base(cfg.GetSkipResourcesFilePath()); base != "skip-resources.txt" {
		t.Errorf("skip file path base = %q, want skip-resources.txt", base)
	}

	content := "# comment line\n\nexample.com\nCDN.GOOGLE.COM\n  \n#another\n"
	if err := os.WriteFile(cfg.GetSkipResourcesFilePath(), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	got := cfg.GetSkipResources()
	want := map[string]bool{"example.com": true, "cdn.google.com": true}
	if len(got) != 2 {
		t.Fatalf("expected 2 hosts (lowercased, comments/blank skipped), got %v", got)
	}
	for _, h := range got {
		if !want[h] {
			t.Errorf("unexpected host %q", h)
		}
	}
}

func TestIsSkippedResource(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")
	if err := os.WriteFile(cfg.GetSkipResourcesFilePath(), []byte("google.com\nbad.example\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		host string
		want bool
	}{
		{"", false},
		{"   ", false},
		{"google.com", true},
		{"GOOGLE.COM", true},
		{"drive.google.com", true},
		{"nogoogle.com", true},   // Contains-match (production behaviour).
		{"example.org", false},
		{"different.example", false},
	}
	for _, c := range cases {
		if got := cfg.IsSkippedResource(c.host); got != c.want {
			t.Errorf("IsSkippedResource(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestAddSkipResource(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")

	if err := cfg.AddSkipResource("   "); err == nil {
		t.Error("expected error for empty host")
	}

	if err := cfg.AddSkipResource("NewHost.com"); err != nil {
		t.Fatalf("AddSkipResource: %v", err)
	}
	got := cfg.GetSkipResources()
	found := false
	for _, h := range got {
		if h == "newhost.com" {
			found = true
		}
	}
	if !found {
		t.Errorf("newhost.com (lowercased) not added, got %v", got)
	}

	// Duplicate (case-insensitive) is a no-op.
	before := len(got)
	if err := cfg.AddSkipResource("NEWHOST.COM"); err != nil {
		t.Fatalf("duplicate add: %v", err)
	}
	if after := len(cfg.GetSkipResources()); after != before {
		t.Errorf("duplicate should not increase count: before=%d after=%d", before, after)
	}
}

// --- Service crash ---

func TestServiceCrash(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")
	if cfg.GetServiceCrashCount() != 0 {
		t.Errorf("default crash count != 0")
	}
	if cfg.GetServiceLastCrash() != 0 {
		t.Errorf("default last crash != 0")
	}
	if err := cfg.SetServiceCrashCount(3); err != nil {
		t.Fatalf("SetServiceCrashCount: %v", err)
	}
	if cfg.GetServiceCrashCount() != 3 {
		t.Errorf("crash count = %d, want 3", cfg.GetServiceCrashCount())
	}
	if err := cfg.SetServiceLastCrash(999); err != nil {
		t.Fatalf("SetServiceLastCrash: %v", err)
	}
	if cfg.GetServiceLastCrash() != 999 {
		t.Errorf("last crash = %d, want 999", cfg.GetServiceLastCrash())
	}
}

// --- AutoSwitch ---

func TestAutoSwitch(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")

	if cfg.GetAutoSwitchEnabled() {
		t.Errorf("default enabled should be false")
	}
	if err := cfg.SetAutoSwitchEnabled(true); err != nil {
		t.Fatalf("SetAutoSwitchEnabled: %v", err)
	}
	if !cfg.GetAutoSwitchEnabled() {
		t.Errorf("enabled should be true")
	}

	// Threshold getter clamps to 50 outside [10,100].
	cfg.AutoSwitchThresholdPct = 5
	if got := cfg.GetAutoSwitchThresholdPct(); got != 50 {
		t.Errorf("getter low clamp = %d, want 50", got)
	}
	cfg.AutoSwitchThresholdPct = 200
	if got := cfg.GetAutoSwitchThresholdPct(); got != 50 {
		t.Errorf("getter high clamp = %d, want 50", got)
	}
	cfg.AutoSwitchThresholdPct = 60
	if got := cfg.GetAutoSwitchThresholdPct(); got != 60 {
		t.Errorf("getter normal = %d, want 60", got)
	}
	// Threshold setter clamps to [10,100].
	if err := cfg.SetAutoSwitchThresholdPct(1); err != nil {
		t.Fatalf("SetAutoSwitchThresholdPct: %v", err)
	}
	if cfg.AutoSwitchThresholdPct != 10 {
		t.Errorf("setter low = %d, want 10", cfg.AutoSwitchThresholdPct)
	}
	if err := cfg.SetAutoSwitchThresholdPct(500); err != nil {
		t.Fatalf("SetAutoSwitchThresholdPct: %v", err)
	}
	if cfg.AutoSwitchThresholdPct != 100 {
		t.Errorf("setter high = %d, want 100", cfg.AutoSwitchThresholdPct)
	}

	// Interval getter clamps to [10,60] for out-of-range at low, [1,60] generally.
	cfg.AutoSwitchIntervalMin = 0
	if got := cfg.GetAutoSwitchIntervalMin(); got != 10 {
		t.Errorf("getter low clamp = %d, want 10", got)
	}
	cfg.AutoSwitchIntervalMin = 100
	if got := cfg.GetAutoSwitchIntervalMin(); got != 60 {
		t.Errorf("getter high clamp = %d, want 60", got)
	}
	cfg.AutoSwitchIntervalMin = 20
	if got := cfg.GetAutoSwitchIntervalMin(); got != 20 {
		t.Errorf("getter normal = %d, want 20", got)
	}
	// Interval setter clamps to [1,60].
	if err := cfg.SetAutoSwitchIntervalMin(0); err != nil {
		t.Fatalf("SetAutoSwitchIntervalMin: %v", err)
	}
	if cfg.AutoSwitchIntervalMin != 1 {
		t.Errorf("setter low = %d, want 1", cfg.AutoSwitchIntervalMin)
	}
	if err := cfg.SetAutoSwitchIntervalMin(70); err != nil {
		t.Fatalf("SetAutoSwitchIntervalMin: %v", err)
	}
	if cfg.AutoSwitchIntervalMin != 60 {
		t.Errorf("setter high = %d, want 60", cfg.AutoSwitchIntervalMin)
	}
	if err := cfg.SetAutoSwitchIntervalMin(15); err != nil {
		t.Fatalf("SetAutoSwitchIntervalMin: %v", err)
	}
	if cfg.AutoSwitchIntervalMin != 15 {
		t.Errorf("setter normal = %d, want 15", cfg.AutoSwitchIntervalMin)
	}
}

func TestLastAutoSelectTime(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")
	if cfg.GetLastAutoSelectTime() != "" {
		t.Errorf("default should be empty")
	}
	if err := cfg.SetLastAutoSelectTime("2024-01-01T00:00:00Z"); err != nil {
		t.Fatalf("SetLastAutoSelectTime: %v", err)
	}
	if cfg.GetLastAutoSelectTime() != "2024-01-01T00:00:00Z" {
		t.Errorf("time mismatch: %q", cfg.GetLastAutoSelectTime())
	}
}
