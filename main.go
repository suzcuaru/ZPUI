package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	zpuiapp "zpui/internal/app"
	"zpui/internal/config"
	"zpui/internal/database"
	"zpui/internal/executil"
	"zpui/internal/logger"
	"zpui/internal/monitor"
	"zpui/internal/proxy"
	"zpui/internal/singleinstance"
	"zpui/internal/tray"
	"zpui/internal/updater"
	"zpui/internal/xboxdns"
	"zpui/internal/zapret"
)

//go:embed all:web/dist
var webFS embed.FS

//go:embed build/windows/icon.ico
var trayIcon []byte

var version = "1.0.0"

func main() {
	if !ensureAdmin() {
		return
	}

	exePath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	exeDir := filepath.Dir(exePath)

	updater.SetCacheDir(exeDir)

	// Проверка единственного экземпляра
	if cleanup, err := singleinstance.Check(exePath); err != nil {
		fmt.Println("ZPUI:", err)
		return
	} else if cleanup != nil {
		defer cleanup()
	}

	configPath := filepath.Join(exeDir, "config.json")
	zapretDir := filepath.Join(exeDir, "zapret")

	cfg := config.Load(configPath, zapretDir)
	cfg.SetZapretPath(zapretDir)
	cfg.ModVersion = version

	logMgr, err := logger.New(cfg.LogsDir(), 7)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logMgr.Close()

	logMgr.Info("main", fmt.Sprintf("ZPUI v%s starting (Wails GUI)...", version))
	logMgr.Info("main", fmt.Sprintf("Zapret dir: %s", cfg.ZapretPath))
	logMgr.Info("main", fmt.Sprintf("Go %s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH))

	dbPath := filepath.Join(exeDir, "zpui.db")
	if err := database.Init(dbPath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()
	logMgr.Info("main", "Database initialized")

	zapretMgr := zapret.NewManager(cfg, logMgr)
	proxyServer := proxy.NewSOCKS5(cfg, logMgr)
	trafficMonitor := monitor.NewTrafficMonitor(logMgr)
	xboxDnsMgr := xboxdns.NewManager(logMgr)

	// Создаём Wails-приложение
	app := zpuiapp.NewApp(cfg, logMgr, zapretMgr, proxyServer, trafficMonitor, xboxDnsMgr, version, exeDir)

	// Окно запускается скрытым при запуске с ПК или если включён "start_minimized"
	startHidden := cfg.StartMinimized
	for _, arg := range os.Args[1:] {
		if arg == "--hidden" || arg == "--tray" {
			startHidden = true
		}
	}
	app.SetStartHidden(startHidden)

	// Создаём tray (контроллер = app, управляет окном через Wails runtime)
	trayApp := tray.New(cfg, logMgr, zapretMgr, proxyServer, app, version, trayIcon)
	go func() {
		runtime.LockOSThread()
		defer func() {
			if r := recover(); r != nil {
				logMgr.Error("tray", fmt.Sprintf("Tray panic: %v", r))
			}
		}()
		trayApp.Run()
	}()

	// Готовим embedded assets для Wails
	distFS, err := fs.Sub(webFS, "web/dist")
	if err != nil {
		log.Fatalf("Failed to create dist sub-FS: %v", err)
	}

	// Запускаем Wails (блокирует)
	logMgr.Info("main", "Starting Wails application...")
	err = wails.Run(&options.App{
		Title:     "ZPUI",
		Width:     960,
		Height:    640,
		MinWidth:  960,
		MinHeight: 640,
		MaxWidth:  960,
		MaxHeight: 640,
		AssetServer: &assetserver.Options{
			Assets: distFS,
		},
		OnStartup:        app.Startup,
		OnShutdown:       app.Shutdown,
		OnBeforeClose:    app.BeforeClose,
		StartHidden:      startHidden,
		Bind:             []interface{}{app},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})

	if err != nil {
		logMgr.Error("main", fmt.Sprintf("Wails error: %v", err))
		log.Fatal(err)
	}
}

func ensureAdmin() bool {
	cmd := executil.HiddenCmd("net", "session")
	if cmd.Run() == nil {
		return true
	}

	exe, _ := os.Executable()
	cmd = executil.HiddenCmd("powershell", "-NoProfile", "-Command",
		fmt.Sprintf("Start-Process '\"%s\"' -Verb RunAs", exe))
	cmd.Run()
	return false
}
