package extension

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// crxWithManifest — минимальный CRX3-контейнер (подпись не проверяется:
// читается только manifest.json из архива).
func crxWithManifest(t *testing.T, manifest string) []byte {
	t.Helper()
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	w, err := zw.Create("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(manifest)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	header := []byte{0x12, 0x02, 0x08, 0x01} // произвольный protobuf-заголовок
	out := []byte("Cr24")
	out = binary.LittleEndian.AppendUint32(out, 3)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(header)))
	out = append(out, header...)
	return append(out, archive.Bytes()...)
}

func TestReadVersionBytes(t *testing.T) {
	v, err := ReadVersionBytes(crxWithManifest(t, `{"manifest_version":3,"name":"ZPUI","version":"1.2.3"}`))
	if err != nil || v != "1.2.3" {
		t.Fatalf("want 1.2.3, got %q err=%v", v, err)
	}
	for name, in := range map[string][]byte{
		"short":      []byte("Cr24"),
		"not crx":    []byte("PK\x03\x04somezipdata"),
		"bad header": append([]byte("Cr24"), 3, 0, 0, 0, 255, 255, 255, 255),
	} {
		if _, err := ReadVersionBytes(in); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestWritePackageRejectsNonCrx(t *testing.T) {
	if err := WritePackage(t.TempDir(), []byte("not a package")); err == nil {
		t.Fatal("want error for non-CRX payload")
	}
	if p := PkgPath(t.TempDir()); p != "" {
		t.Fatalf("no package expected, got %s", p)
	}
}

func TestWritePackageRoundTrip(t *testing.T) {
	dir := t.TempDir()
	pkg := crxWithManifest(t, `{"version":"2.0.0"}`)
	if err := WritePackage(dir, pkg); err != nil {
		t.Fatal(err)
	}
	if v := Version(dir); v != "2.0.0" {
		t.Fatalf("want 2.0.0, got %q", v)
	}
	// Повторная запись заменяет пакет, временный файл не остаётся.
	if err := WritePackage(dir, crxWithManifest(t, `{"version":"2.1.0"}`)); err != nil {
		t.Fatal(err)
	}
	if v := Version(dir); v != "2.1.0" {
		t.Fatalf("want 2.1.0, got %q", v)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "components", "extension"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != File {
		t.Fatalf("должен остаться только %s, найдено: %d записей", File, len(entries))
	}
}
