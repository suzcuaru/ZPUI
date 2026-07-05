package wizard

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

	"zpui/internal/autoselect"
	"zpui/internal/config"
	"zpui/internal/executil"
	"zpui/internal/logger"
	"zpui/internal/zapret"
)

const repoURL = "https://github.com/Flowseal/zapret-discord-youtube"

// Progress — этап мастера настройки.
type Progress struct {
	Step    string `json:"step"`
	Message string `json:"message"`
	Percent int    `json:"percent,omitempty"`
}

// ProgressFn коллбэк прогресса (может быть nil).
type ProgressFn func(Progress)

// Options — параметры запуска мастера.
type Options struct {
	ExeDir     string
	Config     *config.Config
	Log        *logger.Logger
	OnProgress ProgressFn
}

// Result — итог мастера.
type Result struct {
	BestStrategy string `json:"best_strategy,omitempty"`
	ISP          string `json:"isp,omitempty"`
}

// Run выполняет полный цикл мастера настройки:
// git → клон zapret → определение ISP → автоподбор+применение стратегии.
func Run(ctx context.Context, opts Options) (*Result, error) {
	progress := opts.OnProgress
	if progress == nil {
		progress = func(Progress) {}
	}
	log := opts.Log
	report := func(step, msg string) {
		progress(Progress{Step: step, Message: msg})
		if log != nil {
			log.Info("wizard", fmt.Sprintf("[%s] %s", step, msg))
		}
	}

	cfg := opts.Config
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	report("init", "Wizard started")

	if _, err := exec.LookPath("git"); err != nil {
		report("git", "Git not found, installing...")
		if err := installGit(); err != nil {
			report("git", "Failed to install git: "+err.Error())
		}
	} else {
		report("git", "Git found")
	}

	zapretDir := filepath.Join(opts.ExeDir, "zapret")
	winws := filepath.Join(zapretDir, "bin", "winws.exe")
	if _, err := os.Stat(winws); err != nil {
		report("clone", fmt.Sprintf("Cloning zapret to %s...", zapretDir))
		if err := cloneZapret(zapretDir); err != nil {
			return nil, fmt.Errorf("clone failed: %w", err)
		}
		report("clone", "Zapret cloned successfully")
	} else {
		report("clone", "Zapret already exists, pulling latest...")
		pullZapret(zapretDir)
	}

	cfg.SetZapretPath(zapretDir)
	cfg.Save()
	report("config", "Zapret path saved: "+zapretDir)

	report("provider", "Detecting ISP...")
	isp := detectProvider()
	report("provider", "ISP: "+isp)

	report("autotest", "Starting strategy auto-test...")
	mgr := zapret.NewManager(cfg, log)
	total := len(mgr.ListStrategies())
	report("autotest", fmt.Sprintf("Found %d strategies", total))

	completed := 0
	ar := autoselect.RunWithManager(ctx, mgr, func(r zapret.AutoTestResult) {
		if r.Type == "result" && r.Strategy != "" && r.ResourcesN > 0 && total > 0 {
			completed++
			pct := completed * 100 / total
			progress(Progress{
				Step:    "autotest",
				Message: fmt.Sprintf("%s — %d/%d %dms", r.Strategy, r.ResourcesOK, r.ResourcesN, r.ResponseMs),
				Percent: pct,
			})
		} else if r.Message != "" {
			report("autotest", r.Message)
		}
	})

	cfg.FirstRunDone = true
	cfg.Save()

	if ar.Error != "" {
		report("done", "Wizard finished with error: "+ar.Error)
		return &Result{ISP: isp, BestStrategy: ar.Strategy}, fmt.Errorf("%s", ar.Error)
	}

	report("done", "Wizard complete! Best strategy: "+ar.Strategy)
	return &Result{ISP: isp, BestStrategy: ar.Strategy}, nil
}

func cloneZapret(dir string) error {
	cmd := executil.HiddenCmd("git", "clone", "--depth", "1", repoURL, dir)
	return cmd.Run()
}

func pullZapret(dir string) {
	executil.HiddenCmd("git", "-C", dir, "pull", "--ff-only").Run()
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
		Org    string `json:"org"`
		City   string `json:"city"`
		Region string `json:"region"`
	}
	json.Unmarshal(body, &info)
	return info.Org
}
