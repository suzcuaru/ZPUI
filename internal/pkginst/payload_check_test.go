package pkginst

import (
	"os"
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
