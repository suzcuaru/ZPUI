package app

import (
	"path/filepath"
	"testing"

	"zpui/internal/config"
)

func TestDisableDnsOrphansEmptyKnownDoesNotWipe(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")

	c := cfg.GetXboxDnsConfig()
	c.Providers = map[string]bool{"comss": true}
	c.Enabled = true
	if err := cfg.SetXboxDnsConfig(c); err != nil {
		t.Fatalf("set config: %v", err)
	}

	a := &App{cfg: cfg}
	if a.disableDnsOrphans(map[string]bool{}) {
		t.Fatal("empty known list must not be treated as orphan list")
	}
	got := cfg.GetXboxDnsConfig()
	if !got.Providers["comss"] || !got.Enabled {
		t.Fatalf("sources wiped by empty known list: %+v", got)
	}
}
