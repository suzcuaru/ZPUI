package app

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/blockcheck"
	"zpui/internal/executil"
)

// DetectThirdPartyZapret проверяет, есть ли на системе сторонний zapret
// (установленный как служба Windows), но отсутствующий в нашей локальной папке.
func (a *App) DetectThirdPartyZapret() map[string]interface{} {
	hasLocal := a.HasLocalZapret()
	if hasLocal {
		return map[string]interface{}{
			"has_local":          true,
			"has_third_party":    false,
			"third_party_detail": "",
		}
	}

	// Проверяем через службу
	output, err := executil.HiddenCmd("sc", "query", "zapret").Output()
	if err == nil && strings.Contains(string(output), "zapret") {
		detail := extractServiceDetail(output)
		return map[string]interface{}{
			"has_local":          false,
			"has_third_party":    true,
			"third_party_detail": detail,
		}
	}

	// Проверяем через процессы
	procOut, _ := exec.Command("tasklist", "/FI", "IMAGENAME eq winws.exe", "/NH").Output()
	if strings.Contains(string(procOut), "winws.exe") {
		return map[string]interface{}{
			"has_local":          false,
			"has_third_party":    true,
			"third_party_detail": "Обнаружен запущенный процесс winws.exe (сторонний zapret)",
		}
	}

	return map[string]interface{}{
		"has_local":          false,
		"has_third_party":    false,
		"third_party_detail": "",
	}
}

func extractServiceDetail(output []byte) string {
	lines := strings.Split(string(output), "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "STATE") {
			parts := strings.SplitN(l, ":", 2)
			if len(parts) == 2 {
				return "Служба zapret: " + strings.TrimSpace(parts[1])
			}
		}
	}
	return "Служба zapret установлена"
}

// RemoveThirdPartyZapret удаляет сторонний zapret (службу, процесс, драйверы)
func (a *App) RemoveThirdPartyZapret() map[string]interface{} {
	a.log.Info("setup", "Removing third-party zapret...")
	a.zapret.Teardown()
	a.log.Info("setup", "Third-party zapret removed")
	return okResp()
}

// InstallOurZapret скачивает и устанавливает наш zapret
func (a *App) InstallOurZapret() map[string]interface{} {
	a.log.Info("setup", "Installing our zapret...")

	progressFn := func(downloaded, total int64) {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "setup:progress", map[string]interface{}{
				"phase":   "download",
				"current": downloaded,
				"total":   total,
				"percent": percentOrZero(downloaded, total),
			})
		}
	}

	if err := a.zapret.DownloadAndInstall(progressFn); err != nil {
		a.log.Error("setup", "Install failed: "+err.Error())
		return errResp(err.Error())
	}

	a.log.Info("setup", "Zapret installed successfully")
	return okResp()
}

// StartOurZapret запускает zapret и возвращает статус
func (a *App) StartOurZapret() map[string]interface{} {
	a.log.Info("setup", "Starting our zapret...")

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "setup:progress", map[string]interface{}{
			"phase":   "start",
			"current": 0,
			"total":   0,
			"percent": 0,
		})
	}

	if err := a.zapret.Start(); err != nil {
		a.log.Error("setup", "Start failed: "+err.Error())
		return errResp(err.Error())
	}

	version := a.zapret.GetVersion()
	strategy := a.cfg.GetCurrentStrategy()
	if strategy == "" {
		strategy = a.zapret.DefaultStrategyName()
	}

	return map[string]interface{}{
		"version":  version,
		"strategy": strategy,
		"status":   "running",
	}
}

// SetupListStrategies возвращает список стратегий с результатами проверки ресурсов.
// Если передан strategy — временно переключается на неё для проверки.
func (a *App) SetupListStrategies(strategy string) map[string]interface{} {
	strategies := a.zapret.ListStrategies()
	var names []string
	for _, s := range strategies {
		names = append(names, s.Filename)
	}

	if strategy == "" {
		strategy = a.cfg.GetCurrentStrategy()
	}
	if strategy == "" {
		strategy = a.zapret.DefaultStrategyName()
	}

	resourceResults := a.checkResourcesOnStrategy(strategy)

	return map[string]interface{}{
		"strategies": names,
		"current":    strategy,
		"resources":  resourceResults,
	}
}

