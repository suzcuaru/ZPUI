package pkginst

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestRelPathRejectsEscape(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	names := []string{"x64/normal.exe", "x64/../../evil.exe", "..\\evil.exe", "x86/app.dll", "README.md"}
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}

	want := map[int]string{0: "normal.exe", 1: "", 2: "", 3: "", 4: ""}
	for i, f := range r.File {
		if got := relPath(f, "x64/"); got != want[i] {
			t.Errorf("relPath(%q) = %q, want %q", f.Name, got, want[i])
		}
	}
}
