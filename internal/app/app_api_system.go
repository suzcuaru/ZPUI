package app

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"zpui/internal/blockcheck"
	"zpui/internal/database"
	"zpui/internal/executil"
	"zpui/internal/monitor"
	"zpui/internal/sysinfo"
	"zpui/internal/updater"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

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
// AVAILABILITY HISTORY
// ============================================================

func (a *App) GetAvailabilityHistory(hours int, typ string) map[string]interface{} {
	if hours <= 0 {
		hours = 24
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	all, err := database.GetAvailabilityHistory(since)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	if all == nil {
		all = []database.AvailabilityRecord{}
	}
	// Фильтр по типу
	var records []database.AvailabilityRecord
	for _, r := range all {
		if typ == "" || typ == "all" || r.Type == typ {
			records = append(records, r)
		}
	}
	return map[string]interface{}{
		"records": records,
		"count":   len(records),
	}
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

func (a *App) OpenExternal(rawURL string) map[string]interface{} {
	if rawURL == "" {
		return map[string]interface{}{"error": "url required"}
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return map[string]interface{}{"error": "only http/https URLs allowed"}
	}
	executil.HiddenCmd("rundll32", "url.dll,FileProtocolHandler", rawURL).Start()
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
// HEALTH CHECK
// ============================================================

type SystemHealth struct {
	Overall    string            `json:"overall"`
	Components []ComponentHealth `json:"components"`
	Modules    map[string]string `json:"modules"`
	Warnings   []string          `json:"warnings"`
	Timestamp  string            `json:"timestamp"`
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
	modules := map[string]string{}
	for _, name := range []string{"wizard", "autoselect", "selfupdate", "zapretupdate"} {
		exeFile := filepath.Join(exeDir, name+".exe")
		if _, err := os.Stat(exeFile); err != nil {
			modules[name] = "missing"
			warnings = append(warnings, name+".exe: файл не найден")
		} else {
			modules[name] = "ok"
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
		for _, s := range modules {
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
		"modules":    modules,
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
