package main

import (
	"bytes"
	"os"
	"testing"

	"zpui/internal/extension"
)

// TestEmbeddedCrx — встроенный extension.crx обязан быть настоящим CRX3 с
// читаемым manifest.json (build.bat перезаписывает файл перед сборкой;
// тест ловит подмену на заглушку).
func TestEmbeddedCrx(t *testing.T) {
	b, err := crxFS.ReadFile("extension.crx")
	if err != nil {
		t.Fatalf("read embedded extension.crx: %v", err)
	}
	if len(b) < 4 || string(b[:4]) != "Cr24" {
		t.Fatalf("embedded extension.crx is not CRX3 (magic %q)", b[:min(4, len(b))])
	}
	ver, err := extension.ReadVersionBytes(b)
	if err != nil || ver == "" {
		t.Fatalf("embedded package unreadable: ver=%q err=%v", ver, err)
	}
	// Рассинхрон встроенного файла и файла на диске — признак устаревшей
	// сборки пакета (как embed-тест cmd/setup).
	if disk, err := os.ReadFile("extension.crx"); err == nil && !bytes.Equal(disk, b) {
		t.Fatal("embedded extension.crx differs from on-disk file - rebuild cmd/extsetup")
	}
}
