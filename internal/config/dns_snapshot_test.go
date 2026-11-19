package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zpui/internal/config"
)

func TestDnsProvidersBeforeCustomRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.json")
	cfg := config.Load(path, "zapret")

	if snap := cfg.GetDnsProvidersBeforeCustom(); len(snap) != 0 {
		t.Fatalf("fresh config snapshot = %v, want none", snap)
	}

	if err := cfg.SetDnsProvidersBeforeCustom(map[string]bool{"google": true, "adguard": true}); err != nil {
		t.Fatal(err)
	}
	snap := cfg.GetDnsProvidersBeforeCustom()
	if !snap["google"] || !snap["adguard"] || len(snap) != 2 {
		t.Fatalf("snapshot = %v, want google+adguard", snap)
	}

	cfg2 := config.Load(path, "zapret")
	if got := cfg2.GetDnsProvidersBeforeCustom(); !got["google"] || !got["adguard"] || len(got) != 2 {
		t.Fatalf("snapshot lost after reload: %v", got)
	}

	if err := cfg2.SetDnsProvidersBeforeCustom(nil); err != nil {
		t.Fatal(err)
	}
	if got := cfg2.GetDnsProvidersBeforeCustom(); len(got) != 0 {
		t.Fatalf("snapshot after reset = %v, want none", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "dns_providers_before_custom") {
		t.Errorf("empty snapshot must be omitted from json")
	}
}

func TestDnsProvidersBeforeCustomDeepCopy(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load(filepath.Join(dir, "c.json"), "zapret")
	if err := cfg.SetDnsProvidersBeforeCustom(map[string]bool{"google": true}); err != nil {
		t.Fatal(err)
	}

	snap := cfg.GetDnsProvidersBeforeCustom()
	snap["google"] = false
	snap["hacked"] = true
	if got := cfg.GetDnsProvidersBeforeCustom(); !got["google"] || got["hacked"] {
		t.Fatalf("getter leaks internal map: %v", got)
	}

	xd := cfg.GetXboxDnsConfig()
	if len(xd.ProvidersBeforeCustom) == 0 || !xd.ProvidersBeforeCustom["google"] {
		t.Fatalf("GetXboxDnsConfig must carry the snapshot, got %v", xd.ProvidersBeforeCustom)
	}
	xd.ProvidersBeforeCustom["google"] = false
	xd.ProvidersBeforeCustom["hacked"] = true
	if got := cfg.GetDnsProvidersBeforeCustom(); !got["google"] || got["hacked"] {
		t.Fatalf("GetXboxDnsConfig leaks snapshot map: %v", got)
	}

	mutated := map[string]bool{"cloudflare": true}
	if err := cfg.SetDnsProvidersBeforeCustom(mutated); err != nil {
		t.Fatal(err)
	}
	mutated["cloudflare"] = false
	mutated["extra"] = true
	if got := cfg.GetDnsProvidersBeforeCustom(); !got["cloudflare"] || got["extra"] {
		t.Fatalf("setter aliased caller map: %v", got)
	}
}

func TestDnsProvidersSnapshotSurvivesConfigRoundtrip(t *testing.T) {
	// Сценарий статики: снимок обязан переживать пару Get → правка → Set,
	// которой тумблеры персистят очистку Providers.
	dir := t.TempDir()
	path := filepath.Join(dir, "c.json")
	cfg := config.Load(path, "zapret")

	if err := cfg.SetDnsProvidersBeforeCustom(map[string]bool{"google": true}); err != nil {
		t.Fatal(err)
	}

	xd := cfg.GetXboxDnsConfig()
	xd.Providers = map[string]bool{}
	if err := cfg.SetXboxDnsConfig(xd); err != nil {
		t.Fatal(err)
	}
	if got := cfg.GetDnsProvidersBeforeCustom(); !got["google"] {
		t.Fatalf("snapshot wiped by providers-clear roundtrip: %v", got)
	}
	if xd2 := cfg.GetXboxDnsConfig(); xd2.Providers["google"] {
		t.Fatalf("providers must stay cleared during static, got %v", xd2.Providers)
	}

	xd = cfg.GetXboxDnsConfig()
	xd.Providers = cfg.GetDnsProvidersBeforeCustom()
	if err := cfg.SetDnsProvidersBeforeCustom(nil); err != nil {
		t.Fatal(err)
	}
	xd.ProvidersBeforeCustom = nil
	if err := cfg.SetXboxDnsConfig(xd); err != nil {
		t.Fatal(err)
	}

	reloaded := config.Load(path, "zapret").GetXboxDnsConfig()
	if !reloaded.Providers["google"] || len(reloaded.Providers) != 1 {
		t.Fatalf("restore failed: providers=%v", reloaded.Providers)
	}
	if len(reloaded.ProvidersBeforeCustom) != 0 {
		t.Fatalf("snapshot must be cleared after restore, got %v", reloaded.ProvidersBeforeCustom)
	}
	if data, err := os.ReadFile(path); err == nil && strings.Contains(string(data), "dns_providers_before_custom") {
		t.Errorf("cleared snapshot must be omitted from json")
	}
}
