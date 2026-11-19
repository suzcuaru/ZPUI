package main

// ZPUI Setup — полный установщик на Wails (макет mockups/installer-ui).
// Мастер: приветствие → лицензия → режим (если уже установлен) → установка
// → финал. Логика — internal/pkginst (порт installer/ZPUI.nsi).

import (
	"embed"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"zpui/internal/pkginst"
)

var version = "0.0.0"

//go:embed all:assets
var assetsFS embed.FS

//go:embed payload.zip
var payloadFS embed.FS

//go:embed license.txt
var licenseText string

//go:embed changelog.txt
var changelogText string

var flagTarget = flag.String("target", "", "каталог установки (режим разработки, без реестра и UAC)")
var flagPayload = flag.String("payload", "", "путь к payload.zip вместо встроенного")

func main() {
	flag.Parse()
	if version == "" || version == "0.0.0" {
		if v := os.Getenv("ZPUI_VERSION"); v != "" {
			version = v
		}
	}

	pkginst.CheckWebView2()

	payload, err := openPayload(*flagPayload)
	if err != nil {
		pkginst.MessageBoxStop("Установка ZPUI "+version, "Пакет установки повреждён: "+err.Error())
		os.Exit(1)
	}

	dir := *flagTarget
	oldVer := ""
	sandbox := dir != ""
	if !sandbox {
		if !pkginst.IsElevated() {
			pkginst.Elevate()
			os.Exit(0)
		}
		dir, oldVer, _ = pkginst.FindInstall()
	}
	if dir == "" {
		// по битности ОС: 64-битная Windows → настоящий Program Files
		if runtime.GOARCH == "386" {
			if pf64 := os.Getenv("ProgramW6432"); pf64 != "" {
				dir = filepath.Join(pf64, "ZPUI")
			}
		}
		if dir == "" {
			dir = filepath.Join(os.Getenv("ProgramFiles"), "ZPUI")
		}
	}

	app := newApp(&payload, dir, oldVer, version, sandbox)

	assets, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		os.Exit(1)
	}

	err = wails.Run(&options.App{
		Title:            "Установка ZPUI " + version,
		Width:            680,
		Height:           520,
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
