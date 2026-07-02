package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/blockcheck"
	"zpui/internal/database"
	"zpui/internal/executil"
	"zpui/internal/updater"
	"zpui/internal/monitor"
	"zpui/internal/sysinfo"
	"zpui/internal/zapret"
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
			"status":    zapretStatus,
			"version":   a.zapret.GetVersion(),
			"strategy":  strategy,
			"zapretDir": a.cfg.GetZapretPath(),
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
			"version":        a.version,
			"autostart":      a.cfg.AutoStart,
			"web_port":       a.cfg.Web.Port,
			"zapret_repo":    a.cfg.ZapretRepoURL,
			"mod_repo":       a.cfg.ModRepoURL,
			"theme":          a.cfg.Theme,
		"first_run_done": a.cfg.FirstRunDone,
		"start_minimized": a.cfg.StartMinimized,
		"close_to_tray":  a.cfg.GetCloseToTray(),
	},
		"network": map[string]interface{}{
			"hostname": getHostname(),
			"mac":      getMACAddress(),
			"ips":      ips,
		},
	}
}

// ============================================================
// ZAPRET CONTROL
// ============================================================

func (a *App) ZapretStart() map[string]interface{} {
	if err := a.zapret.Start(); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"status": "started"}
}

func (a *App) ZapretStop() map[string]interface{} {
	a.zapret.Stop()
	return map[string]interface{}{"status": "stopped"}
}

func (a *App) ZapretRestart() map[string]interface{} {
	if err := a.zapret.Restart(); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"status": "restarted"}
}

// ============================================================
// STRATEGIES
// ============================================================

func (a *App) GetStrategies() map[string]interface{} {
	return map[string]interface{}{"strategies": a.zapret.ListStrategies()}
}

func (a *App) SetStrategy(filename string) map[string]interface{} {
	if filename == "" {
		return map[string]interface{}{"error": "filename required"}
	}
	if err := a.zapret.SetStrategy(filename); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"status": "ok", "strategy": filename}
}

// ============================================================
// WINDOWS SERVICE
// ============================================================

func (a *App) InstallService(strategy string) map[string]interface{} {
	if strategy == "" {
		strategy = a.zapret.GetCurrentStrategy()
	}
	if err := a.zapret.InstallService(strategy); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"status": "installed"}
}

func (a *App) RemoveService() map[string]interface{} {
	a.zapret.RemoveService()
	return map[string]interface{}{"status": "removed"}
}

func (a *App) GetServiceStatus() interface{} {
	return a.zapret.GetServiceStatus()
}

// InstallServiceLogged — устанавливает службу запрета, записывая процесс в
// logs/install.log (перезаписываемый), с проверкой что служба отвечает.
func (a *App) InstallServiceLogged(strategy string) map[string]interface{} {
	res, err := a.zapret.InstallServiceLogged(strategy)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{
		"success":  res.Success,
		"version":  res.Version,
		"strategy": res.Strategy,
		"running":  res.Running,
		"errors":   res.Errors,
	}
}

// GetInstallLog — содержимое logs/install.log (для показа ошибок пользователю).
func (a *App) GetInstallLog() map[string]interface{} {
	logPath := filepath.Join(a.cfg.LogsDir(), "install.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return map[string]interface{}{"lines": []string{}}
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return map[string]interface{}{"lines": []string{}}
	}
	return map[string]interface{}{"lines": strings.Split(content, "\n")}
}

// DefaultStrategy — стратегия по умолчанию (первый general ALT).
func (a *App) DefaultStrategy() map[string]interface{} {
	return map[string]interface{}{"strategy": a.zapret.DefaultStrategyName()}
}

// ============================================================
// GAME FILTER
// ============================================================

func (a *App) GetGameFilter() map[string]interface{} {
	mode, _, _ := a.zapret.LoadGameFilter()
	return map[string]interface{}{"mode": mode}
}

func (a *App) SetGameFilter(mode string) map[string]interface{} {
	if err := a.zapret.SetGameFilter(mode); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"status": "ok"}
}

// ============================================================
// PROXY
// ============================================================

func (a *App) ProxyStart() map[string]interface{} {
	if err := a.proxy.Start(); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"status": "started"}
}

func (a *App) ProxyStop() map[string]interface{} {
	a.proxy.Stop()
	return map[string]interface{}{"status": "stopped"}
}

func (a *App) GetProxyStatus() map[string]interface{} {
	pcfg := a.cfg.GetProxyConfig()
	return map[string]interface{}{
		"running":    a.proxy.IsRunning(),
		"port":       pcfg.Port,
		"username":   pcfg.Username,
		"auto_start": pcfg.AutoStart,
		"stats":      a.proxy.GetStats(),
	}
}

func (a *App) GetProxyConnections() map[string]interface{} {
	conns := a.proxy.GetConnections()
	byClient := a.proxy.GetActiveConnectionsByClient()
	arpTable := getARPTable()

	deviceInfo := make(map[string]map[string]string)
	for ip := range byClient {
		deviceInfo[ip] = map[string]string{
			"hostname": resolveHostname(ip),
			"mac":      arpTable[ip],
		}
	}

	return map[string]interface{}{
		"connections": conns,
		"by_client":   byClient,
		"device_info": deviceInfo,
	}
}

