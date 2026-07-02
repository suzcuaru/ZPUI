package main

import (
	"context"
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

	zapretMgr := zapret.NewManager(cfg, logMgr)

	strategies := zapretMgr.ListStrategies()
	if len(strategies) == 0 {
		log("ERROR: No strategies found in " + cfg.GetZapretPath())
		os.Exit(1)
	}

	log(fmt.Sprintf("Testing %d strategies...", len(strategies)))

	if zapretMgr.GetStatus() == zapret.StatusRunning {
		log("Stopping current zapret instance...")
		zapretMgr.Stop()
		time.Sleep(2 * time.Second)
	}

	resultsCh := make(chan zapret.AutoTestResult, 50)
	doneCh := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	go zapretMgr.RunAutoTest(ctx, resultsCh, doneCh)

	completed := 0
	total := len(strategies)
	type result struct {
		Strategy   string
		DiscordOK  bool
		YouTubeOK  bool
		ResourcesN int
		ResourcesOK int
		ResponseMs int64
		Score      float64
	}
	var results []result

	for {
		select {
		case r, ok := <-resultsCh:
			if !ok {
				continue
			}
			if r.Type == "progress" {
				completed++
				pct := completed * 100 / total
				fmt.Printf("\r[%3d%%] %-40s D:%v Y:%v  %dms   ",
					pct, r.Strategy, r.DiscordOK, r.YouTubeOK, r.ResponseMs)

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
				results = append(results, result{
					Strategy: r.Strategy, DiscordOK: r.DiscordOK, YouTubeOK: r.YouTubeOK,
					ResourcesN: r.ResourcesN, ResourcesOK: r.ResourcesOK,
					ResponseMs: r.ResponseMs, Score: score,
				})
			}
		case <-doneCh:
			fmt.Println()
			goto done
		case <-ctx.Done():
			fmt.Println()
			log("Timeout reached")
			goto done
		}
	}

done:
	if len(results) == 0 {
		log("No results")
		os.Exit(1)
	}

	best := results[0]
	for _, r := range results[1:] {
		if r.Score > best.Score {
			best = r
		}
	}

	log(fmt.Sprintf("Best: %s (D:%v Y:%v %dms score:%.2f)", best.Strategy, best.DiscordOK, best.YouTubeOK, best.ResponseMs, best.Score))

	cfg.SetCurrentStrategy(best.Strategy + ".bat")
	cfg.Save()
	log("Strategy saved: " + best.Strategy + ".bat")

	fmt.Println("\n=== Results ===")
	for i, r := range results {
		marker := ""
		if r.Strategy == best.Strategy {
			marker = " *"
		}
		fmt.Printf("%2d. %-40s D:%-5v Y:%-5v  %4dms  %.2f%s\n",
			i+1, r.Strategy, r.DiscordOK, r.YouTubeOK, r.ResponseMs, r.Score, marker)
	}
}
