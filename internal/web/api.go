package web

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"zpui/internal/monitor"
	"zpui/internal/zapret"
)

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func readJSON(r *http.Request) (map[string]interface{}, error) {
	var m map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	pcfg := s.cfg.GetProxyConfig()
	traffic := s.monitor.GetCurrentStats()

	ips := getLocalIPs()
	if len(ips) == 0 {
		ips = []string{"127.0.0.1"}
	}

	zapretStatus := string(s.zapret.GetStatus())
	strategy := s.zapret.GetCurrentStrategy()

	if zapretStatus == "running" {
		svcStatus := s.zapret.GetServiceStatus()
		if svcStatus.Strategy != "" {
			strategy = svcStatus.Strategy
		}
	}

	status := map[string]interface{}{
		"zapret": map[string]interface{}{
			"status":    zapretStatus,
			"version":   s.zapret.GetVersion(),
			"strategy":  strategy,
			"zapretDir": s.cfg.GetZapretPath(),
		},
		"proxy": map[string]interface{}{
			"running": s.proxy.IsRunning(),
			"port":    pcfg.Port,
			"stats":   s.proxy.GetStats(),
		},
		"monitor": map[string]interface{}{
			"download_bytes":  traffic.DownloadBytes,
			"upload_bytes":    traffic.UploadBytes,
			"download_speed":  traffic.DownloadSpeed,
			"upload_speed":    traffic.UploadSpeed,
			"download_fmt":    monitor.FormatBytes(traffic.DownloadBytes),
			"upload_fmt":      monitor.FormatBytes(traffic.UploadBytes),
			"dl_speed_fmt":    monitor.FormatSpeed(traffic.DownloadSpeed),
			"ul_speed_fmt":    monitor.FormatSpeed(traffic.UploadSpeed),
		},
		"mod": map[string]interface{}{
			"version":        s.version,
			"autostart":      s.cfg.AutoStart,
			"web_port":       s.cfg.Web.Port,
			"zapret_repo":    s.cfg.ZapretRepoURL,
			"mod_repo":       s.cfg.ModRepoURL,
		},
		"network": map[string]interface{}{
			"hostname": getHostname(),
			"mac":      getMACAddress(),
			"ips":      ips,
		},
	}
	writeJSON(w, status)
}

func (s *Server) handleZapretStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	if err := s.zapret.Start(); err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"status": "started"})
}

func (s *Server) handleZapretStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	s.zapret.Stop()
	writeJSON(w, map[string]interface{}{"status": "stopped"})
}

func (s *Server) handleZapretRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	if err := s.zapret.Restart(); err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"status": "restarted"})
}

func (s *Server) handleStrategies(w http.ResponseWriter, r *http.Request) {
	strategies := s.zapret.ListStrategies()
	writeJSON(w, map[string]interface{}{"strategies": strategies})
}

func (s *Server) handleSetStrategy(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	data, _ := readJSON(r)
	filename, _ := data["filename"].(string)
	if filename == "" {
		writeJSON(w, map[string]interface{}{"error": "filename required"})
		return
	}
	if err := s.zapret.SetStrategy(filename); err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"status": "ok", "strategy": filename})
}

func (s *Server) handleServiceInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	data, _ := readJSON(r)
	strategy, _ := data["strategy"].(string)
	if strategy == "" {
		strategy = s.zapret.GetCurrentStrategy()
	}
	if err := s.zapret.InstallService(strategy); err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"status": "installed"})
}

func (s *Server) handleServiceRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	s.zapret.RemoveService()
	writeJSON(w, map[string]interface{}{"status": "removed"})
}

func (s *Server) handleServiceStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.zapret.GetServiceStatus())
}

func (s *Server) handleGameFilter(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		data, _ := readJSON(r)
		mode, _ := data["mode"].(string)
		if err := s.zapret.SetGameFilter(mode); err != nil {
			writeJSON(w, map[string]interface{}{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]interface{}{"status": "ok"})
		return
	}
	mode, _, _ := s.zapret.LoadGameFilter()
	writeJSON(w, map[string]interface{}{"mode": mode})
}

func (s *Server) handleProxyStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	if err := s.proxy.Start(); err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"status": "started"})
}

func (s *Server) handleProxyStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	s.proxy.Stop()
	writeJSON(w, map[string]interface{}{"status": "stopped"})
}

