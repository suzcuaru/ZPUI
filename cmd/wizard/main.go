package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"zpui/internal/config"
	"zpui/internal/logger"
	"zpui/internal/wizard"
)

var version = "1.0.0"

func main() {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	configPath := filepath.Join(exeDir, "config.json")

	logFile := filepath.Join(exeDir, "logs", "wizard.log")
	os.MkdirAll(filepath.Dir(logFile), 0755)
	logMgr, _ := logger.New(filepath.Dir(logFile), 7)
	defer logMgr.Close()

	cfg := config.Load(configPath, exeDir)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	res, err := wizard.Run(ctx, wizard.Options{
		ExeDir: exeDir,
		Config: cfg,
		Log:    logMgr,
		OnProgress: func(p wizard.Progress) {
			fmt.Printf("[%s] %s\n", p.Step, p.Message)
		},
	})

	fmt.Println("\n=== Wizard complete ===")
	if err != nil {
		fmt.Printf("ERROR: %s\n", err.Error())
		os.Exit(1)
	}
	if res.BestStrategy != "" {
		fmt.Printf("Best strategy: %s\n", res.BestStrategy)
	}
}
