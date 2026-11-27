package extension

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// TestLocalDirResolution — каталог standalone-пакета якорится в
// %LOCALAPPDATA%\ZPUI\extension; LocalPath либо пуст, либо указывает туда.
func TestLocalDirResolution(t *testing.T) {
	want := filepath.Join(os.Getenv("LOCALAPPDATA"), "ZPUI", "extension")
	if got := LocalDir(); got != want {
		t.Fatalf("LocalDir = %q, want %q", got, want)
	}
	if p := LocalPath(); p != "" && !strings.HasPrefix(p, want+string(filepath.Separator)) {
		t.Fatalf("LocalPath = %q вне %q", p, want)
	}
	// Версия пуста ровно тогда, когда пакета нет.
	if LocalPath() == "" && LocalVersion() != "" {
		t.Fatalf("LocalVersion = %q при отсутствии пакета", LocalVersion())
	}
}

// TestWritePackageToRoundTrip — запись в произвольный каталог (якорь
// standalone-установщика) с атомарной заменой и без мусора.
func TestWritePackageToRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := WritePackageTo(dir, crxWithManifest(t, `{"version":"3.0.0"}`)); err != nil {
		t.Fatal(err)
	}
	if v, err := ReadVersion(filepath.Join(dir, File)); err != nil || v != "3.0.0" {
		t.Fatalf("want 3.0.0, got %q err=%v", v, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != File {
		t.Fatalf("должен остаться только %s, найдено: %d записей", File, len(entries))
	}
}

// TestInstallPathErrors — валидация аргументов без записи в реестр.
func TestInstallPathErrors(t *testing.T) {
	fails := InstallPath("", "1.0.0", []string{"chrome"})
	if msg := fails["chrome"]; msg == "" {
		t.Fatal("пустой путь к пакету обязан дать ошибку")
	}
	fails = InstallPath(filepath.Join(t.TempDir(), File), "1.0.0", []string{"nexus"})
	if msg := fails["nexus"]; msg != "неизвестный браузер" {
		t.Fatalf("неизвестный id: got %q", msg)
	}
}

// TestInstallPathRegistry — реальная запись HKCU (guard через Skip: тест не
// про права системы). Пишется редкий браузер, после — обязательная уборка.
func TestInstallPathRegistry(t *testing.T) {
	b, ok := byID("vivaldi")
	if !ok {
		t.Fatal("vivaldi пропал из списка браузеров")
	}
	if k, _, err := registry.CreateKey(registry.CURRENT_USER, regKey(b), registry.SET_VALUE|registry.QUERY_VALUE); err != nil {
		t.Skipf("registry unavailable: %v", err)
	} else {
		k.Close()
	}
	t.Cleanup(func() { _ = registry.DeleteKey(registry.CURRENT_USER, regKey(b)) })

	pkg := filepath.Join(t.TempDir(), File)
	if err := os.WriteFile(pkg, []byte("Cr24"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fails := InstallPath(pkg, "9.9.9", []string{"vivaldi"}); len(fails) != 0 {
		t.Fatalf("install fails: %v", fails)
	}
	if got := regVersion(b); got != "9.9.9" {
		t.Fatalf("registry version = %q, want 9.9.9", got)
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, regKey(b), registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	path, _, err := k.GetStringValue("path")
	k.Close()
	if err != nil || path != pkg {
		t.Fatalf("registry path = %q err=%v, want %q", path, err, pkg)
	}

	if fails := Uninstall([]string{"vivaldi"}); len(fails) != 0 {
		t.Fatalf("uninstall fails: %v", fails)
	}
	if got := regVersion(b); got != "" {
		t.Fatalf("после Uninstall версия = %q, want пусто", got)
	}
}
