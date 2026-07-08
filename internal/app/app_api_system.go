package app

import (
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

// GetResourceStatus returns resource status with 30s cache.
// Use RefreshResourceStatus for manual checks (refresh button).
func (a *App) GetResourceStatus() map[string]interface{} {
	return a.getResourceStatusInternal(false)
}

// RefreshResourceStatus forces resource check ignoring cache.
// Used by the UI refresh button so user sees fresh results.
func (a *App) RefreshResourceStatus() map[string]interface{} {
	return a.getResourceStatusInternal(true)
}

func (a *App) getResourceStatusInternal(force bool) map[string]interface{} {
	if !force {
		a.resourceCacheMu.Lock()
		if time.Since(a.resourceCacheTime) < 30*time.Second && a.resourceCache != nil {
			cached := a.resourceCache
			cachedAt := a.resourceCacheTime
			a.resourceCacheMu.Unlock()
			return a.buildResourceStatusResponse(cached, cachedAt, true)
		}
		a.resourceCacheMu.Unlock()
	}

	defaultTargets, _ := blockcheck.ReadTargets(blockcheck.DefaultTargetsPath(a.cfg.GetZapretPath()))

	var userTargets []blockcheck.BulkTarget
	if body, err := os.ReadFile(filepath.Join(a.cfg.ListsDir(), "list-general-user.txt")); err == nil {
		userTargets = blockcheck.ParseTargets(string(body))
	}

	// Filter out resources in the skip list (always-down / no-point-checking).
	defaultTargets = a.filterSkipped(defaultTargets)
	userTargets = a.filterSkipped(userTargets)

	bc := a.cfg.GetBlockCheckConfig()
	checker := blockcheck.NewChecker(bc.CheckTCP, bc.CheckTLS, bc.CheckHTTP, bc.TimeoutSec)
	report := checker.BulkCheck(defaultTargets, userTargets)

	now := time.Now()
	a.resourceCacheMu.Lock()
	a.resourceCache = report
	a.resourceCacheTime = now
	a.resourceCacheMu.Unlock()

	return a.buildResourceStatusResponse(report, now, false)
}

// buildResourceStatusResponse assembles response with check metadata:
//   checked_at      - ISO time of last check
//   checked_at_unix - unix timestamp (for "N sec ago")
//   cached          - true if response from cache
//   default / user  - result arrays
func (a *App) buildResourceStatusResponse(report *blockcheck.BulkReport, checkedAt time.Time, cached bool) map[string]interface{} {
	return map[string]interface{}{
		"default":         report.Default,
		"user":            report.User,
		"checked_at":      checkedAt.Format("2006-01-02 15:04:05"),
		"checked_at_unix": checkedAt.Unix(),
		"cached":          cached,
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

	bc := a.cfg.GetBlockCheckConfig()
	checker := blockcheck.NewChecker(bc.CheckTCP, bc.CheckTLS, bc.CheckHTTP, bc.TimeoutSec)
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

	bc2 := a.cfg.GetBlockCheckConfig()
	checker := blockcheck.NewChecker(bc2.CheckTCP, bc2.CheckTLS, bc2.CheckHTTP, bc2.TimeoutSec)

	provider := checker.GetProviderInfo()
	direct := checker.Check(rawURL)

	blocked := direct.Verdict != blockcheck.VerdictOK

	inList := a.isHostInUserList(direct.Host)

	// Frontend ResourceChecker.jsx chitaet PascalCase-kljuchi
	// (report.Direct.Verdict, report.Direct.TCP.Ok, ...), poetomu sobiraem
	// map vruchnuju s PascalCase-kljuchami, a ne otdajom FullReport kak est'
	// (ego json-tegi lowercase slomali by frontend).
	return map[string]interface{}{
		"report": map[string]interface{}{
			"URL":        rawURL,
			"Host":       direct.Host,
			"Direct":     directToMap(direct),
			"Provider": map[string]interface{}{
				"IP":      provider.IP,
				"ISP":     provider.ISP,
				"City":    provider.City,
				"Country": provider.Country,
				"Org":     provider.Org,
				"ASN":     provider.ASN,
			},
			"Blocked":    blocked,
			"BlockType":  direct.Verdict,
			"InUserList": inList,
			"CheckedAt":  time.Now().Format("2006-01-02 15:04:05"),
		},
	}
}

// directToMap preobrazuet CheckResult v map s PascalCase-kljuchami,
// chtoby frontend ResourceChecker.jsx mog chitat' report.Direct.TCP.Ok i t.d.
func directToMap(r blockcheck.CheckResult) map[string]interface{} {
	return map[string]interface{}{
		"URL":        r.URL,
		"Host":       r.Host,
		"TCP":        layerToMap(r.TCP),
		"TLS":        layerToMap(r.TLS),
		"HTTP":       layerToMap(r.HTTP),
		"Verdict":    r.Verdict,
		"Confidence": r.Confidence,
		"Notes":      r.Notes,
	}
}

func layerToMap(l blockcheck.LayerResult) map[string]interface{} {
	return map[string]interface{}{
		"Ok":       l.Ok,
		"TimeMs":   l.TimeMs,
		"Error":    l.Error,
		"Status":   l.Status,
		"StubPage": l.StubPage,
		"Header":   l.Header,
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
// filterSkipped removes targets whose Name matches any entry in skip-resources.txt.
// Match is case-insensitive (host == entry OR host ends with "."+entry).
func (a *App) filterSkipped(targets []blockcheck.BulkTarget) []blockcheck.BulkTarget {
	out := make([]blockcheck.BulkTarget, 0, len(targets))
	for _, t := range targets {
		if a.cfg.IsSkippedResource(t.Name) {
			continue
		}
		out = append(out, t)
	}
	return out
}