func (a *App) GetProxyConfig() interface{} {
	return a.cfg.GetProxyConfig()
}

func (a *App) SetProxyConfig(opts map[string]interface{}) map[string]interface{} {
	a.log.Info("proxy", "SetProxyConfig called")
	pcfg := a.cfg.GetProxyConfig()
	if port, ok := opts["port"].(float64); ok {
		pcfg.Port = int(port)
	}
	if user, ok := opts["username"].(string); ok {
		pcfg.Username = user
	}
	if pass, ok := opts["password"].(string); ok {
		pcfg.Password = pass
	}
	if as, ok := opts["auto_start"].(bool); ok {
		pcfg.AutoStart = as
	}
	a.cfg.SetProxyConfig(pcfg)
	if err := a.cfg.Save(); err != nil {
		a.log.Error("proxy", "Save error: "+err.Error())
	} else {
		a.log.Info("proxy", "Proxy config saved to JSON")
	}
	return map[string]interface{}{"status": "ok"}
}

func (a *App) GetProxyQRCode() map[string]interface{} {
	pcfg := a.cfg.GetProxyConfig()
	ips := getLocalIPs()
	if len(ips) == 0 {
		ips = []string{"127.0.0.1"}
	}
	return map[string]interface{}{
		"ips":      ips,
		"port":     pcfg.Port,
		"username": pcfg.Username,
		"password": pcfg.Password,
	}
}

// ============================================================
// MONITOR
// ============================================================

func (a *App) GetTraffic() map[string]interface{} {
	stats := a.monitor.GetCurrentStats()
	return map[string]interface{}{
		"download_bytes": stats.DownloadBytes,
		"upload_bytes":   stats.UploadBytes,
		"download_speed": stats.DownloadSpeed,
		"upload_speed":   stats.UploadSpeed,
		"download_fmt":   monitor.FormatBytes(stats.DownloadBytes),
		"upload_fmt":     monitor.FormatBytes(stats.UploadBytes),
		"dl_speed_fmt":   monitor.FormatSpeed(stats.DownloadSpeed),
		"ul_speed_fmt":   monitor.FormatSpeed(stats.UploadSpeed),
		"timestamp":      stats.Timestamp,
	}
}

func (a *App) GetMonitorDevices() map[string]interface{} {
	byClient := a.proxy.GetActiveConnectionsByClient()
	arpTable := getARPTable()

	type DeviceInfo struct {
		IP          string `json:"ip"`
		Hostname    string `json:"hostname"`
		MAC         string `json:"mac"`
		Connections int    `json:"connections"`
	}
	var devices []DeviceInfo
	for ip, conns := range byClient {
		hostname := resolveHostname(ip)
		mac := arpTable[ip]
		devices = append(devices, DeviceInfo{
			IP:          ip,
			Hostname:    hostname,
			MAC:         mac,
			Connections: len(conns),
		})
	}
	if devices == nil {
		devices = []DeviceInfo{}
	}
	return map[string]interface{}{"devices": devices}
}

// ============================================================
// UPDATES
// ============================================================

func (a *App) CheckForUpdates() interface{} {
	info, err := a.zapret.CheckForUpdates()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return info
}

func (a *App) ApplyUpdate() map[string]interface{} {
	a.saveBackupToDB()
	progress := make(chan zapret.UpdateProgress, 20)
	go a.zapret.PerformUpdate(progress)
	return map[string]interface{}{"status": "started"}
}

// ============================================================
// AUTO TEST (STRATEGY)
// ============================================================

func (a *App) StartAutoTest() map[string]interface{} {
	if a.zapret.IsAutoTestRunning() {
		return map[string]interface{}{"error": "Автотест уже запущен"}
	}
	return map[string]interface{}{"status": "started"}
}

func (a *App) CancelAutoTest() map[string]interface{} {
	a.zapret.CancelAutoTest()
	return map[string]interface{}{"status": "cancelled"}
}

// ============================================================
// AUTOSTART
// ============================================================

func (a *App) GetAutostartStatus() map[string]interface{} {
	return map[string]interface{}{"enabled": a.cfg.AutoStart}
}

