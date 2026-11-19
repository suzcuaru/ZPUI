package main

// ZPUI Uninstall — деинсталлятор на Wails. Кладётся установщиком в каталог
// программы (uninstall.exe) и прописывается в «Установка и удаление программ».

import (
	"embed"
	"flag"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"zpui/internal/pkginst"
)

var version = "0.0.0"

//go:embed all:assets
var assetsFS embed.FS

var flagTarget = flag.String("target", "", "каталог установки (режим разработки, без реестра и UAC)")

func main() {
	flag.Parse()
	if version == "" || version == "0.0.0" {
		if v := os.Getenv("ZPUI_VERSION"); v != "" {
			version = v
		}
	}

	pkginst.CheckWebView2()

	dir := *flagTarget
	ver := version
	if dir == "" {
		if !pkginst.IsElevated() {
			pkginst.Elevate()
			os.Exit(0)
		}
		d, v, err := pkginst.FindInstall()
		if err != nil {
			d = exeDir()
			v = version
		}
		dir, ver = d, v
	}

	app := newApp(dir, ver)

	assets, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		os.Exit(1)
	}

	err = wails.Run(&options.App{
		Title:            "Удаление ZPUI",
		Width:            680,
		Height:           360,
		DisableResize:    true,
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 0x14, G: 0x14, B: 0x18, A: 0xff},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.Startup,
		Bind:      []interface{}{app},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})
	if err != nil {
		os.Exit(1)
	}
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}
