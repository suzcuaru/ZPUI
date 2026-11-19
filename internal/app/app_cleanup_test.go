package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanOrphanListFilesKeepsGoogleList(t *testing.T) {
	a := newNXTestApp(t)
	lists := a.cfg.ListsDir()
	if err := os.MkdirAll(lists, 0o755); err != nil {
		t.Fatal(err)
	}
	google := filepath.Join(lists, "list-google.txt")
	backup := filepath.Join(lists, "ipset-all.txt.backup")
	if err := os.WriteFile(google, []byte("youtube.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("203.0.113.113/32\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	a.cleanOrphanListFiles(filepath.Dir(a.cfg.GetZapretPath()))

	if _, err := os.Stat(google); err != nil {
		t.Fatalf("list-google.txt must survive cleanup: %v", err)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatalf("ipset-all.txt.backup must be removed, stat err = %v", err)
	}
}

func TestEnsureGoogleListCreatesDefaultAndPreservesExisting(t *testing.T) {
	a := newNXTestApp(t)
	path := filepath.Join(a.cfg.ListsDir(), "list-google.txt")

	a.ensureGoogleList()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read created list: %v", err)
	}
	if !strings.Contains(string(data), "youtube.com") {
		t.Fatalf("default list has no youtube.com: %q", string(data))
	}

	const custom = "example.test\n"
	if err := os.WriteFile(path, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	a.ensureGoogleList()
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != custom {
		t.Fatalf("existing list overwritten: %q", string(data))
	}
}

// Регресс 31.10: чистильщик стирал components/tgproxy как «неизвестный»
// сразу после установки — «tgproxy component not installed».
func TestCleanComponentsKeepsTgproxy(t *testing.T) {
	a := newNXTestApp(t)
	comp := filepath.Join(a.exeDir, "components")
	tgp := filepath.Join(comp, "tgproxy")
	if err := os.MkdirAll(tgp, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tgp, "ZPUI-TgProxy.exe"), []byte("MZ"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tgp, "version.txt"), []byte("1.11.3"), 0o644); err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(comp, "someoldmod")
	if err := os.MkdirAll(unknown, 0o755); err != nil {
		t.Fatal(err)
	}

	a.cleanComponents(a.exeDir)

	if _, err := os.Stat(filepath.Join(tgp, "ZPUI-TgProxy.exe")); err != nil {
		t.Fatalf("tgproxy component must survive cleanup: %v", err)
	}
	if _, err := os.Stat(unknown); !os.IsNotExist(err) {
		t.Fatalf("unknown component must be removed, stat err = %v", err)
	}
}
