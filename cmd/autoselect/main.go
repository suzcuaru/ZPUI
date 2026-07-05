package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"zpui/internal/autoselect"
	"zpui/internal/config"
	"zpui/internal/logger"
	"zpui/internal/zapret"
)

var version = "1.0.0"

func main() {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	configPath := filepath.Join(exeDir, "config.json")

	logFile := filepath.Join(exeDir, "logs", "autoselect.log")
	os.MkdirAll(filepath.Dir(logFile), 0755)
	logMgr, _ := logger.New(filepath.Dir(logFile), 7)
	defer logMgr.Close()

	log := func(msg string) {
		fmt.Println(msg)
		logMgr.Info("autoselect", msg)
	}

	log("Auto-selector started")

	cfg := config.Load(configPath, exeDir)
	if cfg.GetZapretPath() == "" {
		log("ERROR: Zapret path not configured. Run wizard first.")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	res := autoselect.Run(ctx, cfg, logMgr, func(r zapret.AutoTestResult) {
		if r.Type == "result" && r.Strategy != "" {
			fmt.Printf("\r[%3d%%] %-40s %d/%d  %dms   ",
				r.Current, r.Strategy, r.ResourcesOK, r.ResourcesN, r.ResponseMs)
		} else if r.Message != "" {
			fmt.Printf("\n%s", r.Message)
		}
	})

	fmt.Println()
	if res.Error != "" {
		log("Finished with error: " + res.Error)
		os.Exit(1)
	}
	if res.Applied {
		log(fmt.Sprintf("Applied strategy: %s", res.Strategy))
	} else {
		log("No strategy applied")
		os.Exit(1)
	}
}
