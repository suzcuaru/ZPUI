package main

// ZPUI-Extension-Setup — standalone-установщик браузерного расширения ZPUI.
// Каркас — как у cmd/setup (Wails, рамка-тайтлбар, дизайн-система ZPUI):
// пишет встроенный CRX3 в %LOCALAPPDATA%\ZPUI\extension и регистрирует его
// в найденных Chromium-браузерах (HKCU, без UAC и повышения). Приложение
// обновляет расширение, запуская этот exe тихо (-silent -only …); вручную —
// окно со списком браузеров и кнопкой «Установить».

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"zpui/internal/extension"
	"zpui/internal/pkginst"
)

var version = "0.0.0"

//go:embed all:assets
var assetsFS embed.FS

//go:embed extension.crx
var crxFS embed.FS

var (
	flagSilent  = flag.Bool("silent", false, "тихая установка во все найденные браузеры (или -only), без окна")
	flagOnly    = flag.String("only", "", "подмножество браузеров через запятую: chrome,edge,yandex,opera,brave,vivaldi")
	flagTarget  = flag.String("target", "", "каталог для пакета (режим разработки, без записей в реестр)")
	flagVersion = flag.Bool("version", false, "показать версию и выйти")
)

func main() {
	flag.Parse()
	// -version и -silent обслуживаются до wails.Run: GUI не нужен.
	if *flagVersion {
		fmt.Println("ZPUI-Extension-Setup", version)
		return
	}
	if version == "" || version == "0.0.0" {
		if v := os.Getenv("ZPUI_VERSION"); v != "" {
			version = v
		}
	}

	crx, err := crxFS.ReadFile("extension.crx")
	extVer, verr := extension.ReadVersionBytes(crx)
	if err != nil || verr != nil {
		pkginst.MessageBoxStop("Расширение ZPUI "+version, "Пакет расширения повреждён: "+firstErr(err, verr))
		os.Exit(1)
	}

	dir := extension.LocalDir()
	sandbox := *flagTarget != ""
	if sandbox {
		dir = *flagTarget
	}

	if err := extension.WritePackageTo(dir, crx); err != nil {
		pkginst.MessageBoxStop("Расширение ZPUI "+version, "Не удалось сохранить пакет: "+err.Error())
		os.Exit(1)
	}

	if *flagSilent {
		os.Exit(runSilent(dir, extVer, sandbox))
	}

	pkginst.CheckWebView2()

	app := newApp(version, extVer, dir, sandbox)

	assets, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		os.Exit(1)
	}

	err = wails.Run(&options.App{
		Title:            "Расширение ZPUI " + version,
		Width:            620,
		Height:           500,
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

func firstErr(errs ...error) string {
	for _, e := range errs {
		if e != nil {
			return e.Error()
		}
	}
	return ""
}

// runSilent — тихий режим: пакет уже сохранён, остаются записи в реестре.
// Все найденные браузеры либо подмножество -only. Код 0 — успех, 1 — ошибка
// (плюс системный MessageBox, процесс без окна).
func runSilent(dir, extVer string, sandbox bool) int {
	known := map[string]bool{}
	ids := []string{}
	for _, b := range extension.List("") {
		known[b.ID] = true
		if b.Found {
			ids = append(ids, b.ID)
		}
	}
	if only := strings.TrimSpace(*flagOnly); only != "" {
		want := map[string]bool{}
		unknown := []string{}
		for _, p := range strings.Split(only, ",") {
			p = strings.ToLower(strings.TrimSpace(p))
			if p == "" {
				continue
			}
			if !known[p] {
				unknown = append(unknown, p)
				continue
			}
			want[p] = true
		}
		if len(unknown) > 0 {
			pkginst.MessageBoxStop("Расширение ZPUI "+version, "Неизвестные браузеры в -only: "+strings.Join(unknown, ", "))
			return 1
		}
		picked := make([]string, 0, len(ids))
		for _, id := range ids {
			if want[id] {
				picked = append(picked, id)
			}
		}
		ids = picked
	}
	if sandbox {
		return 0
	}
	if fails := extension.InstallPath(filepath.Join(dir, extension.File), extVer, ids); len(fails) > 0 {
		msgs := make([]string, 0, len(fails))
		for id, msg := range fails {
			msgs = append(msgs, id+": "+msg)
		}
		pkginst.MessageBoxStop("Расширение ZPUI "+version, "Не удалось зарегистрировать расширение:\r\n"+strings.Join(msgs, "\r\n"))
		return 1
	}
	return 0
}
