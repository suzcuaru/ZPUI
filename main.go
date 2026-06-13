package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"zpui/internal/config"
	"zpui/internal/executil"
	"zpui/internal/logger"
	"zpui/internal/monitor"
	"zpui/internal/proxy"
	"zpui/internal/tray"
	"zpui/internal/web"
	"zpui/internal/zapret"
)

//go:embed all:web/dist
var webFS embed.FS

var (
	version    = "1.0.0"
	configPath string
)

func main() {
	if !ensureAdmin() {
		return
	}

	exePath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	exeDir := filepath.Dir(exePath)

	configPath = filepath.Join(exeDir, "config.json")

	zapretDir := findZapretDir(exeDir)

	cfg := config.Load(configPath, zapretDir)
	cfg.ModVersion = version

	logMgr, err := logger.New(cfg.LogsDir(), 7)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logMgr.Close()

	logMgr.Info("main", fmt.Sprintf("ZPUI v%s starting...", version))
	logMgr.Info("main", fmt.Sprintf("Zapret dir: %s", cfg.ZapretPath))
	logMgr.Info("main", fmt.Sprintf("Go %s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH))

	zapretMgr := zapret.NewManager(cfg, logMgr)

	proxyServer := proxy.NewSOCKS5(cfg, logMgr)

	trafficMonitor := monitor.NewTrafficMonitor(logMgr)

	webServer := web.NewServer(cfg, logMgr, zapretMgr, proxyServer, trafficMonitor, webFS, version)

	trayApp := tray.New(cfg, logMgr, zapretMgr, proxyServer, webServer, version)

	if cfg.Proxy.AutoStart {
		go func() {
			if err := proxyServer.Start(); err != nil {
				logMgr.Error("proxy", fmt.Sprintf("Auto-start proxy failed: %v", err))
			}
		}()
	}

	go func() {
		logMgr.Info("web", "Starting web server on 127.0.0.1:0")
		if err := webServer.Start("127.0.0.1:0"); err != nil {
			logMgr.Error("web", fmt.Sprintf("Web server error: %v", err))
		}
	}()

	webServer.WaitReady()
	logMgr.Info("web", fmt.Sprintf("Web server URL: %s", webServer.GetURL()))

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logMgr.Info("main", "Shutdown signal received")
		gracefulShutdown(zapretMgr, proxyServer, trafficMonitor, webServer, trayApp, logMgr, cfg)
		os.Exit(0)
	}()

	logMgr.Info("main", "Starting system tray...")

	tray.HideConsole()

	if err := trayApp.Run(); err != nil {
		logMgr.Error("tray", fmt.Sprintf("Tray error: %v", err))
		gracefulShutdown(zapretMgr, proxyServer, trafficMonitor, webServer, trayApp, logMgr, cfg)
		os.Exit(1)
	}
}

func findZapretDir(exeDir string) string {
	candidates := []string{
		exeDir,
		filepath.Dir(exeDir),
		filepath.Join(exeDir, "zapret"),
	}
	for _, dir := range candidates {
		winws := filepath.Join(dir, "bin", "winws.exe")
		if _, err := os.Stat(winws); err == nil {
			return dir
		}
	}
	return exeDir
}

func gracefulShutdown(
	zapretMgr *zapret.Manager,
	proxyServer *proxy.SOCKS5Server,
	trafficMonitor *monitor.TrafficMonitor,
	webServer *web.Server,
	trayApp *tray.App,
	logMgr *logger.Logger,
	cfg *config.Config,
) {
	logMgr.Info("main", "Shutting down...")
	proxyServer.Stop()
	trafficMonitor.Stop()
	webServer.Stop()
	logMgr.Info("main", "Shutdown complete")
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