func (a *App) EnableAutostart() map[string]interface{} {
	a.log.Info("autostart", "EnableAutostart called")
	a.cfg.AutoStart = true
	if err := a.cfg.Save(); err != nil {
		a.log.Error("autostart", "Save error: "+err.Error())
	} else {
		a.log.Info("autostart", "Autostart config saved")
	}

	go func() {
		exePath := getExePath()
		a.log.Info("autostart", "Creating scheduled task...")
		executil.HiddenCmd("schtasks", "/delete", "/tn", "ZPUI", "/f").Run()
		cmd := executil.HiddenCmd("schtasks", "/create",
			"/tn", "ZPUI",
			"/tr", fmt.Sprintf(`"%s"`, exePath),
			"/sc", "onlogon",
			"/rl", "highest",
			"/f")
		output, err := cmd.CombinedOutput()
		if err != nil {
			a.log.Error("autostart", fmt.Sprintf("Task Scheduler error: %v: %s", err, string(output)))
			executil.HiddenCmd("reg", "add",
				`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
				"/v", "ZPUI", "/t", "REG_SZ", "/d",
				fmt.Sprintf(`"%s"`, exePath), "/f").Run()
		} else {
			a.log.Info("autostart", "Scheduled task created")
		}
	}()

	return map[string]interface{}{"status": "enabled"}
}

func (a *App) DisableAutostart() map[string]interface{} {
	a.log.Info("autostart", "DisableAutostart called")
	a.cfg.AutoStart = false
	if err := a.cfg.Save(); err != nil {
		a.log.Error("autostart", "Save error: "+err.Error())
	} else {
		a.log.Info("autostart", "Autostart disabled, config saved")
	}

	go func() {
		executil.HiddenCmd("schtasks", "/delete", "/tn", "ZPUI", "/f").Run()
		executil.HiddenCmd("reg", "delete",
			`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
			"/v", "ZPUI", "/f").Run()
	}()

	return map[string]interface{}{"status": "disabled"}
}

// ============================================================
// LOGS
// ============================================================

func (a *App) GetLogs(category string, lines int) map[string]interface{} {
	if category == "" {
		category = "zapret"
	}
	if lines <= 0 {
		lines = 100
	}
	logLines := a.log.ReadRecent(category, lines)
	return map[string]interface{}{"lines": logLines}
}

func (a *App) GetLogFiles() map[string]interface{} {
	return map[string]interface{}{"files": a.log.ListLogFiles()}
}

func (a *App) ClearLogs() map[string]interface{} {
	a.log.Clear()
	return map[string]interface{}{"status": "ok"}
}

func (a *App) GetErrorSnapshots() map[string]interface{} {
	errorsDir := filepath.Join(a.cfg.LogsDir(), "errors")
	entries, err := os.ReadDir(errorsDir)
	if err != nil {
		return map[string]interface{}{"files": []interface{}{}}
	}
	var files []map[string]interface{}
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.IsDir() {
			continue
		}
		info, _ := e.Info()
		files = append(files, map[string]interface{}{
			"name": e.Name(),
			"size": info.Size(),
			"date": info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}
	return map[string]interface{}{"files": files}
}

func (a *App) ReadErrorSnapshot(name string) map[string]interface{} {
	if name == "" {
		return map[string]interface{}{"error": "name required"}
	}
	path := filepath.Join(a.cfg.LogsDir(), "errors", name)
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"content": string(data), "name": name}
}

func (a *App) GetArchiveLogs() map[string]interface{} {
	archiveDir := filepath.Join(a.cfg.LogsDir(), "archive")
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		return map[string]interface{}{"files": []interface{}{}}
	}
	var files []map[string]interface{}
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.IsDir() {
			continue
		}
		info, _ := e.Info()
		files = append(files, map[string]interface{}{
			"name": e.Name(),
			"size": info.Size(),
			"date": info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}
	return map[string]interface{}{"files": files}
}

func (a *App) ReadArchiveLog(name string) map[string]interface{} {
	if name == "" {
		return map[string]interface{}{"error": "name required"}
	}
	path := filepath.Join(a.cfg.LogsDir(), "archive", name)
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"content": string(data), "name": name}
}

// ============================================================
// CONFIG
// ============================================================

func (a *App) GetConfig() map[string]interface{} {
	return map[string]interface{}{
		"current_strategy":    a.cfg.GetCurrentStrategy(),
		"web_port":            a.cfg.Web.Port,
		"proxy":               a.cfg.GetProxyConfig(),
		"xbox_dns":            a.cfg.GetXboxDnsConfig(),
		"autostart":           a.cfg.AutoStart,
		"auto_update_check":   a.cfg.AutoUpdateCheck,
		"theme":               a.cfg.Theme,
		"first_run_done":      a.cfg.FirstRunDone,
		"start_minimized":     a.cfg.StartMinimized,
		"close_to_tray":       a.cfg.GetCloseToTray(),
		"last_zapret_state":   a.cfg.LastZapretState,
		"last_proxy_state":    a.cfg.LastProxyState,
		"last_xbox_dns_state": a.cfg.LastXboxDnsState,
		"auto_start_zapret":   a.cfg.AutoStartZapret,
		"auto_start_proxy":    a.cfg.AutoStartProxy,
		"auto_start_xbox_dns": a.cfg.AutoStartXboxDns,
		"logs":                a.cfg.Logs,
	}
}