func (s *Server) handleProxyStatus(w http.ResponseWriter, r *http.Request) {
	pcfg := s.cfg.GetProxyConfig()
	writeJSON(w, map[string]interface{}{
		"running":  s.proxy.IsRunning(),
		"port":     pcfg.Port,
		"username": pcfg.Username,
		"auto_start": pcfg.AutoStart,
		"stats":    s.proxy.GetStats(),
	})
}

func (s *Server) handleProxyConnections(w http.ResponseWriter, r *http.Request) {
	conns := s.proxy.GetConnections()
	byClient := s.proxy.GetActiveConnectionsByClient()
	arpTable := getARPTable()

	deviceInfo := make(map[string]map[string]string)
	for ip := range byClient {
		deviceInfo[ip] = map[string]string{
			"hostname": resolveHostname(ip),
			"mac":      arpTable[ip],
		}
	}

	writeJSON(w, map[string]interface{}{
		"connections": conns,
		"by_client":   byClient,
		"device_info": deviceInfo,
	})
}

func (s *Server) handleProxyConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		s.log.Info("proxy", "POST /api/proxy/config received")
		data, _ := readJSON(r)
		pcfg := s.cfg.GetProxyConfig()
		if port, ok := data["port"].(float64); ok {
			pcfg.Port = int(port)
		}
		if user, ok := data["username"].(string); ok {
			pcfg.Username = user
		}
		if pass, ok := data["password"].(string); ok {
			pcfg.Password = pass
		}
		if as, ok := data["auto_start"].(bool); ok {
			pcfg.AutoStart = as
		}
		s.cfg.SetProxyConfig(pcfg)
		if err := s.cfg.Save(); err != nil {
			s.log.Error("proxy", fmt.Sprintf("Save error: %v", err))
		} else {
			s.log.Info("proxy", "Proxy config saved to JSON")
		}
		writeJSON(w, map[string]interface{}{"status": "ok"})
		return
	}
	writeJSON(w, s.cfg.GetProxyConfig())
}

func (s *Server) handleProxyQRCode(w http.ResponseWriter, r *http.Request) {
	pcfg := s.cfg.GetProxyConfig()
	ips := getLocalIPs()
	if len(ips) == 0 {
		ips = []string{"127.0.0.1"}
	}
	writeJSON(w, map[string]interface{}{
		"ips":     ips,
		"port":    pcfg.Port,
		"username": pcfg.Username,
		"password": pcfg.Password,
	})
}

func (s *Server) handleTraffic(w http.ResponseWriter, r *http.Request) {
	stats := s.monitor.GetCurrentStats()
	writeJSON(w, map[string]interface{}{
		"download_bytes": stats.DownloadBytes,
		"upload_bytes":   stats.UploadBytes,
		"download_speed": stats.DownloadSpeed,
		"upload_speed":   stats.UploadSpeed,
		"download_fmt":   monitor.FormatBytes(stats.DownloadBytes),
		"upload_fmt":     monitor.FormatBytes(stats.UploadBytes),
		"dl_speed_fmt":   monitor.FormatSpeed(stats.DownloadSpeed),
		"ul_speed_fmt":   monitor.FormatSpeed(stats.UploadSpeed),
		"timestamp":      stats.Timestamp,
	})
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	byClient := s.proxy.GetActiveConnectionsByClient()
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
	writeJSON(w, map[string]interface{}{"devices": devices})
}

func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	info, err := s.zapret.CheckForUpdates()
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, info)
}

func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	progress := make(chan zapret.UpdateProgress, 20)
	go s.zapret.PerformUpdate(progress)
	writeJSON(w, map[string]interface{}{"status": "started"})
}

func (s *Server) handleUpdateStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	progress := make(chan zapret.UpdateProgress, 20)
	go s.zapret.PerformUpdate(progress)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", 500)
		return
	}

	for p := range progress {
		data, _ := json.Marshal(p)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
}

func (s *Server) handleAutoStrategy(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	if s.zapret.IsAutoTestRunning() {
		writeJSON(w, map[string]interface{}{"error": "Автотест уже запущен"})
		return
	}
	writeJSON(w, map[string]interface{}{"status": "started"})
}

func (s *Server) handleStrategyCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	s.zapret.CancelAutoTest()
	writeJSON(w, map[string]interface{}{"status": "cancelled"})
}

