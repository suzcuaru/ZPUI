package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"zpui/internal/app"
	"zpui/internal/config"
	"zpui/internal/database"
	"zpui/internal/logger"
	"zpui/internal/modules"
	"zpui/internal/singleinstance"
	"zpui/internal/tray"
	"zpui/internal/updater"
)

//go:embed all:web/dist
var webFS embed.FS

//go:embed build/windows/icon.ico
var trayIcon []byte

var version = "0.0.0"

func main() {
	var skipChecks bool
	var verbose bool
	for _, arg := range os.Args[1:] {
		switch strings.ToLower(arg) {
		case "--skip-checks", "--skip-startup":
			skipChecks = true
		case "--verbose", "-v", "--debug":
			verbose = true
		}
	}

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
	if verbose || cfg.Verbose {
		logMgr.SetVerbose(true)
	}
	logMgr.Info("main", fmt.Sprintf("ZPUI v%s starting...", version))

	dbPath := filepath.Join(exeDir, "zpui.db")
	db, err := database.Open(dbPath)
	if err != nil {
		logMgr.Error("main", fmt.Sprintf("database: %v", err))
		log.Fatalf("database: %v", err)
	}
	defer db.Close()
	logMgr.Info("main", "Database initialized")

	modulesDir := modules.DefaultRootDir(exeDir)
	if err := modules.EnsureModulesDir(modulesDir); err != nil {
		logMgr.Error("main", fmt.Sprintf("modules dir: %v", err))
	}

	mgr := modules.NewManager(modulesDir, logMgr, cfg.IsModDisabled)

	upd := updater.New("suzcuaru", "ZPUI", version)

	if !skipChecks {
		discovered := modules.Discover(modulesDir)
		logMgr.Info("main", fmt.Sprintf("Discovered %d module(s)", len(discovered)))
		for _, dm := range discovered {
			status := "ok"
			if !dm.EntryOK {
				status = "no-entry-exe"
			}
			logMgr.Debug("main", fmt.Sprintf("  - %s v%s (%s)", dm.Manifest.ID, dm.Manifest.Version, status))
		}
	}

	a := app.New(cfg, logMgr, db, mgr, upd, version, exeDir, skipChecks)

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
		Title:    "ZPUI",
		Width:    960,
		Height:   640,
		MinWidth: 800,
		MinHeight: 600,
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
