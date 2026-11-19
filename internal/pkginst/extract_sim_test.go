package pkginst

import (
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
}

func eSize(p string) int64 {
	st, err := os.Stat(p)
	if err != nil {
		return -1
	}
	return st.Size()
}
