package pkginst

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupUpdateLeftovers(t *testing.T) {
	tmp := t.TempDir()
	exe := t.TempDir()
	write := func(dir, name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	leftovers := []string{
		"ZPUI-Setup-2.1.1.exe",
		"ZPUI-Update-2.1.0.exe",
		"ZPUI-Setup-2.1.1.exe.part",
		"ZPUI-CHANGELOG-2.1.1.txt",
		"zapret-update.zip",
	}
	for _, n := range leftovers {
		write(tmp, n)
	}
	keepTemp := []string{"user-notes.txt", "zpui.db", "settings.json"}
	for _, n := range keepTemp {
		write(tmp, n)
	}

	write(exe, "ZPUI-Update-1.9.0.exe")
	keepExe := []string{"ZPUI.exe", "ZPUI-Setup-2.1.1.exe", "config.json"}
	for _, n := range keepExe {
		write(exe, n)
	}

	cleanupLeftovers(tmp, exe)

	for _, n := range leftovers {
		if _, err := os.Stat(filepath.Join(tmp, n)); !os.IsNotExist(err) {
			t.Errorf("leftover %s must be removed from temp", n)
		}
	}
	for _, n := range keepTemp {
		if _, err := os.Stat(filepath.Join(tmp, n)); err != nil {
			t.Errorf("unrelated temp file %s must be kept: %v", n, err)
		}
	}
	if _, err := os.Stat(filepath.Join(exe, "ZPUI-Update-1.9.0.exe")); !os.IsNotExist(err) {
		t.Errorf("ZPUI-Update-*.exe must be removed from exe dir")
	}
	for _, n := range keepExe {
		if _, err := os.Stat(filepath.Join(exe, n)); err != nil {
			t.Errorf("exe dir file %s must be kept: %v", n, err)
		}
	}
}