func (s *Server) handleStrategyStream(w http.ResponseWriter, r *http.Request) {
	if s.zapret.IsAutoTestRunning() {
		writeJSON(w, map[string]interface{}{"error": "Автотест уже запущен"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", 500)
		return
	}

	ctx := r.Context()
	results := make(chan zapret.AutoTestResult, 50)
	done := make(chan struct{})
	go s.zapret.RunAutoTest(ctx, results, done)

	for {
		select {
		case result, ok := <-results:
			if !ok {
				return
			}
			data, _ := json.Marshal(result)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-done:
			fmt.Fprintf(w, "data: {\"type\":\"done\"}\n\n")
			flusher.Flush()
			return
		case <-ctx.Done():
			s.zapret.CancelAutoTest()
			return
		}
	}
}

func (s *Server) handleAutostartStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"enabled": s.cfg.AutoStart,
	})
}

func (s *Server) handleAutostartEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	s.log.Info("autostart", "POST /api/autostart/enable received")
	s.cfg.AutoStart = true
	if err := s.cfg.Save(); err != nil {
		s.log.Error("autostart", fmt.Sprintf("Save error: %v", err))
	} else {
		s.log.Info("autostart", "Autostart config saved")
	}

	writeJSON(w, map[string]interface{}{"status": "enabled"})

	go func() {
		exePath := getExePath()
		s.log.Info("autostart", "Creating scheduled task...")
		exec.Command("schtasks", "/delete", "/tn", "ZPUI", "/f").Run()
		cmd := exec.Command("schtasks", "/create",
			"/tn", "ZPUI",
			"/tr", fmt.Sprintf(`"%s"`, exePath),
			"/sc", "onlogon",
			"/rl", "highest",
			"/f")
		output, err := cmd.CombinedOutput()
		if err != nil {
			s.log.Error("autostart", fmt.Sprintf("Task Scheduler error: %v: %s", err, string(output)))
			exec.Command("reg", "add",
				`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
				"/v", "ZPUI", "/t", "REG_SZ", "/d",
				fmt.Sprintf(`"%s"`, exePath), "/f").Run()
		} else {
			s.log.Info("autostart", "Scheduled task created")
		}
	}()
}

func (s *Server) handleAutostartDisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	s.log.Info("autostart", "POST /api/autostart/disable received")
	s.cfg.AutoStart = false
	if err := s.cfg.Save(); err != nil {
		s.log.Error("autostart", fmt.Sprintf("Save error: %v", err))
	} else {
		s.log.Info("autostart", "Autostart disabled, config saved")
	}

	writeJSON(w, map[string]interface{}{"status": "disabled"})

	go func() {
		exec.Command("schtasks", "/delete", "/tn", "ZPUI", "/f").Run()
		exec.Command("reg", "delete",
			`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
			"/v", "ZPUI", "/f").Run()
	}()
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	if category == "" {
		category = "zapret"
	}
	lines := 100
	if l := r.URL.Query().Get("lines"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			lines = n
		}
	}
	logLines := s.log.ReadRecent(category, lines)
	writeJSON(w, map[string]interface{}{"lines": logLines})
}

func (s *Server) handleLogFiles(w http.ResponseWriter, r *http.Request) {
	files := s.log.ListLogFiles()
	writeJSON(w, map[string]interface{}{"files": files})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		s.log.Info("config", "POST /api/config received")
		data, _ := readJSON(r)
		s.log.Info("config", fmt.Sprintf("Data: %v", data))
		if port, ok := data["web_port"].(float64); ok {
			s.cfg.Web.Port = int(port)
		}
		if path, ok := data["zapret_path"].(string); ok {
			s.cfg.SetZapretPath(path)
		}
		if err := s.cfg.Save(); err != nil {
			s.log.Error("config", fmt.Sprintf("Save error: %v", err))
		} else {
			s.log.Info("config", "Config saved to JSON")
		}
		writeJSON(w, map[string]interface{}{"status": "ok"})
		return
	}
	writeJSON(w, map[string]interface{}{
		"zapret_path":      s.cfg.GetZapretPath(),
		"current_strategy": s.cfg.GetCurrentStrategy(),
		"web_port":         s.cfg.Web.Port,
		"proxy":            s.cfg.GetProxyConfig(),
		"autostart":        s.cfg.AutoStart,
		"auto_update_check": s.cfg.AutoUpdateCheck,
		"logs":             s.cfg.Logs,
	})
}