func (a *App) SetConfig(opts map[string]interface{}) map[string]interface{} {
	a.log.Info("config", "SetConfig called")
	if port, ok := opts["web_port"].(float64); ok {
		a.cfg.Web.Port = int(port)
	}
	if theme, ok := opts["theme"].(string); ok {
		a.cfg.SetTheme(theme)
	}
	if v, ok := opts["start_minimized"].(bool); ok {
		a.cfg.StartMinimized = v
	}
	if v, ok := opts["close_to_tray"].(bool); ok {
		a.cfg.CloseToTray = v
	}
	if v, ok := opts["auto_start_zapret"].(bool); ok {
		a.cfg.AutoStartZapret = v
	}
	if v, ok := opts["auto_start_proxy"].(bool); ok {
		a.cfg.AutoStartProxy = v
	}
	if v, ok := opts["auto_start_xbox_dns"].(bool); ok {
		a.cfg.AutoStartXboxDns = v
	}
	if v, ok := opts["first_run_done"].(bool); ok {
		a.cfg.FirstRunDone = v
	}
	if err := a.cfg.Save(); err != nil {
		a.log.Error("config", "Save error: "+err.Error())
	} else {
		a.log.Info("config", "Config saved to JSON")
	}
	return map[string]interface{}{"status": "ok"}
}

// ============================================================
// XBOX DNS CONFIG
// ============================================================

func (a *App) GetXboxDnsConfig() interface{} {
	return a.cfg.GetXboxDnsConfig()
}

func (a *App) SetXboxDnsConfig(opts map[string]interface{}) map[string]interface{} {
	cfg := a.cfg.GetXboxDnsConfig()
	wasEnabled := cfg.Enabled
	if v, ok := opts["enabled"].(bool); ok {
		cfg.Enabled = v
	}
	if v, ok := opts["primary_dns"].(string); ok {
		cfg.PrimaryDNS = v
	}
	if v, ok := opts["secondary_dns"].(string); ok {
		cfg.SecondaryDNS = v
	}
	a.cfg.SetXboxDnsConfig(cfg)
	a.log.Info("xbox_dns", "Xbox DNS config saved")

	if cfg.Enabled && !wasEnabled {
		a.xboxDns.Configure(cfg.PrimaryDNS, cfg.SecondaryDNS)
		go func() {
			if err := a.xboxDns.Enable(); err != nil {
				a.log.Error("xbox_dns", "Enable failed: "+err.Error())
			}
		}()
	} else if !cfg.Enabled && wasEnabled {
		go func() {
			if err := a.xboxDns.Disable(); err != nil {
				a.log.Error("xbox_dns", "Disable failed: "+err.Error())
			}
		}()
	}

	return map[string]interface{}{"status": "ok"}
}

func (a *App) ToggleXboxDns(enabled bool) map[string]interface{} {
	cfg := a.cfg.GetXboxDnsConfig()
	wasEnabled := cfg.Enabled
	cfg.Enabled = enabled
	a.cfg.SetXboxDnsConfig(cfg)

	if enabled && !wasEnabled {
		a.xboxDns.Configure(cfg.PrimaryDNS, cfg.SecondaryDNS)
		go func() {
			if err := a.xboxDns.Enable(); err != nil {
				a.log.Error("xbox_dns", "Enable failed: "+err.Error())
			}
		}()
		return map[string]interface{}{"status": "starting"}
	} else if !enabled && wasEnabled {
		go func() {
			if err := a.xboxDns.Disable(); err != nil {
				a.log.Error("xbox_dns", "Disable failed: "+err.Error())
			}
		}()
		return map[string]interface{}{"status": "stopping"}
	}
	return map[string]interface{}{"status": "nochange"}
}

// ============================================================
// ZAPRET INSTALL
// ============================================================

func (a *App) InstallZapret(sourceDir string) map[string]interface{} {
	if sourceDir == "" {
		return map[string]interface{}{"error": "source_dir required"}
	}
	a.saveBackupToDB()
	progress := make(chan zapret.UpdateProgress, 20)
	go a.zapret.InstallZapret(sourceDir, progress)
	return map[string]interface{}{"status": "started"}
}

// saveBackupToDB сохраняет слепок состояния zapret в базу данных перед обновлением.
// При следующем запуске, если zapret повреждён, состояние будет восстановлено.
func (a *App) saveBackupToDB() {
	snap := a.zapret.CaptureState()
	if data, err := json.Marshal(snap); err == nil {
		if err := database.SaveZapretBackup(string(data)); err != nil {
			a.log.Warn("app", "Не удалось сохранить backup в базу: "+err.Error())
		}
	}
}

// ============================================================
// CACHE CLEAR
// ============================================================

func (a *App) ClearCache(target string) map[string]interface{} {
	cleared := []string{}

	if target == "discord" || target == "all" {
		discordPaths := []string{
			os.Getenv("APPDATA") + `\discord\Cache`,
			os.Getenv("APPDATA") + `\discord\Code Cache`,
			os.Getenv("APPDATA") + `\discord\GPUCache`,
			os.Getenv("APPDATA") + `\discord\Service Worker\CacheStorage`,
			os.Getenv("APPDATA") + `\discord\Service Worker\ScriptCache`,
		}
		for _, p := range discordPaths {
			if err := os.RemoveAll(p); err == nil {
				cleared = append(cleared, filepath.Base(p))
			}
		}
	}

	if target == "network" || target == "all" {
		executil.HiddenCmd("ipconfig", "/flushdns").Run()
		executil.HiddenCmd("netsh", "winsock", "reset").Run()
		executil.HiddenCmd("netsh", "int", "ip", "reset").Run()
		cleared = append(cleared, "DNS cache", "Winsock", "IP stack")
	}

	if len(cleared) > 0 {
		return map[string]interface{}{"status": "ok", "cleared": cleared}
	}
	return map[string]interface{}{"status": "nothing", "cleared": []string{}}
}

