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
	// Утёкший снимок BackupUserFiles — непустой каталог, удаляется целиком.
	snapshot := filepath.Join(tmp, "zpui-user-1234567890")
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		t.Fatal(err)
	}
	write(snapshot, "config.json")

	keepTemp := []string{"user-notes.txt", "zpui.db", "settings.json", "otherapp.tmp"}
	for _, n := range keepTemp {
		write(tmp, n)
	}
	foreignDir := filepath.Join(tmp, "some-other-app")
	if err := os.MkdirAll(foreignDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(foreignDir, "data.bin")

	write(exe, "ZPUI-Update-1.9.0.exe")
	write(exe, "selfupdate.exe")
	write(exe, "security.exe")
	write(exe, "wizard.exe")
	keepExe := []string{"ZPUI.exe", "ZPUI-Setup-2.1.1.exe", "ZPUI-Service.exe", "uninstall.exe", "config.json", "zapret-backup.zip"}
	for _, n := range keepExe {
		write(exe, n)
	}

	freed, removed, skipped := cleanupLeftovers(tmp, exe)

	// 5 файлов temp + каталог-снимок + 4 файла exeDir.
	if removed != 10 {
		t.Fatalf("removed = %d, want 10", removed)
	}
	if skipped != 0 {
		t.Fatalf("skipped = %d, want 0", skipped)
	}
	// freed — по обычным файлам (по 1 байту): 5 temp + 4 exeDir;
	// содержимое каталога-снимка в размер не входит.
	if freed != 9 {
		t.Fatalf("freed = %d, want 9 (files only, dir contents not counted)", freed)
	}

	for _, n := range leftovers {
		if _, err := os.Stat(filepath.Join(tmp, n)); !os.IsNotExist(err) {
			t.Errorf("leftover %s must be removed from temp", n)
		}
	}
	if _, err := os.Stat(snapshot); !os.IsNotExist(err) {
		t.Errorf("zpui-user-* snapshot dir must be removed")
	}
	for _, n := range keepTemp {
		if _, err := os.Stat(filepath.Join(tmp, n)); err != nil {
			t.Errorf("unrelated temp file %s must be kept: %v", n, err)
		}
	}
	if _, err := os.Stat(foreignDir); err != nil {
		t.Errorf("foreign dir must be kept: %v", err)
	}
	for _, n := range []string{"ZPUI-Update-1.9.0.exe", "selfupdate.exe", "security.exe", "wizard.exe"} {
		if _, err := os.Stat(filepath.Join(exe, n)); !os.IsNotExist(err) {
			t.Errorf("legacy %s must be removed from exe dir", n)
		}
	}
	for _, n := range keepExe {
		if _, err := os.Stat(filepath.Join(exe, n)); err != nil {
			t.Errorf("exe dir file %s must be kept: %v", n, err)
		}
	}
}
