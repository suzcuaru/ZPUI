package app

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"zpui/internal/config"
	"zpui/internal/database"
	"zpui/internal/executil"
	"zpui/internal/monitor"
	"zpui/internal/power"
	"zpui/internal/tgproxy"
)

// ============================================================
// STATUS
// ============================================================

// archName — битность запущенного процесса (для тайтлбара и отчётов).
func archName() string {
	switch runtime.GOARCH {
	case "386":
		return "x86"
	case "amd64":
		return "x64"
	default:
		return runtime.GOARCH
	}
}

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
	pRun := a.tgproxy.IsRunning()

	var lastXbox bool
	_ = a.cfg.Update(func(c *config.Config) {
		c.LastZapretState = zRun
		c.LastProxyState = pRun
		c.LastXboxDnsState = c.XboxDns.Enabled
		lastXbox = c.XboxDns.Enabled
	})
	return map[string]interface{}{
		"zapret":   zRun,
		"proxy":    pRun,
		"xbox_dns": lastXbox,
	}
}

// svcInstalledHint — кэш факта «служба winws установлена»: чек в SCM не чаще
// раза в 10 секунд, в остальное время — последнее известное значение. Нужен,
// чтобы панель службы в настройках рисовалась сразу, без скелетона.
func (a *App) svcInstalledHint() bool {
	if time.Since(time.Unix(0, a.svcHintAt.Load())) > 10*time.Second {
		installed := a.HasSystemZapretService()
		a.svcHintInstalled.Store(installed)
		a.svcHintAt.Store(time.Now().UnixNano())
	}
	return a.svcHintInstalled.Load()
}

// sysXboxAppliedCached — IsSetInSystem с TTL 3 c: чтение реестра DNS
// адаптера на каждый 2-с тик /api/status не нужно.
func (a *App) sysXboxAppliedCached() bool {
	a.sysCacheMu.Lock()
	defer a.sysCacheMu.Unlock()
	if a.sysXboxAppliedAt.IsZero() || time.Since(a.sysXboxAppliedAt) >= 3*time.Second {
		a.sysXboxApplied = a.xboxDns.IsSetInSystem()
		a.sysXboxAppliedAt = time.Now()
	}
	return a.sysXboxApplied
}

// sysAutostartCached — факт автозапуска с TTL 5 c (два чтения реестра).
// Свежий факт без кэша даёт GetAutostartStatus — панель настроек читает его.
func (a *App) sysAutostartCached() bool {
	a.sysCacheMu.Lock()
	defer a.sysCacheMu.Unlock()
	if a.sysAutostartAt.IsZero() || time.Since(a.sysAutostartAt) >= 5*time.Second {
		a.sysAutostart = autostartEnabled()
		a.sysAutostartAt = time.Now()
	}
	return a.sysAutostart
}

// sysNetCached — hostname/MAC/локальные IP с TTL 5 c.
func (a *App) sysNetCached() (ips []string, mac, hostname string) {
	a.sysCacheMu.Lock()
	defer a.sysCacheMu.Unlock()
	if a.sysNetAt.IsZero() || time.Since(a.sysNetAt) >= 5*time.Second {
		a.sysIPs = getLocalIPs()
		a.sysMAC = getMACAddress()
		a.sysHostname = getHostname()
		a.sysNetAt = time.Now()
	}
	return a.sysIPs, a.sysMAC, a.sysHostname
}

func (a *App) invalidateSysCaches() {
	a.sysCacheMu.Lock()
	defer a.sysCacheMu.Unlock()
	a.sysXboxAppliedAt = time.Time{}
	a.sysAutostartAt = time.Time{}
	a.sysNetAt = time.Time{}
}

// GetStatus — агрегированный статус системы (замена GET /api/status).