// ============================================================
// LISTS
// ============================================================

func (a *App) GetLists() map[string]interface{} {
	listsDir := a.cfg.ListsDir()
	listFiles := []string{
		"list-general.txt",
		"list-general-user.txt",
		"list-exclude.txt",
		"list-exclude-user.txt",
		"ipset-all.txt",
		"ipset-exclude.txt",
		"ipset-exclude-user.txt",
	}
	type ListInfo struct {
		Name     string   `json:"name"`
		Lines    []string `json:"lines"`
		Count    int      `json:"count"`
		Editable bool     `json:"editable"`
	}
	var result []ListInfo
	for _, f := range listFiles {
		path := filepath.Join(listsDir, f)
		data, err := os.ReadFile(path)
		lines := []string{}
		if err == nil {
			for _, l := range strings.Split(string(data), "\n") {
				l = strings.TrimSpace(l)
				if l != "" && !strings.HasPrefix(l, "#") {
					lines = append(lines, l)
				}
			}
		}
		editable := strings.HasSuffix(f, "-user.txt")
		result = append(result, ListInfo{Name: f, Lines: lines, Count: len(lines), Editable: editable})
	}
	return map[string]interface{}{"lists": result}
}

func (a *App) SaveList(name string, content string) map[string]interface{} {
	if name == "" {
		return map[string]interface{}{"error": "name required"}
	}
	if !strings.HasSuffix(name, "-user.txt") {
		return map[string]interface{}{"error": "only user lists are editable"}
	}
	listsDir := a.cfg.ListsDir()
	path := filepath.Join(listsDir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"status": "ok"}
}

// ============================================================
// RESOURCE STATUS
// ============================================================

func (a *App) GetResourceStatus() map[string]interface{} {
	a.resourceCacheMu.Lock()
	if time.Since(a.resourceCacheTime) < 30*time.Second && a.resourceCache != nil {
		a.resourceCacheMu.Unlock()
		return map[string]interface{}{
			"default": a.resourceCache.Default,
			"user":    a.resourceCache.User,
		}
	}
	a.resourceCacheMu.Unlock()

	dialer := &net.Dialer{Timeout: 3 * time.Second}
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext:     dialer.DialContext,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	type check struct {
		host string
		url  string
	}

	var defaultResources []check
	targetsPath := filepath.Join(a.cfg.GetZapretPath(), "utils", "targets.txt")
	if body, err := os.ReadFile(targetsPath); err == nil {
		for _, line := range strings.Split(string(body), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			val = strings.Trim(val, `"`)
			if strings.HasPrefix(val, "PING:") {
				continue
			}
			if !strings.HasPrefix(val, "http://") && !strings.HasPrefix(val, "https://") {
				continue
			}
			defaultResources = append(defaultResources, check{host: key, url: val})
		}
	}

	var userHosts []string
	if body, err := os.ReadFile(filepath.Join(a.cfg.ListsDir(), "list-general-user.txt")); err == nil {
		for _, line := range strings.Split(string(body), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				userHosts = append(userHosts, line)
			}
		}
	}

	var allChecks []check
	allChecks = append(allChecks, defaultResources...)
	for _, h := range userHosts {
		allChecks = append(allChecks, check{host: h, url: "https://" + h})
	}

	results := make([]map[string]interface{}, len(allChecks))
	var wg sync.WaitGroup
	for i, c := range allChecks {
		wg.Add(1)
		go func(idx int, c check) {
			defer wg.Done()
			ok := false
			resp, err := client.Get(c.url)
			if err == nil {
				resp.Body.Close()
				ok = resp.StatusCode < 500
			}
			if !ok {
				conn, err := net.DialTimeout("tcp", c.host+":443", 3*time.Second)
				if err == nil {
					conn.Close()
					ok = true
				}
			}
			results[idx] = map[string]interface{}{
				"name": c.host,
				"url":  c.url,
				"ok":   ok,
			}
		}(i, c)
	}
	wg.Wait()

	var defaultResults, userResults []map[string]interface{}
	for i, r := range results {
		if i < len(defaultResources) {
			defaultResults = append(defaultResults, r)
		} else {
			userResults = append(userResults, r)
		}
	}

	a.resourceCacheMu.Lock()
	a.resourceCache = &resCacheData{Default: defaultResults, User: userResults}
	a.resourceCacheTime = time.Now()
	a.resourceCacheMu.Unlock()

	return map[string]interface{}{
		"default": defaultResults,
		"user":    userResults,
	}
}

