package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"zpui/internal/tray"
)

func main() {
	root := findRoot()
	appicon := filepath.Join(root, "build", "appicon.png")
	iconIco := filepath.Join(root, "build", "windows", "icon.ico")

	// generate 512x512 PNG
	img := image.NewRGBA(image.Rect(0, 0, 512, 512))
	tray.DrawShieldIcon(img)

	f, err := os.Create(appicon)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create %s: %v\n", appicon, err)
		os.Exit(1)
	}
	if err := png.Encode(f, img); err != nil {
		fmt.Fprintf(os.Stderr, "encode PNG: %v\n", err)
		os.Exit(1)
	}
	f.Close()
	fmt.Println("Generated:", appicon)

	// generate multi-size ICO
	sizes := []int{16, 32, 64, 128, 256}
	var icoBuf bytes.Buffer
	icoBuf.Write([]byte{0, 0, 1, 0, byte(len(sizes)), 0})

	type imgEntry struct {
		data   []byte
		offset int
	}
	entries := make([]imgEntry, len(sizes))
	offset := 6 + len(sizes)*16

	for i, sz := range sizes {
		m := image.NewRGBA(image.Rect(0, 0, sz, sz))
		tray.DrawShieldIcon(m)

		var buf bytes.Buffer
		png.Encode(&buf, m)
		pngData := buf.Bytes()

		entries[i] = imgEntry{data: pngData, offset: offset}
		offset += len(pngData)
	}

	for i, sz := range sizes {
		e := entries[i]
		pngData := e.data

		var entry [16]byte
		if sz >= 256 {
			entry[0] = 0
			entry[1] = 0
		} else {
			entry[0] = byte(sz)
			entry[1] = byte(sz)
		}
		entry[2] = 0
		entry[3] = 0
		entry[4] = 1
		entry[5] = 32
		entry[6] = byte(len(pngData))
		entry[7] = byte(len(pngData) >> 8)
		entry[8] = byte(len(pngData) >> 16)
		entry[9] = byte(len(pngData) >> 24)
		entry[10] = byte(e.offset)
		entry[11] = byte(e.offset >> 8)
		entry[12] = byte(e.offset >> 16)
		entry[13] = byte(e.offset >> 24)
		icoBuf.Write(entry[:])
	}

	for _, e := range entries {
		icoBuf.Write(e.data)
	}

	if err := os.WriteFile(iconIco, icoBuf.Bytes(), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write ICO: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Generated:", iconIco)
}

func findRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}
