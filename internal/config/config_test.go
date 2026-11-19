package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
	// Дефолт — чистый лист: ни пресет Xbox, ни провайдеры не включены,
	// в систему по умолчанию ничего не прописывается.
	if xd.XboxEnabled {
		t.Errorf("default XboxEnabled = true, want false")
	}
	if xd.Providers == nil || len(xd.Providers) != 0 {
		t.Errorf("default Providers = %v, want empty map", xd.Providers)
	}
	if len(xd.Custom) != 0 {
		t.Errorf("default Custom = %+v, want empty", xd.Custom)
	}
	if eff := xd.EffectiveServers(); len(eff) != 0 {
		t.Errorf("EffectiveServers on defaults = %+v, want none", eff)
	}
	if ips := xd.PlainIPs(); len(ips) != 0 {
		t.Errorf("PlainIPs on defaults = %v, want none", ips)
	}

	// Персистентность: включённый провайдер + запись + пресет переживают
	// Save → Load; легаси-таблица не resurrect'ится.
	xd.Providers["google"] = true
	xd.XboxEnabled = true
	xd.Custom = append(xd.Custom, config.CustomDns{ID: "c1", Name: "Мой DNS", Primary: "9.9.9.9", Enabled: true})
	if err := cfg.SetXboxDnsConfig(xd); err != nil {
		t.Fatal(err)
	}
	xd.Servers = append(xd.Servers, config.DnsServer{Addr: "1.0.0.1", Type: "plain", Enabled: false})
	if err := cfg.SetXboxDnsConfig(xd); err != nil {
		t.Fatal(err)
	}
	cfg2 := config.Load(filepath.Join(dir, "c.json"), "zapret")
	xd2 := cfg2.GetXboxDnsConfig()
	if !xd2.Providers["google"] {
		t.Errorf("provider lost after reload: %+v", xd2.Providers)
	}
	// пресет Xbox выведен из базовых провайдеров: флаг глушится при Load
	if xd2.XboxEnabled {
		t.Errorf("XboxEnabled must be forced off by Load, got true")
	}
	if len(xd2.Custom) != 1 || xd2.Custom[0].Primary != "9.9.9.9" || !xd2.Custom[0].Enabled {
		t.Errorf("Custom lost after reload: %+v", xd2.Custom)
	}
	ips := xd2.PlainIPs()
	want := []string{"8.8.8.8", "8.8.4.4", "9.9.9.9"}
	if len(ips) != len(want) {
		t.Fatalf("PlainIPs = %v, want %v", ips, want)
	}
	for i, a := range want {
		if ips[i] != a {
			t.Errorf("PlainIPs[%d] = %s, want %s", i, ips[i], a)
		}
	}
}

func TestSetXboxDnsConfigNormalizes(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")

	// Валидные записи сохраняются, невалидные — отбрасываются.
	if err := cfg.SetXboxDnsConfig(config.XboxDnsConfig{
		Servers: []config.DnsServer{
			{Addr: "8.8.8.8", Type: "plain", Enabled: true},
			{Addr: "999.1.1.1", Type: "plain", Enabled: true},
			{Addr: "dns.quad9.net", Type: "dot", Enabled: true},
			{Addr: "https://dns.google/dns-query", Type: "doh", Enabled: true},
			{Addr: "not a host", Type: "dot", Enabled: true},
		},
	}); err != nil {
		t.Fatalf("SetXboxDnsConfig: %v", err)
	}
	xd := cfg.GetXboxDnsConfig()
	if len(xd.Servers) != 3 {
		t.Fatalf("Servers = %+v, want 3 valid entries kept", xd.Servers)
	}
	if xd.Servers[0].Addr != "8.8.8.8" {
		t.Errorf("Servers[0] = %+v, want 8.8.8.8 kept", xd.Servers[0])
	}
	if xd.Servers[1].Type != "dot" || xd.Servers[2].Type != "doh" {
		t.Errorf("dot/doh entries lost: %+v", xd.Servers)
	}
	// Тип записи больше не влияет на применение: IP у записи любого типа
	// попадает в plain-список для системы.
	ips := xd.PlainIPs()
	if len(ips) != 1 || ips[0] != "8.8.8.8" {
		t.Errorf("PlainIPs = %v, want [8.8.8.8] (type ignored)", ips)
	}
}