// ============================================================
// IPSET
// ============================================================

func (a *App) GetIpsetStatus() map[string]interface{} {
	listFile := filepath.Join(a.cfg.ListsDir(), "ipset-all.txt")
	data, err := os.ReadFile(listFile)
	if err != nil {
		return map[string]interface{}{"status": "any"}
	}
	content := strings.TrimSpace(string(data))
	lines := strings.Split(content, "\n")
	nonEmpty := 0
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			nonEmpty++
		}
	}
	status := "loaded"
	if nonEmpty == 0 {
		status = "any"
	} else if nonEmpty == 1 && strings.Contains(content, "203.0.113.113") {
		status = "none"
	}
	return map[string]interface{}{"status": status}
}

func (a *App) ToggleIpset(mode string) map[string]interface{} {
	listFile := filepath.Join(a.cfg.ListsDir(), "ipset-all.txt")
	backupFile := listFile + ".backup"

	switch mode {
	case "none":
		os.WriteFile(backupFile, mustReadFile(listFile), 0644)
		os.WriteFile(listFile, []byte("203.0.113.113/32\n"), 0644)
	case "any":
		os.WriteFile(listFile, []byte(""), 0644)
	case "loaded":
		if backup, err := os.ReadFile(backupFile); err == nil {
			os.WriteFile(listFile, backup, 0644)
		}
	}
	return map[string]interface{}{"status": "ok"}
}

// ============================================================
// AUTO UPDATE FLAG
// ============================================================

func (a *App) GetAutoUpdateStatus() map[string]interface{} {
	flagFile := filepath.Join(a.cfg.GetZapretPath(), "utils", "check_updates.enabled")
	_, err := os.Stat(flagFile)
	return map[string]interface{}{"enabled": err == nil}
}

func (a *App) ToggleAutoUpdate(enabled bool) map[string]interface{} {
	flagFile := filepath.Join(a.cfg.GetZapretPath(), "utils", "check_updates.enabled")
	utilsDir := filepath.Join(a.cfg.GetZapretPath(), "utils")
	os.MkdirAll(utilsDir, 0755)
	if enabled {
		os.WriteFile(flagFile, []byte("ENABLED"), 0644)
	} else {
		os.Remove(flagFile)
	}
	return map[string]interface{}{"status": "ok"}
}

// ============================================================
// UPDATE IPSET / HOSTS
// ============================================================

func (a *App) UpdateIpset() map[string]interface{} {
	listFile := filepath.Join(a.cfg.ListsDir(), "ipset-all.txt")
	url := "https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/refs/heads/main/.service/ipset-service.txt"
	return downloadAndSave(url, listFile)
}

func (a *App) UpdateHosts() map[string]interface{} {
	url := "https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/refs/heads/main/.service/hosts"
	tmpFile := filepath.Join(os.TempDir(), "zapret_hosts.txt")
	return downloadAndSave(url, tmpFile)
}

// ============================================================
// UP INFO
// ============================================================

func (a *App) GetUpInfo() map[string]interface{} {
	ips := getLocalIPs()
	if len(ips) == 0 {
		ips = []string{"127.0.0.1"}
	}
	traffic := a.monitor.GetCurrentStats()
	pcfg := a.cfg.GetProxyConfig()

	return map[string]interface{}{
		"hostname": getHostname(),
		"ip":       ips,
		"mac":      getMACAddress(),
		"zapret": map[string]interface{}{
			"status":   string(a.zapret.GetStatus()),
			"version":  a.zapret.GetVersion(),
			"strategy": a.zapret.GetCurrentStrategy(),
		},
		"proxy": map[string]interface{}{
			"running": a.proxy.IsRunning(),
			"port":    pcfg.Port,
		},
		"traffic": map[string]interface{}{
			"download": monitor.FormatBytes(traffic.DownloadBytes),
			"upload":   monitor.FormatBytes(traffic.UploadBytes),
			"dl_speed": monitor.FormatSpeed(traffic.DownloadSpeed),
			"ul_speed": monitor.FormatSpeed(traffic.UploadSpeed),
		},
		"mod_version": a.version,
		"timestamp":   time.Now().Format("2006-01-02 15:04:05"),
	}
}

// ============================================================
// EXTERNAL
// ============================================================

func (a *App) OpenExternal(url string) map[string]interface{} {
	if url == "" {
		return map[string]interface{}{"error": "url required"}
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return map[string]interface{}{"error": "only http/https URLs allowed"}
	}
	executil.HiddenCmd("cmd", "/c", "start", "", url).Start()
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

// ============================================================
// SYSTEM RESOURCES
// ============================================================

func (a *App) GetSystemResources() interface{} {
	return sysinfo.GetSystemResources()
}

// ============================================================
// NETWORK INFO
// ============================================================

func (a *App) GetNetworkInfo() map[string]interface{} {
	result := map[string]interface{}{
		"public_ip": "",
		"isp":       "",
		"asn":       "",
		"city":      "",
		"country":   "",
		"org":       "",
		"local_ips": []string{},
	}

	checker := blockcheck.NewChecker(8, "")
	info := checker.GetProviderInfo()
	result["public_ip"] = info.IP
	result["isp"] = info.ISP
	result["asn"] = info.ASN
	result["city"] = info.City
	result["country"] = info.Country
	result["org"] = info.Org

	ifaces, err := net.InterfaceAddrs()
	if err == nil {
		var ips []string
		for _, addr := range ifaces {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				ips = append(ips, ipnet.IP.String())
			}
		}
		result["local_ips"] = ips
	}

	return result
}