// selectionLastAt — unix-сек последнего завершённого ручного прогона
// подбора (app_flags full_sweep_last_at); 0 — ручных прогонов ещё не было.
func (a *App) selectionLastAt() int64 {
	if v, err := database.GetFlag("full_sweep_last_at"); err == nil {
		if n, perr := strconv.ParseInt(v, 10, 64); perr == nil {
			return n
		}
	}
	return 0
}
func (a *App) GetStatus() map[string]interface{} {
	traffic := a.monitor.GetCurrentStats()
	batteryPct, hasBattery := power.BatteryPercent()

	ips, macAddr, hostName := a.sysNetCached()
	if len(ips) == 0 {
		ips = []string{"127.0.0.1"}
	}

	zapretStatus := string(a.zapret.GetStatus())
	strategy := a.zapret.GetCurrentStrategy()

	svcInstalled := a.svcInstalledHint()
	if zapretStatus == "running" {
		svcStatus := a.zapret.GetServiceStatus()
		if svcStatus.Strategy != "" {
			strategy = svcStatus.Strategy
		}
		svcInstalled = svcStatus.Installed
	}

	xboxInfo := map[string]interface{}{
		"applied": a.sysXboxAppliedCached(),
	}
	a.cfg.RView(func() {
		xboxInfo["enabled"] = a.cfg.XboxDns.Enabled
		xboxInfo["xbox_enabled"] = a.cfg.XboxDns.XboxEnabled
		xboxInfo["servers"] = a.cfg.XboxDns.PlainIPs()
	})

	modInfo := map[string]interface{}{
		"version":       a.version,
		"arch":          archName(),
		"autostart":     a.sysAutostartCached(),
		"close_to_tray": a.cfg.GetCloseToTray(),
	}
	a.cfg.RView(func() {
		modInfo["web_port"] = a.cfg.Web.Port
		modInfo["zapret_repo"] = a.cfg.ZapretRepoURL
		modInfo["mod_repo"] = a.cfg.ModRepoURL
		modInfo["theme"] = a.cfg.Theme
		modInfo["first_run_done"] = a.cfg.FirstRunDone
		modInfo["start_minimized"] = a.cfg.StartMinimized
	})

	return map[string]interface{}{
		// Метка последнего завершённого ручного прогона подбора (unix-сек):
		// дашборд по ней показывает напоминание, когда прогону больше 7 дней.
		"selection_last_at": a.selectionLastAt(),
		"zapret": map[string]interface{}{
			"status":            zapretStatus,
			"version":           a.zapret.GetVersion(),
			"engine":            "zapret",
			"strategy":          strategy,
			"zapretDir":         a.cfg.GetZapretPath(),
			"auto_test_running": a.zapret.IsAutoTestRunning(),
			"service_installed": svcInstalled,
		},
		"tg_proxy": map[string]interface{}{
			"running": a.tgproxy.IsRunning(),
			"version": tgproxy.Version,
			"status":  a.tgproxy.GetStatus(),
			"stats":   a.tgproxy.GetStats(),
		},
		"xbox_dns": xboxInfo,
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
		"mod": modInfo,
		"network": map[string]interface{}{
			"hostname": hostName,
			"mac":      macAddr,
			"ips":      ips,
		},
		// Эффективное состояние экономного режима вычисляет бэкенд: после
		// закрытия/переоткрытия окна индикатор в сайдбаре появляется сразу,
		// независимо от доступности getBattery во фронтенде.
		"power_saving":        power.Active(),
		"power_saving_level":  power.Level(),
		"power_saving_paused": power.MonitoringPaused(),
		"power_battery": map[string]interface{}{
			"percent": batteryPct,
			"has":     hasBattery,
		},
	}
}

// ============================================================
// RESTART
// ============================================================

func (a *App) RestartApp() map[string]interface{} {
	a.log.Info("app", "RestartApp called")
	done := make(chan struct{})
	go func() {
		_ = a.cfg.SetZapretSkipped(false)
		_ = a.cfg.Update(func(c *config.Config) {
			c.FirstRunDone = false
			c.LastZapretState = false
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		a.log.Warn("app", "RestartApp: config reset timed out — restarting anyway")
	}
	go a.Restart()
	return map[string]interface{}{"status": "ok"}
}

// RestartAppClean перезапускает приложение без сброса настроек и мастера
// первого запуска. Используется после неудачного восстановления запрета:
// после перезапуска автозапуск движка и восстановление пробуют ещё раз.
func (a *App) RestartAppClean() map[string]interface{} {
	a.log.Info("app", "RestartAppClean called")
	go a.Restart()
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
	// Списки — единицы МБ; потолок страхует 32-битный процесс от OOM
	// при странном ответе сервера.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	os.MkdirAll(filepath.Dir(destPath), 0755)
	if err := os.WriteFile(destPath, body, 0644); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"status": "ok"}
}
