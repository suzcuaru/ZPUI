package main

// ZPUI Updater — обновлятор на Wails (Go + веб-макет из mockups/installer-ui).
// Заменяет NSIS-обновлятор: та же логика в internal/pkginst.

import (
	"embed"
	"flag"
	"io/fs"
	"os"

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

var (
	flagTarget  = flag.String("target", "", "каталог установки (режим разработки, без реестра и UAC)")
	flagPayload = flag.String("payload", "", "путь к payload.zip вместо встроенного")
	flagOldVer  = flag.String("oldversion", "", "старая версия для плашки (режим разработки)")
)

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
		pkginst.MessageBoxStop("Обновление ZPUI "+version, "Пакет обновления повреждён: "+err.Error())
		os.Exit(1)
	}

	dir := *flagTarget
	oldVer := *flagOldVer
	sandbox := dir != ""
	if !sandbox {
		if !pkginst.IsElevated() {
			pkginst.Elevate()
			os.Exit(0)
		}
		dir, oldVer, err = pkginst.FindInstall()
		if err != nil {
			pkginst.MessageBoxStop("Обновление ZPUI "+version,
				"ZPUI не найден на этом компьютере.\r\nСначала запустите полный установщик ZPUI-Setup.")
			os.Exit(1)
		}
		if oldVer == "" {
			oldVer = "?"
		}
	}

	app := newApp(&payload, dir, oldVer, version, sandbox)

	assets, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		os.Exit(1)
	}

	err = wails.Run(&options.App{
		Title:            "Обновление ZPUI " + version,
		Width:            680,
		Height:           480,
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
