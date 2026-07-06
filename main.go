package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"zpui/internal/app"
	"zpui/internal/config"
	"zpui/internal/logger"
	"zpui/internal/modules"
	"zpui/internal/singleinstance"
	"zpui/internal/tray"
)

//go:embed all:web/dist
var webFS embed.FS

//go:embed build/windows/icon.ico
var trayIcon []byte

var version = "0.0.0"

func main() {
	exePath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	exeDir := filepath.Dir(exePath)

	if cleanup, err := singleinstance.Check(); err != nil {
		fmt.Println("ZPUI:", err)
		return
	} else if cleanup != nil {
		defer cleanup()
	}

	configPath := filepath.Join(exeDir, "config.json")
	cfg := config.Load(configPath)
	cfg.AppVersion = version

	logMgr, err := logger.New(cfg.LogsDir(), 7)
	if err != nil {
		log.Fatalf("logger: %v", err)
	}
	defer logMgr.Close()
	logMgr.Info("main", fmt.Sprintf("ZPUI v%s starting...", version))

	modulesDir := modules.DefaultRootDir(exeDir)
	if err := modules.EnsureModulesDir(modulesDir); err != nil {
		logMgr.Error("main", fmt.Sprintf("modules dir: %v", err))
	}

	mgr := modules.NewManager(modulesDir, logMgr, cfg.IsModDisabled)

	discovered := modules.Discover(modulesDir)
	logMgr.Info("main", fmt.Sprintf("Discovered %d module(s)", len(discovered)))
	for _, dm := range discovered {
		status := "ok"
		if !dm.EntryOK {
			status = "no-entry-exe"
		}
		logMgr.Info("main", fmt.Sprintf("  - %s v%s (%s)", dm.Manifest.ID, dm.Manifest.Version, status))
	}

	if cfg.AutoStartMods {
		mgr.AutoStartAll(discovered)
	}

	a := app.New(cfg, logMgr, mgr, version, exeDir)

	trayApp := tray.New(cfg, logMgr, a, version, trayIcon)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logMgr.Error("tray", fmt.Sprintf("panic: %v", r))
			}
		}()
		trayApp.Run()
	}()

	distFS, err := setupAssets()
	if err != nil {
		log.Fatalf("assets: %v", err)
	}

	logMgr.Info("main", "Starting Wails...")
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
		OnStartup:     a.Startup,
		OnShutdown:    a.Shutdown,
		OnBeforeClose: a.BeforeClose,
		Bind:          []interface{}{a},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})
	if err != nil {
		logMgr.Error("main", fmt.Sprintf("wails: %v", err))
		log.Fatal(err)
	}
}
