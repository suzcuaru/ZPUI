package main

import (
	"archive/zip"
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestEmbeddedPayloadHasTgproxy(t *testing.T) {
	b, err := payloadFS.ReadFile("payload.zip")
	if err != nil {
		t.Fatalf("read embedded payload: %v", err)
	}
	count := func(data []byte) int {
		z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return -1
		}
		n := 0
		for _, f := range z.File {
			if strings.HasSuffix(f.Name, "ZPUI-TgProxy.exe") {
				n++
			}
		}
		return n
	}
	embedded := count(b)
	var disk []byte
	disk, err = os.ReadFile("payload.zip")
	if err != nil {
		disk = nil
	}
	onDisk := count(disk)
	t.Logf("embedded tgproxy entries=%d, on-disk payload.zip tgproxy entries=%d", embedded, onDisk)
	if onDisk > 0 && embedded == 0 {
		t.Fatalf("payload.zip has the tgproxy component but the binary embeds a stale one - rebuild cmd/setup")
	}
	if embedded == 0 && onDisk == 0 {
		t.Skip("tgproxy component not built - payload without it is expected")
	}
}