func (a *App) checkResourcesOnStrategy(strategy string) []blockcheck.BulkResult {
	targets, _ := blockcheck.ReadTargets(blockcheck.DefaultTargetsPath(a.cfg.GetZapretPath()))

	if strategy != "" {
		a.cfg.SetCurrentStrategy(strategy)
		if err := a.zapret.SetStrategy(strategy); err != nil {
			a.log.Warn("setup", fmt.Sprintf("Strategy switch failed: %v", err))
		}
		time.Sleep(2 * time.Second)
	}

	bc := a.cfg.GetBlockCheckConfig()
	checker := blockcheck.NewChecker(bc.CheckTCP, bc.CheckTLS, bc.CheckHTTP, bc.TimeoutSec)
	report := checker.BulkCheck(targets, nil)
	return report.Default
}

// SetupControlCheck останавливает запрет и проверяет ресурсы без обхода.
// Результат сохраняется как эталон (controlBaseline) для сравнения со стратегиями.
func (a *App) SetupControlCheck() map[string]interface{} {
	a.log.Info("setup", "Control check: stopping zapret for baseline...")

	a.zapret.Stop()
	executil.HiddenCmd("taskkill", "/IM", "winws.exe", "/F").Run()
	executil.HiddenCmd("sc", "stop", "WinDivert").Run()
	executil.HiddenCmd("sc", "stop", "WinDivert14").Run()
	time.Sleep(3 * time.Second)

	targets, _ := blockcheck.ReadTargets(blockcheck.DefaultTargetsPath(a.cfg.GetZapretPath()))
	if len(targets) == 0 {
		return errResp("не найдены ресурсы для проверки (lists/list-general.txt)")
	}

	bc2 := a.cfg.GetBlockCheckConfig()
	checker := blockcheck.NewChecker(bc2.CheckTCP, bc2.CheckTLS, bc2.CheckHTTP, bc2.TimeoutSec)
	report := checker.BulkCheck(targets, nil)

	baseline := make(map[string]bool)
	blockedCount := 0
	for _, r := range report.Default {
		baseline[r.Name] = !r.OK
		if !r.OK {
			blockedCount++
		}
	}
	a.controlBaseline = baseline

	a.log.Info("setup", fmt.Sprintf("Control check done: %d/%d blocked without zapret", blockedCount, len(report.Default)))

	return map[string]interface{}{
		"total":   len(report.Default),
		"blocked": blockedCount,
	}
}

