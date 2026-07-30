package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"zpui/internal/config"
	"zpui/internal/executil"
	"zpui/internal/logger"
	"zpui/internal/zapret"
)

//go:embed all:frontend/dist
var frontFS embed.FS

//go:embed build/windows/icon.ico
var appIcon []byte

var version = "3.1.0"

func main() {
	executil.SetProcessAppID("ZPUI")

	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)

	mode := "manual"
	isCheckMode := false
	for _, arg := range os.Args[1:] {
		if arg == "--from-app" {
			mode = "app"
		}
		if arg == "--check" {
			isCheckMode = true
		}
	}

	logFile := filepath.Join(exeDir, "logs", "selfupdate.log")
	os.MkdirAll(filepath.Dir(logFile), 0755)
	logMgr, _ := logger.New(filepath.Dir(logFile), exeDir)
	defer logMgr.Close()
	// В --check режиме stdout должен содержать ТОЛЬКО JSON состояния.
	// Лог-строки в консоль отключаем, иначеMain App не сможет их распарсить
	// (получит «invalid character '-' after array element» из-за дат в логах).
	if isCheckMode {
		logMgr.SetConsoleOutput(false)
	}
	logMgr.Info("selfupdate", fmt.Sprintf("selfupdate v%s starting (mode=%s)", version, mode))

	configPath := filepath.Join(exeDir, "config.json")
	zapretDir := filepath.Join(exeDir, "zapret")
	cfg := config.Load(configPath, zapretDir)
	cfg.SetZapretPath(zapretDir)

	zapretMgr := zapret.NewManager(cfg, logMgr)

	app := NewApp(cfg, logMgr, zapretMgr, version, exeDir, mode)

	if isCheckMode {
		state := app.GetState()
		json.NewEncoder(os.Stdout).Encode(state)
		return
	}

	distFS, err := fs.Sub(frontFS, "frontend/dist")
	if err != nil {
		log.Fatalf("Failed to create dist sub-FS: %v", err)
	}

	title := "ZPUI — Центр обновлений"

	if err := wails.Run(&options.App{
		Title:     title,
		Width:     480,
		Height:    460,
		MinWidth:  480,
		MinHeight: 460,
		MaxWidth:  480,
		MaxHeight: 460,
		AssetServer: &assetserver.Options{
			Assets: distFS,
		},
		OnStartup: func(ctx context.Context) {
			app.Startup(ctx)
			go func() {
				time.Sleep(300 * time.Millisecond)
				disableMaximizeButton(title)
			}()
		},
		Bind: []interface{}{app},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	}); err != nil {
		log.Fatal(err)
	}
}
