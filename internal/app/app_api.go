package app

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zpui/internal/executil"
	"zpui/internal/monitor"
)

// ============================================================
// STATUS
// ============================================================

// GetSystemTheme — определяет тёмную/светлую тему Windows через реестр.
func (a *App) GetSystemTheme() string {
	out, err := executil.HiddenCmd("reg", "query",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`,
		"/v", "AppsUseLightTheme").Output()
	if err != nil {
		return "dark"
	}
	if strings.Contains(string(out), "0x0") {
		return "dark"
	}
	return "light"
}

// SaveComponentStates — сохраняет текущие состояния компонентов в конфиг.
func (a *App) SaveComponentStates() map[string]interface{} {
	zRun := a.zapret.GetStatus() == "running"
	pRun := a.proxy.IsRunning()

	a.cfg.LastZapretState = zRun
	a.cfg.LastProxyState = pRun
	a.cfg.LastXboxDnsState = a.cfg.XboxDns.Enabled
	if err := a.cfg.Save(); err != nil {
		a.log.Error("app", "SaveComponentStates error: "+err.Error())
	}
	return map[string]interface{}{
		"zapret":   zRun,
		"proxy":    pRun,
		"xbox_dns": a.cfg.XboxDns.Enabled,
	}
}

// GetStatus — агрегированный статус системы (замена GET /api/status).
func (a *App) GetStatus() map[string]interface{} {
	pcfg := a.cfg.GetProxyConfig()
	traffic := a.monitor.GetCurrentStats()

	ips := getLocalIPs()
	if len(ips) == 0 {
		ips = []string{"127.0.0.1"}
	}

	zapretStatus := string(a.zapret.GetStatus())
	strategy := a.zapret.GetCurrentStrategy()

	if zapretStatus == "running" {
		svcStatus := a.zapret.GetServiceStatus()
		if svcStatus.Strategy != "" {
			strategy = svcStatus.Strategy
		}
	}

	return map[string]interface{}{
		"zapret": map[string]interface{}{
			"status":           zapretStatus,
			"version":          a.zapret.GetVersion(),
			"strategy":         strategy,
			"zapretDir":        a.cfg.GetZapretPath(),
			"auto_test_running": a.zapret.IsAutoTestRunning(),
		},
		"proxy": map[string]interface{}{
			"running": a.proxy.IsRunning(),
			"port":    pcfg.Port,
			"stats":   a.proxy.GetStats(),
		},
		"xbox_dns": map[string]interface{}{
			"enabled":       a.cfg.XboxDns.Enabled,
			"primary_dns":   a.cfg.XboxDns.PrimaryDNS,
			"secondary_dns": a.cfg.XboxDns.SecondaryDNS,
		},
		"monitor": map[string]interface{}{
			"download_bytes": traffic.DownloadBytes,
			"upload_bytes":   traffic.UploadBytes,
			"download_speed": traffic.DownloadSpeed,
			"upload_speed":   traffic.UploadSpeed,
			"download_fmt":   monitor.FormatBytes(traffic.DownloadBytes),
			"upload_fmt":     monitor.FormatBytes(traffic.UploadBytes),
			"dl_speed_fmt":   monitor.FormatSpeed(traffic.DownloadSpeed),
			"ul_speed_fmt":   monitor.FormatSpeed(traffic.UploadSpeed),
		},
		"mod": map[string]interface{}{
			"version":         a.version,
			"autostart":       a.cfg.AutoStart,
			"web_port":        a.cfg.Web.Port,
			"zapret_repo":     a.cfg.ZapretRepoURL,
			"mod_repo":        a.cfg.ModRepoURL,
			"theme":           a.cfg.Theme,
			"first_run_done":  a.cfg.FirstRunDone,
			"start_minimized": a.cfg.StartMinimized,
			"close_to_tray":   a.cfg.GetCloseToTray(),
		},
		"network": map[string]interface{}{
			"hostname": getHostname(),
			"mac":      getMACAddress(),
			"ips":      ips,
		},
	}
}

// ============================================================
// RESTART
// ============================================================

func (a *App) RestartApp() map[string]interface{} {
	a.log.Info("app", "RestartApp called")
	exePath := getExePath()
	a.cfg.SetZapretSkipped(false)
	a.cfg.FirstRunDone = false
	a.cfg.LastZapretState = false
	if err := a.cfg.Save(); err != nil {
		a.log.Error("app", "RestartApp config save error: "+err.Error())
	}
	psScript := fmt.Sprintf("Start-Sleep -Seconds 2; Start-Process -FilePath '%s'", exePath)
	if err := executil.HiddenCmd("powershell", "-NoProfile", "-Command", psScript).Start(); err != nil {
		a.log.Error("app", "RestartApp spawn error: "+err.Error())
		return map[string]interface{}{"error": err.Error()}
	}
	go a.Quit()
	return map[string]interface{}{"status": "ok"}
}

// ============================================================
// HELPERS
// ============================================================

func getExePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
}

func mustReadFile(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return []byte("203.0.113.113/32\n")
	}
	return data
}

func downloadAndSave(url, destPath string) map[string]interface{} {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return map[string]interface{}{"error": fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	os.MkdirAll(filepath.Dir(destPath), 0755)
	if err := os.WriteFile(destPath, body, 0644); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"status": "ok"}
}