func (s *Server) handleZapretInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	data, _ := readJSON(r)
	sourceDir, _ := data["source_dir"].(string)
	if sourceDir == "" {
		writeJSON(w, map[string]interface{}{"error": "source_dir required"})
		return
	}
	progress := make(chan zapret.UpdateProgress, 20)
	go s.zapret.InstallZapret(sourceDir, progress)
	writeJSON(w, map[string]interface{}{"status": "started"})
}

func (s *Server) handleUpInfo(w http.ResponseWriter, r *http.Request) {
	ips := getLocalIPs()
	if len(ips) == 0 {
		ips = []string{"127.0.0.1"}
	}
	traffic := s.monitor.GetCurrentStats()
	pcfg := s.cfg.GetProxyConfig()

	writeJSON(w, map[string]interface{}{
		"hostname":    getHostname(),
		"ip":          ips,
		"mac":         getMACAddress(),
		"zapret": map[string]interface{}{
			"status":   string(s.zapret.GetStatus()),
			"version":  s.zapret.GetVersion(),
			"strategy": s.zapret.GetCurrentStrategy(),
		},
		"proxy": map[string]interface{}{
			"running": s.proxy.IsRunning(),
			"port":    pcfg.Port,
		},
		"traffic": map[string]interface{}{
			"download":    monitor.FormatBytes(traffic.DownloadBytes),
			"upload":      monitor.FormatBytes(traffic.UploadBytes),
			"dl_speed":    monitor.FormatSpeed(traffic.DownloadSpeed),
			"ul_speed":    monitor.FormatSpeed(traffic.UploadSpeed),
		},
		"mod_version": s.version,
		"timestamp":   time.Now().Format("2006-01-02 15:04:05"),
	})
}

func getExePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	results := map[string]interface{}{}

	results["bfe_service"] = checkService("BFE", "Base Filtering Engine")
	results["zapret_service"] = checkService("zapret", "Служба Zapret")
	results["windivert"] = checkWinDivert(s.cfg.GetZapretPath())
	results["winws_process"] = checkProcess("winws.exe", "winws.exe (Zapret)")
	results["tcp_timestamps"] = checkTCPTimestamps()
	results["firewall"] = checkFirewallRule()
	results["system_proxy"] = checkSystemProxy()
	results["conflicting"] = checkConflictingServices()
	results["killer"] = checkServiceList("Killer", "Killer Network Service")
	results["intel"] = checkIntelConnectivity()
	results["checkpoint"] = checkCheckPoint()
	results["smartbyte"] = checkServiceList("SmartByte", "SmartByte")
	results["adguard"] = checkProcess("AdguardSvc.exe", "Adguard")
	results["vpn"] = checkVPN()
	results["dns"] = checkDNS()
	results["hosts_file"] = checkHostsFile()
	results["proxy"] = checkProxy(s)

	writeJSON(w, results)
}

