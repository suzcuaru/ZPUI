package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"zpui/internal/config"
	"zpui/internal/logger"
	"zpui/internal/zapret"
)

var version = "1.0.0"

func main() {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	configPath := filepath.Join(exeDir, "config.json")

	logFile := filepath.Join(exeDir, "logs", "zapretupdate.log")
	os.MkdirAll(filepath.Dir(logFile), 0755)
	logMgr, _ := logger.New(filepath.Dir(logFile), 7)
	defer logMgr.Close()

	log := func(msg string) {
		fmt.Println(msg)
		logMgr.Info("zapretupdate", msg)
	}
	logErr := func(msg string) {
		fmt.Fprintln(os.Stderr, msg)
		logMgr.Error("zapretupdate", msg)
	}

	log("Zapret updater started")

	cfg := config.Load(configPath, exeDir)
	zapretDir := cfg.GetZapretPath()
	if zapretDir == "" {
		logErr("Zapret path not configured")
		os.Exit(1)
	}

	listsDir := cfg.ListsDir()

	log("Backing up user lists...")
	backupDir := filepath.Join(exeDir, ".backup", "lists_"+time.Now().Format("20060102_150405"))
	os.MkdirAll(backupDir, 0755)

	backupCount := backupUserLists(listsDir, backupDir)
	if backupCount > 0 {
		log(fmt.Sprintf("Backed up %d user list files", backupCount))
	} else {
		log("No user list files found")
	}

	log("Checking for updates...")
	zapretMgr := zapret.NewManager(cfg, logMgr)

	info, err := zapretMgr.CheckForUpdates()
	if err != nil {
		logErr("Update check failed: " + err.Error())
		os.Exit(1)
	}

	if !info.UpdateNeeded {
		log(fmt.Sprintf("Zapret is up to date (v%s)", info.CurrentVersion))
		return
	}

	log(fmt.Sprintf("Update available: v%s -> v%s", info.CurrentVersion, info.LatestVersion))

	log("Stopping zapret if running...")
	zapretMgr.Stop()
	time.Sleep(2 * time.Second)

	progress := make(chan zapret.UpdateProgress, 20)
	go func() {
		defer close(progress)
		if err := zapretMgr.PerformUpdate(progress); err != nil {
			logErr("Update failed: " + err.Error())
			os.Exit(1)
		}
	}()

	for p := range progress {
		if p.Error != "" {
			logErr("Error: " + p.Error)
		} else {
			log(fmt.Sprintf("[%3d%%] %s", p.Percent, p.Step))
		}
	}

	log("Restoring user lists...")
	restoreCount := restoreUserLists(backupDir, listsDir)
	if restoreCount > 0 {
		log(fmt.Sprintf("Restored %d user list files", restoreCount))
	}

	log("Zapret update complete!")

	if cfg.LastZapretState {
		log("Restarting zapret...")
		zapretMgr.Start()
	}
}

func backupUserLists(listsDir, backupDir string) int {
	entries, err := os.ReadDir(listsDir)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if isUserList(name) {
			src := filepath.Join(listsDir, name)
			dst := filepath.Join(backupDir, name)
			if data, err := os.ReadFile(src); err == nil {
				os.WriteFile(dst, data, 0644)
				count++
			}
		}
	}
	return count
}

func restoreUserLists(backupDir, listsDir string) int {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return 0
	}
	count := 0
	os.MkdirAll(listsDir, 0755)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		src := filepath.Join(backupDir, entry.Name())
		dst := filepath.Join(listsDir, entry.Name())
		if data, err := os.ReadFile(src); err == nil {
			os.WriteFile(dst, data, 0644)
			count++
		}
	}
	return count
}

func isUserList(name string) bool {
	for _, prefix := range []string{"list-", "user-", "custom-"} {
		if len(name) > len(prefix) && name[:len(prefix)] == prefix {
			return true
		}
	}
	return name == "list-general-user.txt"
}
