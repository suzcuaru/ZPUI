package app

import "testing"

func TestZPUIServerManifestCache(t *testing.T) {
	a := &App{}
	a.setZPUIServerManifest("", "", "")
	if v, u, s := a.zpuiServerManifestCached(); v != "" || u != "" || s != "" {
		t.Fatalf("expected empty cache, got v=%q u=%q s=%q", v, u, s)
	}

	a.setZPUIServerManifest("2.1.1", "https://srv/api/v1/updates/zpui/download?id=1", "abc123")
	v, u, s := a.zpuiServerManifestCached()
	if v != "2.1.1" || u != "https://srv/api/v1/updates/zpui/download?id=1" || s != "abc123" {
		t.Fatalf("cache not stored: v=%q u=%q s=%q", v, u, s)
	}

	if !a.zpuiManifestCacheNewer("2.1.0") {
		t.Fatal("cached 2.1.1 must be newer than 2.1.0")
	}
	if a.zpuiManifestCacheNewer("2.1.1") || a.zpuiManifestCacheNewer("2.2.0") {
		t.Fatal("cache not newer must be reported as not newer")
	}

	a.setZPUIServerManifest("", "https://x", "y")
	if v, u, s := a.zpuiServerManifestCached(); v != "" || u != "" || s != "" {
		t.Fatalf("empty version must clear whole cache, got v=%q u=%q s=%q", v, u, s)
	}
	if a.zpuiManifestCacheNewer("0.0.1") {
		t.Fatal("empty cache must never be newer")
	}
}
