package pkginst

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPayloadContainsTgproxy(t *testing.T) {
	data, err := os.ReadFile(`../../cmd/setup/payload.zip`)
	if err != nil {
		t.Skip("payload.zip not found")
	}
	p := &Payload{Reader: sliceReader(data), Size: int64(len(data))}
	main, wintun, service, tgproxy, zapret, root, err := payloadTree(p, "x64/")
	if err != nil {
		t.Fatalf("payloadTree: %v", err)
	}
	t.Logf("x64: main=%d wintun=%d service=%d tgproxy=%d zapret=%d root=%d", len(main), len(wintun), len(service), len(tgproxy), len(zapret), len(root))
	if len(tgproxy) == 0 {
		t.Fatalf("payload has no components/tgproxy files for x64")
	}
	for _, f := range tgproxy {
		t.Logf("tgproxy entry: %s", f.Name)
	}
}

// Пакет плагина и его установщик входят в распаковываемое дерево компонентов:
// установщик сносил components/ целиком и не клал extension обратно.
func TestPayloadContainsExtension(t *testing.T) {
	data, err := os.ReadFile(`../../cmd/setup/payload.zip`)
	if err != nil {
		t.Skip("payload.zip not found")
	}
	p := &Payload{Reader: sliceReader(data), Size: int64(len(data))}
	_, _, _, files, _, _, err := payloadTree(p, "x64/")
	if err != nil {
		t.Fatalf("payloadTree: %v", err)
	}
	have := map[string]bool{}
	for _, f := range files {
		have[filepath.ToSlash(f.Name)] = true
	}
	for _, want := range []string{"x64/components/extension/zpui-extension.crx", "x64/components/extension/ZPUI-Extension-Setup.exe"} {
		if !have[want] {
			t.Errorf("payload file not in extracted tree: %s", want)
		}
	}
}
