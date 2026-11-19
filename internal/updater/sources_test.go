package updater

import "testing"

func TestChainExplicit(t *testing.T) {
	for _, mode := range []string{"server", "yandex", "github", "ghproxy"} {
		got := Chain(mode, ProductZPUI)
		if len(got) != 1 || got[0] != mode {
			t.Fatalf("Chain(%q): want [%q], got %v", mode, mode, got)
		}
	}
}

func TestChainAuto(t *testing.T) {
	zpui := Chain("auto", ProductZPUI)
	if len(zpui) != 3 || zpui[0] != "server" || zpui[1] != "yandex" || zpui[2] != "github" {
		t.Fatalf("auto zpui: %v", zpui)
	}
	z2 := Chain("auto", ProductZapret2)
	if len(z2) != 1 || z2[0] != "server" {
		t.Fatalf("auto zapret2: %v", z2)
	}
	// Незнакомый режим = auto.
	weird := Chain("mystery", ProductZapret)
	if len(weird) != 3 || weird[0] != "server" {
		t.Fatalf("unknown mode: %v", weird)
	}
}

func TestGitHubURLs(t *testing.T) {
	raw := "https://github.com/o/r/releases/download/v1/f.zip"
	if got := GitHubURLs("github", raw); len(got) != 1 || got[0] != raw {
		t.Fatalf("github mode: %v", got)
	}
	got := GitHubURLs("ghproxy", raw)
	if len(got) != 3 {
		t.Fatalf("ghproxy mode: %v", got)
	}
	if got[0] != "https://gh-proxy.com/"+raw || got[1] != "https://ghfast.top/"+raw || got[2] != raw {
		t.Fatalf("ghproxy urls: %v", got)
	}
	if GitHubURLs("github", "") != nil {
		t.Fatalf("empty url must give nil")
	}
}
