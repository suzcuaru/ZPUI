package app

import (
	"net/url"
	"time"
	"zpui/internal/database"
	"zpui/internal/executil"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)


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

	pRunning := a.tgproxy.IsRunning()
	pHealth := ComponentHealth{
		Name:    "Telegram proxy",
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

	modules := map[string]string{}

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

func (a *App) AutoInstallZapret() map[string]interface{} {
	a.writeStartupLog("SETUP", "Автоустановка zapret...")
	if !a.isNetworkAvailable() {
		a.writeStartupLog("SETUP", "Сеть недоступна — скачивание невозможно")
		return errResp("Network unavailable — cannot download zapret")
	}

	a.saveBackupToDB()
	database.SetZapretJustUpdated(true)
	a.writeStartupLog("SETUP", "Скачивание zapret...")
	err := a.pauseEngineWatchdogDuring(func() error {
		return a.zapret.DownloadAndInstall(func(downloaded, total int64) {
			runtime.EventsEmit(a.ctx, "download:progress", map[string]interface{}{
				"downloaded": downloaded,
				"total":      total,
				"percent":    percentOrZero(downloaded, total),
			})
		})
	})
	if err != nil {
		a.writeStartupLog("SETUP", "Ошибка установки: "+err.Error())
		return map[string]interface{}{"error": err.Error()}
	}
	a.writeStartupLog("SETUP", "Zapret установлен")

	strategy := a.zapret.GetCurrentStrategy()
	if strategy == "" {
		strategies := a.zapret.ListStrategies()
		if len(strategies) > 0 {
			strategy = strategies[0].Filename
			a.cfg.SetCurrentStrategy(strategy)
			a.cfg.Save()
		}
	}

	a.writeStartupLog("SETUP", "Стратегия: "+a.zapret.GetCurrentStrategy())
	return map[string]interface{}{
		"status":   "ok",
		"version":  a.zapret.GetVersion(),
		"strategy": a.zapret.GetCurrentStrategy(),
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