type diagResult struct {
	Status string `json:"status"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
}

func checkService(name, label string) diagResult {
	cmd := exec.Command("sc", "query", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return diagResult{Status: "warn", Label: label, Detail: "Не удалось проверить"}
	}
	out := string(output)
	if strings.Contains(out, "RUNNING") {
		return diagResult{Status: "ok", Label: label, Detail: "Работает"}
	}
	if strings.Contains(out, "STOPPED") {
		return diagResult{Status: "warn", Label: label, Detail: "Остановлен"}
	}
	if strings.Contains(out, "does not exist") {
		return diagResult{Status: "warn", Label: label, Detail: "Не найден"}
	}
	return diagResult{Status: "ok", Label: label, Detail: "Присутствует"}
}

func checkWinDivert(zapretDir string) diagResult {
	if zapretDir == "" {
		return diagResult{Status: "warn", Label: "WinDivert", Detail: "Путь не задан"}
	}
	paths := []string{
		zapretDir + "\\WinDivert.dll",
		zapretDir + "\\bin\\WinDivert.dll",
		zapretDir + "\\WinDivert\\WinDivert.dll",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return diagResult{Status: "ok", Label: "WinDivert", Detail: "Найден"}
		}
	}
	return diagResult{Status: "error", Label: "WinDivert", Detail: "Не найден"}
}

func checkTCPTimestamps() diagResult {
	cmd := exec.Command("netsh", "interface", "tcp", "show", "global")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return diagResult{Status: "warn", Label: "TCP Timestamps", Detail: "Не удалось проверить"}
	}
	out := string(output)
	if strings.Contains(out, "disabled") || strings.Contains(out, "Отключено") {
		return diagResult{Status: "ok", Label: "TCP Timestamps", Detail: "Отключены (OK)"}
	}
	return diagResult{Status: "warn", Label: "TCP Timestamps", Detail: "Включены — могут мешать"}
}

func checkFirewallRule() diagResult {
	cmd := exec.Command("netsh", "advfirewall", "firewall", "show", "rule", "name=ZPUI_SOCKS5")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return diagResult{Status: "warn", Label: "Firewall (SOCKS5)", Detail: "Правило не найдено"}
	}
	out := string(output)
	if strings.Contains(out, "ZPUI_SOCKS5") {
		return diagResult{Status: "ok", Label: "Firewall (SOCKS5)", Detail: "Правило активно"}
	}
	return diagResult{Status: "warn", Label: "Firewall (SOCKS5)", Detail: "Правило не найдено"}
}

func checkConflictingServices() diagResult {
	conflicts := []string{}
	services := map[string]string{
		"AdguardService":    "Adguard",
		"SmartByte":         "SmartByte",
		"SAB":               "McAfee",
		"ekrn":              "ESET",
		"ksvlasst":          "Kaspersky",
	}
	for svc, name := range services {
		cmd := exec.Command("sc", "query", svc)
		output, _ := cmd.CombinedOutput()
		if strings.Contains(string(output), "RUNNING") {
			conflicts = append(conflicts, name)
		}
	}
	if len(conflicts) > 0 {
		return diagResult{Status: "warn", Label: "Конфликты", Detail: strings.Join(conflicts, ", ")}
	}
	return diagResult{Status: "ok", Label: "Конфликты", Detail: "Не обнаружены"}
}

func checkDNS() diagResult {
	cmd := exec.Command("powershell", "-Command", "Get-DnsClientServerAddress -AddressFamily IPv4 | Select-Object -First 10")
	output, _ := cmd.CombinedOutput()
	out := string(output)
	if strings.Contains(out, "127.0.0.1") || strings.Contains(out, "localhost") {
		return diagResult{Status: "warn", Label: "DNS", Detail: "Обнаружен локальный DNS"}
	}
	return diagResult{Status: "ok", Label: "DNS", Detail: "Стандартный"}
}

func checkProxy(s *Server) diagResult {
	if s.proxy.IsRunning() {
		return diagResult{Status: "ok", Label: "SOCKS5 Прокси", Detail: "Работает"}
	}
	return diagResult{Status: "warn", Label: "SOCKS5 Прокси", Detail: "Остановлен"}
}

func checkProcess(name, label string) diagResult {
	cmd := exec.Command("tasklist", "/FI", "IMAGENAME eq "+name)
	output, _ := cmd.CombinedOutput()
	if strings.Contains(string(output), name) {
		return diagResult{Status: "ok", Label: label, Detail: "Запущен"}
	}
	return diagResult{Status: "warn", Label: label, Detail: "Не запущен"}
}

func checkSystemProxy() diagResult {
	cmd := exec.Command("reg", "query", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyEnable")
	output, _ := cmd.CombinedOutput()
	out := string(output)
	if strings.Contains(out, "0x1") {
		cmd2 := exec.Command("reg", "query", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyServer")
		output2, _ := cmd2.CombinedOutput()
		proxy := strings.TrimSpace(strings.ReplaceAll(string(output2), "ProxyServer", ""))
		proxy = strings.TrimSpace(strings.ReplaceAll(proxy, "REG_SZ", ""))
		return diagResult{Status: "warn", Label: "Системный прокси", Detail: "Включен: " + proxy}
	}
	return diagResult{Status: "ok", Label: "Системный прокси", Detail: "Отключен"}
}

func checkServiceList(keyword, label string) diagResult {
	cmd := exec.Command("sc", "query", "state=", "all")
	output, _ := cmd.CombinedOutput()
	if strings.Contains(string(output), keyword) {
		return diagResult{Status: "warn", Label: label, Detail: "Обнаружен — может конфликтовать"}
	}
	return diagResult{Status: "ok", Label: label, Detail: "Не обнаружен"}
}

func checkIntelConnectivity() diagResult {
	cmd := exec.Command("sc", "query", "state=", "all")
	output, _ := cmd.CombinedOutput()
	out := string(output)
	if strings.Contains(out, "Intel") && strings.Contains(out, "Connectivity") && strings.Contains(out, "Network") {
		return diagResult{Status: "warn", Label: "Intel Connectivity", Detail: "Обнаружен — конфликтует с Zapret"}
	}
	return diagResult{Status: "ok", Label: "Intel Connectivity", Detail: "Не обнаружен"}
}

func checkCheckPoint() diagResult {
	cmd := exec.Command("sc", "query", "state=", "all")
	output, _ := cmd.CombinedOutput()
	out := string(output)
	if strings.Contains(out, "TracSrvWrapper") || strings.Contains(out, "EPWD") {
		return diagResult{Status: "warn", Label: "Check Point", Detail: "Обнаружен — конфликтует с Zapret"}
	}
	return diagResult{Status: "ok", Label: "Check Point", Detail: "Не обнаружен"}
}

func checkVPN() diagResult {
	cmd := exec.Command("sc", "query", "state=", "all")
	output, _ := cmd.CombinedOutput()
	out := string(output)
	var found []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(strings.ToUpper(line), "VPN") && strings.Contains(line, "SERVICE_NAME") {
			name := strings.TrimPrefix(line, "SERVICE_NAME: ")
			found = append(found, name)
		}
	}
	if len(found) > 0 {
		return diagResult{Status: "warn", Label: "VPN сервисы", Detail: strings.Join(found, ", ")}
	}
	return diagResult{Status: "ok", Label: "VPN сервисы", Detail: "Не обнаружены"}
}

func checkHostsFile() diagResult {
	hostsPath := os.Getenv("SystemRoot") + `\System32\drivers\etc\hosts`
	data, err := os.ReadFile(hostsPath)
	if err != nil {
		return diagResult{Status: "warn", Label: "Hosts файл", Detail: "Не удалось прочитать"}
	}
	content := strings.ToLower(string(data))
	blocked := []string{}
	for _, domain := range []string{"youtube.com", "youtu.be", "discord.com", "google.com"} {
		if strings.Contains(content, domain) && !strings.HasPrefix(strings.TrimSpace(content), "#") {
			blocked = append(blocked, domain)
		}
	}
	if len(blocked) > 0 {
		return diagResult{Status: "warn", Label: "Hosts файл", Detail: "Найдены записи: " + strings.Join(blocked, ", ")}
	}
	return diagResult{Status: "ok", Label: "Hosts файл", Detail: "Чистый"}
}

func (s *Server) handleCacheClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	data, _ := readJSON(r)
	target, _ := data["target"].(string)

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
		cmd := exec.Command("ipconfig", "/flushdns")
		cmd.Run()
		cmd2 := exec.Command("netsh", "winsock", "reset")
		cmd2.Run()
		cmd3 := exec.Command("netsh", "int", "ip", "reset")
		cmd3.Run()
		cleared = append(cleared, "DNS cache", "Winsock", "IP stack")
	}

	if len(cleared) > 0 {
		writeJSON(w, map[string]interface{}{"status": "ok", "cleared": cleared})
	} else {
		writeJSON(w, map[string]interface{}{"status": "nothing", "cleared": []string{}})
	}
}

func (s *Server) handleLists(w http.ResponseWriter, r *http.Request) {
	listsDir := s.cfg.ListsDir()
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
		Name    string   `json:"name"`
		Lines   []string `json:"lines"`
		Count   int      `json:"count"`
		Editable bool    `json:"editable"`
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
	writeJSON(w, map[string]interface{}{"lists": result})
}

func (s *Server) handleListsSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	data, _ := readJSON(r)
	name, _ := data["name"].(string)
	content, _ := data["content"].(string)
	if name == "" {
		writeJSON(w, map[string]interface{}{"error": "name required"})
		return
	}
	if !strings.HasSuffix(name, "-user.txt") {
		writeJSON(w, map[string]interface{}{"error": "only user lists are editable"})
		return
	}
	listsDir := s.cfg.ListsDir()
	path := filepath.Join(listsDir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"status": "ok"})
}

type resCacheData struct {
	Default []map[string]interface{}
	User    []map[string]interface{}
}

var (
	resourceCache     *resCacheData
	resourceCacheTime time.Time
	resourceCacheMu   sync.Mutex
)

func (s *Server) handleResourceStatus(w http.ResponseWriter, r *http.Request) {
	resourceCacheMu.Lock()
	if time.Since(resourceCacheTime) < 30*time.Second && resourceCache != nil {
		resourceCacheMu.Unlock()
		writeJSON(w, map[string]interface{}{
			"default": resourceCache.Default,
			"user":    resourceCache.User,
		})
		return
	}
	resourceCacheMu.Unlock()

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

	defaultResources := []check{}
	targetsPath := filepath.Join(s.cfg.GetZapretPath(), "utils", "targets.txt")
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

	userHosts := []string{}
	if body, err := os.ReadFile(filepath.Join(s.cfg.ListsDir(), "list-general-user.txt")); err == nil {
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

	resourceCacheMu.Lock()
	resourceCache = &resCacheData{Default: defaultResults, User: userResults}
	resourceCacheTime = time.Now()
	resourceCacheMu.Unlock()

	writeJSON(w, map[string]interface{}{
		"default": defaultResults,
		"user":    userResults,
	})
}

func (s *Server) handleIpsetStatus(w http.ResponseWriter, r *http.Request) {
	listFile := filepath.Join(s.cfg.ListsDir(), "ipset-all.txt")
	data, err := os.ReadFile(listFile)
	if err != nil {
		writeJSON(w, map[string]interface{}{"status": "any"})
		return
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
	writeJSON(w, map[string]interface{}{"status": status})
}

func (s *Server) handleIpsetToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	data, _ := readJSON(r)
	mode, _ := data["mode"].(string)
	listFile := filepath.Join(s.cfg.ListsDir(), "ipset-all.txt")
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
	writeJSON(w, map[string]interface{}{"status": "ok"})
}

func (s *Server) handleAutoUpdateStatus(w http.ResponseWriter, r *http.Request) {
	flagFile := filepath.Join(s.cfg.GetZapretPath(), "utils", "check_updates.enabled")
	_, err := os.Stat(flagFile)
	writeJSON(w, map[string]interface{}{"enabled": err == nil})
}

func (s *Server) handleAutoUpdateToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	data, _ := readJSON(r)
	enabled, _ := data["enabled"].(bool)
	flagFile := filepath.Join(s.cfg.GetZapretPath(), "utils", "check_updates.enabled")
	utilsDir := filepath.Join(s.cfg.GetZapretPath(), "utils")
	os.MkdirAll(utilsDir, 0755)
	if enabled {
		os.WriteFile(flagFile, []byte("ENABLED"), 0644)
	} else {
		os.Remove(flagFile)
	}
	writeJSON(w, map[string]interface{}{"status": "ok"})
}

func (s *Server) handleUpdateIpset(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	listFile := filepath.Join(s.cfg.ListsDir(), "ipset-all.txt")
	url := "https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/refs/heads/main/.service/ipset-service.txt"
	downloadAndSave(url, listFile, w)
}

func (s *Server) handleUpdateHosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	url := "https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/refs/heads/main/.service/hosts"
	tmpFile := filepath.Join(os.TempDir(), "zapret_hosts.txt")
	downloadAndSave(url, tmpFile, w)
}

func downloadAndSave(url, destPath string, w http.ResponseWriter) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		writeJSON(w, map[string]interface{}{"error": fmt.Sprintf("HTTP %d", resp.StatusCode)})
		return
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	os.MkdirAll(filepath.Dir(destPath), 0755)
	if err := os.WriteFile(destPath, body, 0644); err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"status": "ok"})
}

func mustReadFile(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return []byte("203.0.113.113/32\n")
	}
	return data
}

func (s *Server) handleOpenExternal(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	data, _ := readJSON(r)
	url, _ := data["url"].(string)
	if url == "" {
		writeJSON(w, map[string]interface{}{"error": "url required"})
		return
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		writeJSON(w, map[string]interface{}{"error": "only http/https URLs allowed"})
		return
	}
	cmd := exec.Command("cmd", "/c", "start", "", url)
	cmd.Start()
	writeJSON(w, map[string]interface{}{"status": "ok"})
}