// ============================================================
// RESOURCE CHECKER (Block Checker)
// ============================================================

func (a *App) CheckResource(rawURL string) map[string]interface{} {
	if rawURL == "" {
		return map[string]interface{}{"error": "URL required"}
	}

	var proxyAddr string
	if a.proxy.IsRunning() {
		pcfg := a.cfg.GetProxyConfig()
		proxyAddr = fmt.Sprintf("127.0.0.1:%d", pcfg.Port)
	}

	checker := blockcheck.NewChecker(5, proxyAddr)

	provider := checker.GetProviderInfo()
	direct := checker.Check(rawURL)

	var bypassResult *blockcheck.CheckResult
	if proxyAddr != "" {
		bypassResult = checker.CheckViaProxy(rawURL)
	}

	blocked := direct.Verdict != blockcheck.VerdictOK
	bypassWorks := false
	if bypassResult != nil {
		bypassWorks = bypassResult.Verdict == blockcheck.VerdictOK
	}

	inList := a.isHostInUserList(direct.Host)

	report := blockcheck.FullReport{
		URL:         rawURL,
		Host:        direct.Host,
		Direct:      direct,
		WithBypass:  bypassResult,
		Provider:    provider,
		Blocked:     blocked,
		BlockType:   direct.Verdict,
		BypassWorks: bypassWorks,
		InUserList:  inList,
		CheckedAt:   time.Now().Format("2006-01-02 15:04:05"),
	}

	return map[string]interface{}{
		"report": report,
	}
}

func (a *App) AddHostToUserList(host string) map[string]interface{} {
	if host == "" {
		return map[string]interface{}{"error": "host required"}
	}

	listPath := filepath.Join(a.cfg.ListsDir(), "list-general-user.txt")
	data, _ := os.ReadFile(listPath)
	lines := strings.Split(string(data), "\n")

	for _, l := range lines {
		if strings.TrimSpace(l) == host {
			return map[string]interface{}{"status": "already_exists"}
		}
	}

	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") && content != "" {
		content += "\n"
	}
	content += host + "\n"

	if err := os.WriteFile(listPath, []byte(content), 0644); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"status": "ok"}
}

func (a *App) isHostInUserList(host string) bool {
	listPath := filepath.Join(a.cfg.ListsDir(), "list-general-user.txt")
	data, err := os.ReadFile(listPath)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == host {
			return true
		}
	}
	return false
}

// ============================================================
// FIRST RUN / ZAPRET MANAGEMENT
// ============================================================

func (a *App) HasLocalZapret() bool {
	winws := filepath.Join(a.cfg.GetZapretPath(), "bin", "winws.exe")
	_, err := os.Stat(winws)
	return err == nil
}

func (a *App) HasSystemZapretService() bool {
	cmd := executil.HiddenCmd("sc", "query", "zapret")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "zapret")
}

func (a *App) RemoveSystemZapretService() map[string]interface{} {
	a.log.Info("zapret", "Removing system zapret service...")

	executil.HiddenCmd("net", "stop", "zapret").Run()
	executil.HiddenCmd("taskkill", "/IM", "winws.exe", "/F").Run()
	time.Sleep(1 * time.Second)

	executil.HiddenCmd("sc", "delete", "zapret").Run()
	executil.HiddenCmd("sc", "delete", "WinDivert").Run()
	executil.HiddenCmd("sc", "delete", "WinDivert14").Run()

	a.log.Info("zapret", "System zapret service removed")
	return map[string]interface{}{"status": "ok"}
}

func (a *App) RunWizard() map[string]interface{} {
	exePath, err := os.Executable()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	exeDir := filepath.Dir(exePath)
	wizardPath := filepath.Join(exeDir, "wizard.exe")

	if _, err := os.Stat(wizardPath); err != nil {
		return map[string]interface{}{"error": "wizard.exe не найден"}
	}

	cmd := executil.HiddenCmd(wizardPath)
	if err := cmd.Start(); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	a.log.Info("app", "Wizard started (PID: "+strconv.Itoa(cmd.Process.Pid)+")")
	return map[string]interface{}{"status": "ok"}
}

func (a *App) CheckWizardDone() bool {
	return a.HasLocalZapret()
}

// ============================================================
// MODS SYSTEM
// ============================================================

// ============================================================
// HEALTH CHECK
// ============================================================

