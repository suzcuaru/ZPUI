package config_test

import (
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