func TestDnsPreviousRoundtrip(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")
	if got := cfg.GetXboxDnsPrevious(); len(got) != 0 {
		t.Errorf("default Previous = %v, want empty", got)
	}
	snap := []string{"Ethernet|4|192.168.1.1", "Ethernet|6|dhcp"}
	if err := cfg.SetXboxDnsPrevious(snap); err != nil {
		t.Fatalf("SetXboxDnsPrevious: %v", err)
	}
	if got := cfg.GetXboxDnsPrevious(); len(got) != 2 || got[0] != snap[0] {
		t.Errorf("Previous = %v, want %v", got, snap)
	}
	if err := cfg.SetXboxDnsPrevious(nil); err != nil {
		t.Fatalf("SetXboxDnsPrevious(nil): %v", err)
	}
	if got := cfg.GetXboxDnsPrevious(); len(got) != 0 {
		t.Errorf("Previous after reset = %v, want empty", got)
	}
}

// Регрессия бага «Load() перетирает пользовательские адреса на 111.88.96.50/.51»:
// старые конфиги с сервисными адресами в легаси-полях мигрируют в пресет
// (xbox_enabled), пользовательская таблица переносится в модель провайдеров;
// после Load() повторное чтение не меняет пользовательские адреса.
func TestDnsLegacyMigrationAndNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.json")

	// 1. Легаси-конфиг с СЕРВИСНЫМИ адресами (в т.ч. перетыми багом .50/.51):
	// в записи ничего не попадает, пресет остаётся включённым. Ключ set_ipv6
	// из старых конфигов просто игнорируется (поля IPv6 больше нет).
	old := `{"xbox_dns": {"enabled": true, "xbox_enabled": true, "primary_dns": "111.88.96.50", "secondary_dns": "111.88.96.51", "set_ipv6": true}}`
	if err := os.WriteFile(path, []byte(old), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Load(path, "zapret")
	xd := cfg.GetXboxDnsConfig()
	for _, c := range xd.Custom {
		if c.Primary == "111.88.96.50" || c.Primary == "111.88.96.51" {
			t.Errorf("service address %s must not be imported into custom entries", c.Primary)
		}
	}
	if len(xd.Servers) != 0 {
		t.Errorf("legacy table must be cleared by provider migration, got %v", xd.Servers)
	}
	// пресет Xbox выведен из базовых провайдеров: Load глушит флаг даже
	// для легаси-конфигов с сервисными адресами
	if xd.XboxEnabled {
		t.Errorf("preset must be forced off by Load, got true")
	}

	// 2. Пользовательские адреса в легаси-полях: становятся записями Custom
	// (включёнными — master в этом конфиге применён).
	custom := `{"xbox_dns": {"enabled": true, "primary_dns": "8.8.8.8", "secondary_dns": "1.1.1.1"}}`
	if err := os.WriteFile(path, []byte(custom), 0644); err != nil {
		t.Fatal(err)
	}
	cfg = config.Load(path, "zapret")
	xd = cfg.GetXboxDnsConfig()
	if len(xd.Custom) != 2 || xd.Custom[0].Primary != "8.8.8.8" || xd.Custom[1].Primary != "1.1.1.1" {
		t.Fatalf("custom legacy addresses must migrate to Custom, got %+v", xd.Custom)
	}
	if !xd.Custom[0].Enabled {
		t.Errorf("migrated entries must be enabled while master applied, got %+v", xd.Custom[0])
	}

	// 3. Повторный Load не теряет и не подменяет записи.
	cfg2 := config.Load(path, "zapret")
	xd2 := cfg2.GetXboxDnsConfig()
	if len(xd2.Custom) != 2 || xd2.Custom[0].Primary != "8.8.8.8" {
		t.Errorf("reload must preserve custom entries, got %+v", xd2.Custom)
	}

	// 4. Пользовательские адреса переживают Save + Load (суть багфикса).
	if err := cfg.SetXboxDnsConfig(config.XboxDnsConfig{
		XboxEnabled: true,
		Custom:      []config.CustomDns{{ID: "c9", Name: "Quad9", Primary: "9.9.9.9", Enabled: true}},
	}); err != nil {
		t.Fatalf("SetXboxDnsConfig: %v", err)
	}
	cfg3 := config.Load(path, "zapret")
	xd3 := cfg3.GetXboxDnsConfig()
	if len(xd3.Custom) != 1 || xd3.Custom[0].Primary != "9.9.9.9" {
		t.Errorf("custom entry overwritten after reload: %+v (bug regression)", xd3.Custom)
	}
	ips := xd3.PlainIPs()
	want := []string{"9.9.9.9"}
	if len(ips) != len(want) || ips[0] != "9.9.9.9" {
		t.Errorf("PlainIPs should contain only custom addresses (preset forced off), got %v", ips)
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
	xd := cfg.GetXboxDnsConfig()
	// Легаси-адреса из старого конфига мигрируют в модель провайдеров:
	// пара 8.8.8.8/8.8.4.4 — точное множество Google.
	if !xd.Providers["google"] || len(xd.Custom) != 0 {
		t.Errorf("DNS legacy migration failed: providers=%v custom=%+v", xd.Providers, xd.Custom)
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

	cfg2 := config.Load(path, "ignored")
	if cfg2.GetTheme() != "dark" {
		t.Errorf("theme roundtrip failed: %q", cfg2.GetTheme())
	}
	if cfg2.GetCurrentStrategy() != "mystream.bat" {
		t.Errorf("strategy roundtrip failed: %q", cfg2.GetCurrentStrategy())
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

func TestTgProxyConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")
	// Get генерирует secret автоматически и сохраняет.
	got := cfg.GetTgProxyConfig()
	if len(got.Secret) != 32 {
		t.Fatalf("expected 32-hex secret, got %q", got.Secret)
	}
	if got.Port != 1443 || got.BindHost != "0.0.0.0" {
		t.Fatalf("unexpected defaults: %+v", got)
	}
	// Повторное чтение возвращает тот же secret (стабильность).
	if cfg.GetTgProxyConfig().Secret != got.Secret {
		t.Errorf("secret changed between reads")
	}
	// Round-trip set.
	tp := config.TelegramProxyConfig{Enabled: true, AutoStart: true, Port: 1444, BindHost: "0.0.0.0", Secret: "abcdef0123456789abcdef0123456789"}
	if err := cfg.SetTgProxyConfig(tp); err != nil {
		t.Fatalf("SetTgProxyConfig: %v", err)
	}
	g2 := cfg.GetTgProxyConfig()
	if !g2.Enabled || g2.Port != 1444 || g2.BindHost != "0.0.0.0" || g2.Secret != tp.Secret {
		t.Errorf("tgproxy roundtrip mismatch: %+v", g2)
	}
	// Пустые port/bindhost при set должны нормализоваться к дефолтам.
	if err := cfg.SetTgProxyConfig(config.TelegramProxyConfig{Secret: "00000000000000000000000000000000"}); err != nil {
		t.Fatalf("SetTgProxyConfig defaults: %v", err)
	}
	g3 := cfg.GetTgProxyConfig()
	if g3.Port != 1443 || g3.BindHost != "0.0.0.0" {
		t.Errorf("defaults not normalized: %+v", g3)
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
		{"nogoogle.com", true}, // Contains-match (production behaviour).
		{"example.org", false},
		{"different.example", false},
	}
	for _, c := range cases {
		if got := cfg.IsSkippedResource(c.host); got != c.want {
			t.Errorf("IsSkippedResource(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

// Системный skip-файл (skip-resources-system.txt) учитывается гейтом наравне
// с пользовательским, но в редакторе UI (GetSkipResources) не отображается.
func TestIsSkippedResourceSystemFile(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")
	sysPath := cfg.GetSkipResourcesSystemFilePath()
	if filepath.Base(sysPath) != "skip-resources-system.txt" {
		t.Fatalf("system skip path base = %q, want skip-resources-system.txt", filepath.Base(sysPath))
	}
	if err := os.WriteFile(sysPath, []byte("# system header\n\nbanned.example\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if !cfg.IsSkippedResource("Sub.Banned.Example") {
		t.Error("IsSkippedResource should match domain from system skip file")
	}
	if cfg.IsSkippedResource("other.example") {
		t.Error("IsSkippedResource matched host absent from both files")
	}
	if got := cfg.GetSkipResources(); len(got) != 0 {
		t.Errorf("GetSkipResources should return only the user file, got %v", got)
	}
	got := cfg.GetSkipResourcesSystem()
	if len(got) != 1 || got[0] != "banned.example" {
		t.Errorf("GetSkipResourcesSystem = %v, want [banned.example]", got)
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

	if !cfg.GetAutoSwitchEnabled() {
		t.Errorf("default enabled should be true (auto-switch on by default)")
	}
	if err := cfg.SetAutoSwitchEnabled(true); err != nil {
		t.Fatalf("SetAutoSwitchEnabled: %v", err)
	}
	if !cfg.GetAutoSwitchEnabled() {
		t.Errorf("enabled should be true")
	}

	// Threshold getter clamps to 50 outside [50,80].
	cfg.AutoSwitchThresholdPct = 30
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
	// Threshold setter clamps to [50,80].
	if err := cfg.SetAutoSwitchThresholdPct(10); err != nil {
		t.Fatalf("SetAutoSwitchThresholdPct: %v", err)
	}
	if cfg.AutoSwitchThresholdPct != 50 {
		t.Errorf("setter low = %d, want 50", cfg.AutoSwitchThresholdPct)
	}
	if err := cfg.SetAutoSwitchThresholdPct(500); err != nil {
		t.Fatalf("SetAutoSwitchThresholdPct: %v", err)
	}
	if cfg.AutoSwitchThresholdPct != 80 {
		t.Errorf("setter high = %d, want 80", cfg.AutoSwitchThresholdPct)
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

// Старый конфиг (до 1.8.0): автозапуск Xbox DNS явно выключен, маркера
// миграции нет. После Load миграция должна один раз включить автозапуск.
func TestLoadMigratesDNSAutostartDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	old := `{"auto_start_xbox_dns": false, "theme": "dark"}`
	if err := os.WriteFile(path, []byte(old), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Load(path, "zapret")
	if !cfg.AutoStartXboxDns {
		t.Error("migration should enable auto_start_xbox_dns for pre-1.8.0 configs")
	}
	if !cfg.DNSDefaultMigrated {
		t.Error("migration marker should be set after migration")
	}

	// Повторная загрузка уже мигрировавшего конфига: значение не трогается.
	cfg2 := config.Load(path, "zapret")
	if !cfg2.DNSDefaultMigrated || !cfg2.AutoStartXboxDns {
		t.Error("migrated config should stay migrated on reload")
	}
}

// Новый конфиг: дефолты — автозапуск DNS включён, порог «От АКБ», степень низкая.
func TestFreshDefaultsPowerAndDNS(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")
	if !cfg.AutoStartXboxDns {
		t.Error("fresh default auto_start_xbox_dns = false, want true")
	}
	if cfg.PowerLevel != 1 {
		t.Errorf("fresh default power_level = %d, want 1 (low)", cfg.PowerLevel)
	}
	if cfg.PowerThreshold != -1 {
		t.Errorf("fresh default power_threshold = %d, want -1 (on battery)", cfg.PowerThreshold)
	}
}


// --- ServerConfig: единственный писатель ---

// Параллельные UpdateServerConfig не теряют записи: N горутин дописывают
// маркер в AccountEmail — итог содержит все N. Прямые записи мимо
// UpdateServerConfig запрещены конвенцией (см. SetConfig).
func TestUpdateServerConfigSingleWriter(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := cfg.UpdateServerConfig(func(s *config.ServerConfig) {
				s.AccountEmail = fmt.Sprintf("%s,%d", s.AccountEmail, i)
			}); err != nil {
				t.Errorf("UpdateServerConfig: %v", err)
			}
		}(i)
	}
	wg.Wait()
	email := cfg.GetServerConfig().AccountEmail
	for i := 0; i < n; i++ {
		if !strings.Contains(email, ","+strconv.Itoa(i)) {
			t.Errorf("AccountEmail lost update %d: %q", i, email)
		}
	}

	// Значение переживает сохранение на диск и перезагрузку конфига.
	cfg2 := config.Load(filepath.Join(dir, "c.json"), "zapret")
	if got := cfg2.GetServerConfig().AccountEmail; got != email {
		t.Errorf("after reload AccountEmail = %q, want %q", got, email)
	}
}

// Дефолтная пользовательская таблица DNS сеется один раз; после удаления
// записей пользователем заново не появляется (маркер dns_defaults_seeded).
// Свежий конфиг стартует с пустой моделью провайдеров: легаси-таблица
// не resurrect'ится, записи пользователя переживают перезагрузку.
func TestDnsDefaultsSeededOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.json")

	cfg := config.Load(path, "zapret")
	xd := cfg.GetXboxDnsConfig()
	if len(xd.Servers) != 0 || len(xd.Custom) != 0 || len(xd.Providers) != 0 {
		t.Fatalf("fresh config must start empty, got servers=%v custom=%+v providers=%v",
			xd.Servers, xd.Custom, xd.Providers)
	}
	if !cfg.DNSDefaultsSeeded {
		t.Fatal("seed marker must be set")
	}
	// Перезагрузка конфига ничего не теряет и не порождает легаси-таблицу.
	if got := config.Load(path, "zapret").GetXboxDnsConfig(); len(got.Servers) != 0 || len(got.Custom) != 0 {
		t.Fatalf("reload must keep the model empty, got servers=%v custom=%+v", got.Servers, got.Custom)
	}

	// Пользовательская запись добавляется включённой и остаётся включённой
	// после перезагрузки.
	xd.Custom = append(xd.Custom, config.CustomDns{ID: "c1", Name: "Quad9", Primary: "9.9.9.9", Enabled: true})
	if err := cfg.SetXboxDnsConfig(xd); err != nil {
		t.Fatalf("add custom entry: %v", err)
	}
	got := config.Load(path, "zapret").GetXboxDnsConfig()
	if len(got.Custom) != 1 || got.Custom[0].Primary != "9.9.9.9" || !got.Custom[0].Enabled {
		t.Fatalf("custom entry lost after reload: %+v", got.Custom)
	}

	// Пользователь удалил запись — при следующем старте не возвращается.
	if err := cfg.SetXboxDnsConfig(config.XboxDnsConfig{Custom: []config.CustomDns{}, Providers: got.Providers}); err != nil {
		t.Fatalf("clear custom: %v", err)
	}
	cfg2 := config.Load(path, "zapret")
	if got := cfg2.GetXboxDnsConfig(); len(got.Custom) != 0 {
		t.Fatalf("deleted entries must not come back, got %+v", got.Custom)
	}
}

// Пустая таблица + включённый пресет: IP пресета попадают в применение —
// pre-flight и Enable работают по ним (кейс «тумблер не включается»).
func TestEmptyTableWithPresetGivesPlainIPs(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")
	if err := cfg.SetXboxDnsConfig(config.XboxDnsConfig{
		Servers:     []config.DnsServer{},
		XboxEnabled: true,
	}); err != nil {
		t.Fatalf("SetXboxDnsConfig: %v", err)
	}
	ips := cfg.GetXboxDnsConfig().PlainIPs()
	if len(ips) < 2 {
		t.Fatalf("PlainIPs with preset only = %v, want preset addresses", ips)
	}
}

// Звук интерфейса: свежая установка — включён; апгрейд конфига старой версии
// (явное «выключено» по старому дефолту) — включается один раз; после
// миграции значение меняется только пользователем.
func TestSoundsDefaultMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.json")

	cfg := config.Load(path, "zapret")
	if !cfg.GetSounds().Enabled {
		t.Fatal("fresh install: sounds must default to enabled")
	}

	// Пользователь выключил — перезагрузка конфига состояние уважает.
	if err := cfg.SetSounds(config.SoundsConfig{Enabled: false, Volume: 60, Scheme: "soft"}); err != nil {
		t.Fatalf("disable sounds: %v", err)
	}
	if cfg2 := config.Load(path, "zapret"); cfg2.GetSounds().Enabled {
		t.Fatal("user-disabled sounds must survive reload (marker already set)")
	}

	// Конфиг старой версии: без маркера, звук off — включается принудительно один раз.
	old := `{"sounds":{"enabled":false,"volume":60,"scheme":"soft"},"theme":"dark"}`
	if err := os.WriteFile(path, []byte(old), 0644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	cfg3 := config.Load(path, "zapret")
	if !cfg3.GetSounds().Enabled {
		t.Fatal("legacy config: sounds must be force-enabled once")
	}

	// После миграции выключение пользователем сохраняется.
	if err := cfg3.SetSounds(config.SoundsConfig{Enabled: false, Volume: 30, Scheme: "quiet"}); err != nil {
		t.Fatalf("disable after migration: %v", err)
	}
	if cfg4 := config.Load(path, "zapret"); cfg4.GetSounds().Enabled {
		t.Fatal("sounds must stay off after user change post-migration")
	}
}

func TestValidPortSpec(t *testing.T) {
	valid := []string{"", "1024-65535", "1024-1934,1936-65535", "443,2053,8443", "1", "65535", " 1024 - 5000 "}
	invalid := []string{"abc", "1024-", "-5000", "0-100", "5000-1024", "1-65536", "443,,8443", "443,abc"}
	for _, s := range valid {
		if !config.ValidPortSpec(s) {
			t.Errorf("ValidPortSpec(%q) = false, want true", s)
		}
	}
	for _, s := range invalid {
		if config.ValidPortSpec(s) {
			t.Errorf("ValidPortSpec(%q) = true, want false", s)
		}
	}
}

func TestGetGameFilterPortsDefaults(t *testing.T) {
	cfg := config.Load(filepath.Join(t.TempDir(), "gf.json"), "zapret")
	tcp, udp := cfg.GetGameFilterPorts()
	if tcp != "1024-65535" || udp != "1024-65535" {
		t.Fatalf("defaults: got %q/%q", tcp, udp)
	}
	if err := cfg.SetGameFilterPorts("1024-1934,1936-65535", ""); err != nil {
		t.Fatalf("SetGameFilterPorts: %v", err)
	}
	if err := cfg.SetGameFilterPorts("bad", ""); err == nil {
		t.Fatal("SetGameFilterPorts must reject invalid spec")
	}
	tcp, _ = cfg.GetGameFilterPorts()
	if tcp != "1024-1934,1936-65535" {
		t.Fatalf("custom tcp: got %q", tcp)
	}
}
