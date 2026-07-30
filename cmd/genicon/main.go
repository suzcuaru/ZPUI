package main

import (
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
	fmt.Println("Place your own icon.ico in build/windows/")
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