type SystemHealth struct {
	Overall    string                   `json:"overall"`
	Components []ComponentHealth        `json:"components"`
	Satellites map[string]string       `json:"satellites"`
	Warnings   []string                 `json:"warnings"`
	Timestamp  string                   `json:"timestamp"`
}

type ComponentHealth struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Running bool   `json:"running"`
	Detail  string `json:"detail,omitempty"`
	Version string `json:"version,omitempty"`
}

func (a *App) HealthCheck() map[string]interface{} {
	warnings := []string{}
	components := []ComponentHealth{}

	zStatus := string(a.zapret.GetStatus())
	zHealth := ComponentHealth{
		Name:    "Запрет",
		Status:  zStatus,
		Running: zStatus == "running",
		Version: a.zapret.GetVersion(),
	}
	if !a.HasLocalZapret() {
		zHealth.Status = "missing"
		zHealth.Detail = "Локальный Запрет не найден"
		warnings = append(warnings, "Запрет: не установлен локально")
	}
	components = append(components, zHealth)

	pRunning := a.proxy.IsRunning()
	pHealth := ComponentHealth{
		Name:    "Proxy",
		Status:  "stopped",
		Running: pRunning,
	}
	if pRunning {
		pHealth.Status = "running"
	} else {
		pHealth.Detail = "Остановлен"
	}
	components = append(components, pHealth)

	xdRunning := a.xboxDns.IsEnabled()
	xdHealth := ComponentHealth{
		Name:    "Xbox DNS",
		Status:  "stopped",
		Running: xdRunning,
	}
	if xdRunning {
		xdHealth.Status = "running"
	}
	components = append(components, xdHealth)

	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	satellites := map[string]string{}
	for _, name := range []string{"wizard", "autoselect", "selfupdate", "zapretupdate"} {
		exeFile := filepath.Join(exeDir, name+".exe")
		if _, err := os.Stat(exeFile); err != nil {
			satellites[name] = "missing"
			warnings = append(warnings, name+".exe: файл не найден")
		} else {
			satellites[name] = "ok"
		}
	}

	overall := "healthy"
	if len(warnings) > 0 {
		hasBroken := false
		for _, c := range components {
			if c.Status == "missing" || c.Status == "broken" {
				hasBroken = true
				break
			}
		}
		for _, s := range satellites {
			if s == "missing" {
				hasBroken = true
				break
			}
		}
		if hasBroken {
			overall = "critical"
		} else {
			overall = "degraded"
		}
	}

	return map[string]interface{}{
		"overall":    overall,
		"components": components,
		"satellites": satellites,
		"warnings":   warnings,
		"timestamp":  time.Now().Format("15:04:05"),
	}
}

// ============================================================
// BACKUP & RESTORE
// ============================================================

func (a *App) GetBackups(component string) []updater.BackupEntry {
	bm := updater.NewBackupManager(a.exeDir)
	return bm.ListBackups(component)
}

func (a *App) RestoreFromBackup(backupName string) map[string]interface{} {
	bm := updater.NewBackupManager(a.exeDir)
	if err := bm.RestoreBackup(backupName); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"status": "ok"}
}

func (a *App) GetIgnoredVersions() []updater.IgnoredVersion {
	bm := updater.NewBackupManager(a.exeDir)
	return bm.ListIgnoredVersions()
}

func (a *App) AddIgnoredVersion(component, version, reason string) map[string]interface{} {
	bm := updater.NewBackupManager(a.exeDir)
	if err := bm.AddIgnoredVersion(component, version, reason); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"status": "ok"}
}

func (a *App) RemoveIgnoredVersion(component, version string) map[string]interface{} {
	bm := updater.NewBackupManager(a.exeDir)
	if err := bm.RemoveIgnoredVersion(component, version); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"status": "ok"}
}

func (a *App) AutoInstallZapret() map[string]interface{} {
	a.saveBackupToDB()
	err := a.zapret.DownloadAndInstall(func(downloaded, total int64) {
		runtime.EventsEmit(a.ctx, "download:progress", map[string]interface{}{
			"downloaded": downloaded,
			"total":      total,
			"percent":    percentOrZero(downloaded, total),
		})
	})
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	strategy := a.zapret.GetCurrentStrategy()
	if strategy == "" {
		strategies := a.zapret.ListStrategies()
		if len(strategies) > 0 {
			strategy = strategies[0].Filename
			a.cfg.SetCurrentStrategy(strategy)
			a.cfg.Save()
		}
	}

	var startErr string
	if strategy != "" {
		if err := a.zapret.SetStrategy(strategy); err != nil {
			startErr = err.Error()
		}
	} else {
		if err := a.zapret.Start(); err != nil {
			startErr = err.Error()
		}
	}

	return map[string]interface{}{
		"status":      "ok",
		"version":     a.zapret.GetVersion(),
		"strategy":    a.zapret.GetCurrentStrategy(),
		"start_error": startErr,
	}
}

func percentOrZero(done, total int64) int {
	if total == 0 {
		return 0
	}
	p := int(done * 100 / total)
	if p > 100 {
		return 100
	}
	return p
}

