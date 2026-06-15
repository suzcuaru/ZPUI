package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"zpui/internal/executil"
	"zpui/internal/monitor"
	"zpui/internal/zapret"
)

// ============================================================
// STATUS
// ============================================================

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
			"version":      a.version,
			"autostart":    a.cfg.AutoStart,
			"web_port":     a.cfg.Web.Port,
			"zapret_repo":  a.cfg.ZapretRepoURL,
			"mod_repo":     a.cfg.ModRepoURL,
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

// ============================================================
// CONFIG
// ============================================================

func (a *App) GetConfig() map[string]interface{} {
	return map[string]interface{}{
		"zapret_path":       a.cfg.GetZapretPath(),
		"current_strategy":  a.cfg.GetCurrentStrategy(),
		"web_port":          a.cfg.Web.Port,
		"proxy":             a.cfg.GetProxyConfig(),
		"autostart":         a.cfg.AutoStart,
		"auto_update_check": a.cfg.AutoUpdateCheck,
		"logs":              a.cfg.Logs,
	}
}

func (a *App) SetConfig(opts map[string]interface{}) map[string]interface{} {
	a.log.Info("config", "SetConfig called")
	if port, ok := opts["web_port"].(float64); ok {
		a.cfg.Web.Port = int(port)
	}
	if path, ok := opts["zapret_path"].(string); ok {
		a.cfg.SetZapretPath(path)
	}
	if err := a.cfg.Save(); err != nil {
		a.log.Error("config", "Save error: "+err.Error())
	} else {
		a.log.Info("config", "Config saved to JSON")
	}
	return map[string]interface{}{"status": "ok"}
}

// ============================================================
// ZAPRET INSTALL
// ============================================================

func (a *App) InstallZapret(sourceDir string) map[string]interface{} {
	if sourceDir == "" {
		return map[string]interface{}{"error": "source_dir required"}
	}
	progress := make(chan zapret.UpdateProgress, 20)
	go a.zapret.InstallZapret(sourceDir, progress)
	return map[string]interface{}{"status": "started"}
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
