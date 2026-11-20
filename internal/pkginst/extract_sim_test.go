package pkginst

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractTreeProducesTgproxy(t *testing.T) {
	data, err := os.ReadFile(`../../cmd/setup/payload.zip`)
	if err != nil {
		t.Skip("payload.zip not found")
	}
	p := &Payload{Reader: sliceReader(data), Size: int64(len(data))}
	dir := t.TempDir()
	if err := extractTree(p, dir, nil, false, true, ""); err != nil {
		t.Fatalf("extractTree: %v", err)
	}
	tgp := filepath.Join(dir, "components", "tgproxy")
	entries, err := os.ReadDir(tgp)
	if err != nil {
		t.Fatalf("no components/tgproxy after extractTree: %v", err)
	}
	for _, e := range entries {
		t.Logf("tgproxy/%s (%d bytes)", e.Name(), eSize(filepath.Join(tgp, e.Name())))
	}
	if _, err := os.Stat(filepath.Join(tgp, "ZPUI-TgProxy.exe")); err != nil {
		t.Fatalf("ZPUI-TgProxy.exe not extracted")
	}

	// Регрессия (фикс 2.1.2): записи каталогов payload («x64/skills/», size 0)
	// не должны разворачиваться в нулевой файл «skills» — иначе дочерние
	// записи падают в MkdirAll («предок — файл»).
	if fi, err := os.Stat(filepath.Join(dir, "skills")); err != nil || !fi.IsDir() {
		t.Fatalf("skills must exist as a directory: err=%v", err)
	}
	skillRel := hostArchPrefix() + "skills/zpui-mcp/SKILL.md"
	skillPath := filepath.Join(dir, "skills", "zpui-mcp", "SKILL.md")
	st, err := os.Stat(skillPath)
	if err != nil {
		t.Fatalf("skills/zpui-mcp/SKILL.md not extracted: %v", err)
	}
	var wantSkill int64 = -1
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if filepath.ToSlash(filepath.Clean(f.Name)) == skillRel {
			wantSkill = f.FileInfo().Size()
			break
		}
	}
	if wantSkill < 0 {
		t.Fatalf("payload has no entry %s", skillRel)
	}
	if st.Size() != wantSkill {
		t.Fatalf("SKILL.md size mismatch: got %d, payload entry %s = %d", st.Size(), skillRel, wantSkill)
	}
	t.Logf("skills/zpui-mcp/SKILL.md (%d bytes, matches payload)", st.Size())
}

func eSize(p string) int64 {
	st, err := os.Stat(p)
	if err != nil {
		return -1
	}
	return st.Size()
}
