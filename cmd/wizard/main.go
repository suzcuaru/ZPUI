package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"zpui/internal/config"
	"zpui/internal/executil"
	"zpui/internal/logger"
	"zpui/internal/zapret"
)

var version = "1.0.0"

const repoURL = "https://github.com/Flowseal/zapret-discord-youtube"

func main() {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	configPath := filepath.Join(exeDir, "config.json")

	logFile := filepath.Join(exeDir, "logs", "wizard.log")
	os.MkdirAll(filepath.Dir(logFile), 0755)
	logMgr, _ := logger.New(filepath.Dir(logFile), 7)
	defer logMgr.Close()

	log := func(step string, msg string) {
		fmt.Printf("[%s] %s\n", step, msg)
		logMgr.Info("wizard", fmt.Sprintf("[%s] %s", step, msg))
	}

	log("init", "Wizard started")

	cfg := config.Load(configPath, exeDir)

	// Step 1: Check git
	log("git", "Checking git installation...")
	if _, err := exec.LookPath("git"); err != nil {
		log("git", "Git not found, installing...")
		if err := installGit(); err != nil {
			log("git", "Failed to install git: "+err.Error())
		}
	} else {
		log("git", "Git found")
	}

	// Step 2: Clone zapret
	zapretDir := filepath.Join(exeDir, "zapret")
	winws := filepath.Join(zapretDir, "bin", "winws.exe")
	if _, err := os.Stat(winws); err != nil {
		log("clone", fmt.Sprintf("Cloning zapret to %s...", zapretDir))
		if err := cloneZapret(zapretDir); err != nil {
			log("clone", "Clone failed: "+err.Error())
			fmt.Printf("ERROR: %s\n", err.Error())
			os.Exit(1)
		}
		log("clone", "Zapret cloned successfully")
	} else {
		log("clone", "Zapret already exists, pulling latest...")
		pullZapret(zapretDir)
	}

	cfg.SetZapretPath(zapretDir)
	cfg.Save()
	log("config", "Zapret path saved: "+zapretDir)

	// Step 3: Detect provider
	log("provider", "Detecting ISP...")
	isp := detectProvider()
	log("provider", "ISP: "+isp)

	// Step 4: Auto-test strategies
	log("autotest", "Starting strategy auto-test...")
	zapretMgr := zapret.NewManager(cfg, logMgr)

	strategies := zapretMgr.ListStrategies()
	log("autotest", fmt.Sprintf("Found %d strategies", len(strategies)))

	resultsCh := make(chan zapret.AutoTestResult, 50)
	doneCh := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	go zapretMgr.RunAutoTest(ctx, resultsCh, doneCh)

	completed := 0
	total := len(strategies)
	var bestStrategy string
	var bestScore float64

	for {
		select {
		case r, ok := <-resultsCh:
			if !ok {
				continue
			}
			if r.Type == "progress" {
				completed++
				pct := completed * 100 / total
				fmt.Printf("\r[autotest] %s (%d%%) — Discord:%v YouTube:%v %dms   ",
					r.Strategy, pct, r.DiscordOK, r.YouTubeOK, r.ResponseMs)

				score := 0.0
				if r.DiscordOK {
					score += 0.5
				}
				if r.YouTubeOK {
					score += 0.5
				}
				if r.ResourcesN > 0 {
					score += float64(r.ResourcesOK) / float64(r.ResourcesN) * 0.3
				}
				if r.ResponseMs > 0 {
					score -= float64(r.ResponseMs) / 10000.0
				}
				if score > bestScore {
					bestScore = score
					bestStrategy = r.Strategy
				}
			}
		case <-doneCh:
			fmt.Println()
			if bestStrategy != "" {
				log("autotest", "Best strategy: "+bestStrategy)
				cfg.SetCurrentStrategy(bestStrategy + ".bat")
				cfg.Save()
			} else {
				log("autotest", "No working strategy found")
			}
			goto done
		case <-ctx.Done():
			fmt.Println()
			log("autotest", "Timeout reached")
			goto done
		}
	}

done:
	cfg.FirstRunDone = true
	cfg.Save()
	log("done", "Wizard complete!")
	fmt.Println("\n=== Wizard complete ===")
	if bestStrategy != "" {
		fmt.Printf("Best strategy: %s\n", bestStrategy)
	}
}

func cloneZapret(dir string) error {
	cmd := executil.HiddenCmd("git", "clone", "--depth", "1", repoURL, dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func pullZapret(dir string) {
	cmd := executil.HiddenCmd("git", "-C", dir, "pull", "--ff-only")
	cmd.Run()
}

func installGit() error {
	cmd := executil.HiddenCmd("winget", "install", "--id", "Git.Git", "-e", "--source", "winget", "--accept-package-agreements", "--accept-source-agreements")
	return cmd.Run()
}

func detectProvider() string {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://ipinfo.io/json")
	if err != nil {
		return "unknown"
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var info struct {
		Org   string `json:"org"`
		City  string `json:"city"`
		Region string `json:"region"`
	}
	json.Unmarshal(body, &info)
	return info.Org
}