// SetupTestStrategy применяет стратегию, ждёт подтверждения запуска,
// проверяет ресурсы и сравнивает с эталоном (controlBaseline).
// Возвращает процент разблокированных ресурсов.
func (a *App) SetupTestStrategy(strategy string) map[string]interface{} {
	if strategy == "" {
		return errResp("strategy required")
	}

	a.log.Info("setup", fmt.Sprintf("Testing strategy: %s", strategy))

	executil.HiddenCmd("taskkill", "/IM", "winws.exe", "/F").Run()
	time.Sleep(1 * time.Second)

	a.cfg.SetCurrentStrategy(strategy)
	if err := a.zapret.SetStrategy(strategy); err != nil {
		return errResp(fmt.Sprintf("strategy apply failed: %v", err))
	}

	running := false
	for i := 0; i < 15; i++ {
		svc := a.zapret.GetServiceStatus()
		if svc.Running {
			running = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !running {
		a.log.Warn("setup", fmt.Sprintf("Strategy %s: service not running after 15s, checking anyway", strategy))
	}
	time.Sleep(2 * time.Second)

	targets, _ := blockcheck.ReadTargets(blockcheck.DefaultTargetsPath(a.cfg.GetZapretPath()))
	bc3 := a.cfg.GetBlockCheckConfig()
	checker := blockcheck.NewChecker(bc3.CheckTCP, bc3.CheckTLS, bc3.CheckHTTP, bc3.TimeoutSec)
	report := checker.BulkCheck(targets, nil)

	baseline := a.controlBaseline
	if baseline == nil {
		baseline = make(map[string]bool)
	}

	blockedInBaseline := 0
	unblocked := 0
	var stillBlocked []map[string]interface{}
	for _, r := range report.Default {
		if !baseline[r.Name] {
			continue
		}
		blockedInBaseline++
		if r.OK {
			unblocked++
		} else {
			stillBlocked = append(stillBlocked, map[string]interface{}{
				"name":    r.Name,
				"url":     r.URL,
				"verdict": r.Verdict,
			})
		}
	}

	percentage := 100
	if blockedInBaseline > 0 {
		percentage = unblocked * 100 / blockedInBaseline
	}

	a.log.Info("setup", fmt.Sprintf("Strategy %s: %d%% (%d/%d unblocked)",
		strategy, percentage, unblocked, blockedInBaseline))

	return map[string]interface{}{
		"strategy":            strategy,
		"percentage":          percentage,
		"blocked_in_baseline": blockedInBaseline,
		"unblocked":           unblocked,
		"still_blocked":       stillBlocked,
	}
}

// SetupApplyStrategy применяет выбранную стратегию
func (a *App) SetupApplyStrategy(strategy string) map[string]interface{} {
	if strategy == "" {
		return errResp("strategy required")
	}

	a.cfg.SetCurrentStrategy(strategy)
	if err := a.zapret.SetStrategy(strategy); err != nil {
		return errResp(err.Error())
	}

	return okResp()
}

// SetupConfigureFilters настройка игрового фильтра (disabled/all/tcp/udp)
func (a *App) SetupConfigureFilters(mode string) map[string]interface{} {
	if err := a.zapret.SetGameFilter(mode); err != nil {
		return errResp(err.Error())
	}

	return okResp()
}

// SetupConfigureDNS настройка Xbox DNS (xbox-dns.ru: 111.88.96.50 / 111.88.96.51)
func (a *App) SetupConfigureDNS(enable bool) map[string]interface{} {
	if !enable {
		if a.xboxDns.IsEnabled() {
			a.xboxDns.Disable()
		}
		xd := a.cfg.GetXboxDnsConfig()
		xd.Enabled = false
		a.cfg.SetXboxDnsConfig(xd)
		return okResp()
	}

	xd := a.cfg.GetXboxDnsConfig()
	a.xboxDns.Configure(xd.PrimaryDNS, xd.SecondaryDNS)
	if err := a.xboxDns.Enable(); err != nil {
		return errResp(err.Error())
	}
	xd.Enabled = true
	a.cfg.SetXboxDnsConfig(xd)

	a.log.Info("setup", fmt.Sprintf("Xbox DNS enabled: %s / %s", xd.PrimaryDNS, xd.SecondaryDNS))
	return okResp()
}

// SetupConfigureProxy настройка прокси
func (a *App) SetupConfigureProxy(enable bool, port int, bindHost string) map[string]interface{} {
	if !enable {
		if a.proxy.IsRunning() {
			a.proxy.Stop()
		}
		pcfg := a.cfg.GetProxyConfig()
		pcfg.Enabled = false
		a.cfg.SetProxyConfig(pcfg)
		return okResp()
	}

	if port <= 0 {
		port = 1080
	}
	if bindHost == "" {
		bindHost = "127.0.0.1"
	}

	pcfg := a.cfg.GetProxyConfig()
	pcfg.Enabled = true
	pcfg.AutoStart = true
	pcfg.Port = port
	pcfg.BindHost = bindHost
	a.cfg.SetProxyConfig(pcfg)

	if err := a.proxy.Start(); err != nil {
		return errResp(err.Error())
	}

	return okResp()
}

// SetupSkip помечает, что пользователь пропустил настройку
func (a *App) SetupSkip() map[string]interface{} {
	a.cfg.SetZapretSkipped(true)
	a.cfg.FirstRunDone = true
	a.cfg.Save()
	return okResp()
}

// SetupComplete финализирует настройку
func (a *App) SetupComplete() map[string]interface{} {
	a.cfg.FirstRunDone = true
	a.cfg.Save()
	return okResp()
}
