package app

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"

	"zpui/internal/config"
	"zpui/internal/power"
	"zpui/internal/zsvc"
)

// ============================================================
// AUTOSTART
// ============================================================
//
// Механизм: HKCU\...\Run — классическая запись автозапуска, «как у всех
// приложений»: видна во вкладке «Автозапуск» Диспетчера задач и управляется
// и оттуда, и тумблером приложения. Запись запускает не сам exe, а релей
// ZPUI.exe --run-start-task, который скрыто дёргает задачу Планировщика
// ZPUI-Start: у задачи RunLevel Highest, поэтому приложение стартует с правами
// админа БЕЗ запроса UAC при входе. У задачи
// нет триггера входа — стреляет только Run-запись, управление целиком
// остаётся у Диспетчера задач (отключение там = StartupApproved-флаг).
// Отключение в Диспетчере задач не удаляет запись, а помечает её в
// StartupApproved\Run (первый байт нечётный) — этот флаг читается как
// «выключено». Если задачу создать не удалось — фолбэк: запись запускает exe
// напрямую (как раньше, ценой UAC при входе). Служба ZPUI автозапуском не
// занимается: она использует внутреннюю задачу только для краш-восстановления.

const (
	autostartRunKey      = `Software\Microsoft\Windows\CurrentVersion\Run`
	autostartApprovedKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
	autostartValueName   = "ZPUI"
)

// autostartCommand — значение Run-записи: запуск задачи автозапуска (права
// админа без UAC при входе) через релей ZPUI.exe --run-start-task. Прямой
// schtasks.exe в записи нельзя: это консольное приложение, и при входе
// Explorer запускает его с видимым окном терминала. ZPUI.exe — GUI-подсистема,
// консоли не создаёт, задачу дёргает скрыто и сразу выходит. Флаг «свёрнутого»
// запуска живёт в задаче.
func autostartCommand(exePath string) string {
	return `"` + exePath + `" --run-start-task`
}

// autostartDirectCommand — фолбэк, если задачу создать не удалось: exe
// напрямую (при входе появится запрос UAC).
func autostartDirectCommand(exePath string, hidden bool) string {
	cmd := `"` + exePath + `"`
	if hidden {
		cmd += " --hidden"
	}
	return cmd
}

// autostartRunValue — текущее значение Run-записи (ok=false, записи нет).
func autostartRunValue() (string, bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, autostartRunKey, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(autostartValueName)
	if err != nil {
		return "", false
	}
	return v, true
}

func autostartRunExists() bool {
	_, ok := autostartRunValue()
	return ok
}

// autostartDisabledByUser — запись отключена в Диспетчере задач
// (StartupApproved\Run, первый байт нечётный).
func autostartDisabledByUser() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, autostartApprovedKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetBinaryValue(autostartValueName)
	if err != nil || len(v) == 0 {
		return false
	}
	return v[0]&1 == 1
}

// autostartEnabled — реальное эффективное состояние автозапуска.
func autostartEnabled() bool {
	return autostartRunExists() && !autostartDisabledByUser()
}

// ensureAutostartTask создаёт/канонизирует задачу автозапуска под текущий
// exe и настройку свёрнутого запуска. ok=false — задачу создать не удалось.
func (a *App) ensureAutostartTask(exePath string) bool {
	if err := zsvc.CreateStartTask(exePath, a.cfg.StartMinimized); err != nil {
		a.log.Warn("autostart", "Start task create failed: "+err.Error())
		return false
	}
	return true
}

// setAutostartOn включает автозапуск: задача + значение записи + снятие флага
// отключения. При невозможности создать задачу — фолбэк на прямой запуск.
func (a *App) setAutostartOn(exePath string, hidden bool) error {
	value := autostartCommand(exePath)
	if !a.ensureAutostartTask(exePath) {
		value = autostartDirectCommand(exePath, hidden)
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, autostartRunKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetStringValue(autostartValueName, value); err != nil {
		return err
	}
	clearAutostartApproved()
	return nil
}

func clearAutostartApproved() {
	k, err := registry.OpenKey(registry.CURRENT_USER, autostartApprovedKey, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	_ = k.DeleteValue(autostartValueName)
}

func removeAutostart() {
	if k, err := registry.OpenKey(registry.CURRENT_USER, autostartRunKey, registry.SET_VALUE); err == nil {
		_ = k.DeleteValue(autostartValueName)
		k.Close()
	}
	clearAutostartApproved()
	zsvc.RemoveStartTask()
}

func removeLegacyStartupShortcut() {
	startupPath := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows",
		"Start Menu", "Programs", "Startup", "ZPUI.lnk")
	os.Remove(startupPath)
}

func (a *App) GetAutostartStatus() map[string]interface{} {
	return map[string]interface{}{
		"enabled": autostartEnabled(),
		"active":  autostartRunExists(),
	}
}

func (a *App) EnableAutostart() map[string]interface{} {
	a.log.Info("autostart", "EnableAutostart called")
	exePath := getExePath()
	if exePath == "" {
		a.log.Error("autostart", "Cannot get exe path")
		return errResp("не удалось определить путь к исполняемому файлу")
	}
	if err := a.setAutostartOn(exePath, a.cfg.StartMinimized); err != nil {
		a.log.Error("autostart", "Autostart enable failed: "+err.Error())
		return errResp(err.Error())
	}
	zsvc.RemoveLegacyTask()
	a.log.Info("autostart", "Autostart enabled (task ZPUI-Start, no UAC at logon)")
	return okRespWithData(map[string]interface{}{"status": "enabled"})
}

func (a *App) DisableAutostart() map[string]interface{} {
	a.log.Info("autostart", "DisableAutostart called")
	removeAutostart()
	zsvc.RemoveLegacyTask()
	a.log.Info("autostart", "Run-key autostart disabled")
	return okRespWithData(map[string]interface{}{"status": "disabled"})
}

// ============================================================
// CONFIG
// ============================================================

func (a *App) GetConfig() map[string]interface{} {
	m := map[string]interface{}{
		"current_strategy":        a.cfg.GetCurrentStrategy(),
		"xbox_dns":                a.cfg.GetXboxDnsConfig(),
		"autostart":               a.sysAutostartCached(),
		"auto_skip_dead_domains":  a.cfg.GetAutoSkipDeadDomains(),
		"proxy_enabled":           a.cfg.GetProxyEnabled(),
		"language":                a.cfg.GetLanguage(),
		"sounds":                  a.cfg.GetSounds(),
		"zapret_skipped":          a.cfg.GetZapretSkipped(),
		"vpn_ignore":              a.cfg.GetVpnIgnore(),
		"close_to_tray":           a.cfg.GetCloseToTray(),
		"notifications_enabled":   a.cfg.GetNotificationsEnabled(),
		"resource_drop_pct":       a.cfg.GetResourceDropPct(),
		"resource_check_interval": a.cfg.GetResourceCheckInterval(),
		"server":                  a.cfg.GetServerConfig(),
		"authorization_enabled":   a.cfg.GetAuthorizationEnabled(),
	}
	a.cfg.RView(func() {
		m["web_port"] = a.cfg.Web.Port
		m["auto_update_check"] = a.cfg.AutoUpdateCheck
		m["theme"] = a.cfg.Theme
		m["first_run_done"] = a.cfg.FirstRunDone
		m["start_minimized"] = a.cfg.StartMinimized
		m["last_zapret_state"] = a.cfg.LastZapretState
		m["last_proxy_state"] = a.cfg.LastProxyState
		m["last_xbox_dns_state"] = a.cfg.LastXboxDnsState
		m["auto_start_zapret"] = a.cfg.AutoStartZapret
		m["auto_start_proxy"] = a.cfg.AutoStartProxy
		m["auto_start_xbox_dns"] = a.cfg.AutoStartXboxDns
		m["power_saving"] = a.cfg.PowerSaving
		m["power_level"] = a.cfg.PowerLevel
		m["power_threshold"] = a.cfg.PowerThreshold
		m["notify_zpui_updates"] = a.cfg.NotifyZPUIUpdates
		m["notify_zapret_updates"] = a.cfg.NotifyZapretUpdates
		m["notify_missing_files"] = a.cfg.NotifyMissingFiles
		m["notify_service_crash"] = a.cfg.NotifyServiceCrash
		m["notify_errors"] = a.cfg.NotifyErrors
		m["notify_resource_drop"] = a.cfg.NotifyResourceDrop
		m["show_strategy_modal"] = a.cfg.ShowStrategyModal
		m["notify_strategy_test"] = a.cfg.NotifyStrategyTest
		m["logs"] = a.cfg.Logs
		// Копия под read-локом: карта конфига наружу не отдаётся (гонка с
		// писателями), nil сериализуется в null — фронт трактует как «не задано».
		var togglesCopy map[string]bool
		if len(a.cfg.StratTierToggles) > 0 {
			togglesCopy = make(map[string]bool, len(a.cfg.StratTierToggles))
			for k, v := range a.cfg.StratTierToggles {
				togglesCopy[k] = v
			}
		}
		m["strat_tier_toggles"] = togglesCopy
	})
	return m
}

func (a *App) SetConfig(opts map[string]interface{}) map[string]interface{} {
	a.log.Info("config", "SetConfig called")
	if port, ok := opts["web_port"].(float64); ok {
		_ = a.cfg.Update(func(c *config.Config) { c.Web.Port = int(port) })
	}
	if theme, ok := opts["theme"].(string); ok {
		a.cfg.SetTheme(theme)
		// Обычная тема снимает активный слот кастомной (галерея и ряд «Тема»
		// не должны расходиться).
		if theme != "custom" {
			a.themes.ClearActive()
		}
	}
	if snd, ok := opts["sounds"].(map[string]interface{}); ok {
		sc := a.cfg.GetSounds()
		if v, ok := snd["enabled"].(bool); ok {
			sc.Enabled = v
		}
		if v, ok := snd["volume"].(float64); ok && v >= 0 && v <= 100 {
			sc.Volume = int(v)
		}
		if v, ok := snd["scheme"].(string); ok {
			switch v {
			case "standard", "soft", "quiet":
				sc.Scheme = v
			}
		}
		a.cfg.SetSounds(sc)
	}
	// Флаги верхнего уровня — одной мутацией под замком (прямые записи в
	// экспортируемые поля гонятся с фоновыми читателями конфига).
	var syncStartMinimized bool
	if err := a.cfg.Update(func(c *config.Config) {
		if v, ok := opts["proxy_enabled"].(bool); ok {
			c.Proxy.Enabled = v
		}
		if v, ok := opts["auto_update_check"].(bool); ok {
			c.AutoUpdateCheck = v
		}
		if v, ok := opts["auto_skip_dead_domains"].(bool); ok {
			c.AutoSkipDeadDomains = v
		}
		if v, ok := opts["start_minimized"].(bool); ok && v != c.StartMinimized {
			// Настройка влияет только на автозапуск: при живой службе — её IPC
			// (сам пересоздаст задачу с нужным --hidden), иначе локально.
			c.StartMinimized = v
			syncStartMinimized = true
		}
		if v, ok := opts["close_to_tray"].(bool); ok {
			c.CloseToTray = v
		}
		if v, ok := opts["auto_start_zapret"].(bool); ok {
			c.AutoStartZapret = v
		}
		if v, ok := opts["auto_start_proxy"].(bool); ok {
			c.AutoStartProxy = v
		}
		if v, ok := opts["auto_start_xbox_dns"].(bool); ok {
			c.AutoStartXboxDns = v
		}
		if v, ok := opts["power_saving"].(bool); ok {
			c.PowerSaving = v
		}
		if v, ok := opts["power_level"].(float64); ok {
			c.PowerLevel = int(v)
		}
		if v, ok := opts["power_threshold"].(float64); ok {
			c.PowerThreshold = int(v)
		}
		power.Set(c.PowerSaving, c.PowerLevel, c.PowerThreshold)
		if v, ok := opts["language"].(string); ok {
			c.Language = v
		}
		if v, ok := opts["first_run_done"].(bool); ok {
			c.FirstRunDone = v
		}
	}); err != nil {
		a.log.Error("config", "Update: "+err.Error())
	}
	if syncStartMinimized {
		// Настройка влияет только на автозапуск: Run-запись переписывается
		// с новым --hidden (само значение флага отключения из Диспетчера
		// задач не трогается).
		a.updateAutostartRunValue()
	}
	if v, ok := opts["zapret_skipped"].(bool); ok {
		a.cfg.SetZapretSkipped(v)
	}
	if raw, ok := opts["vpn_ignore"].([]interface{}); ok {
		names := make([]string, 0, len(raw))
		for _, item := range raw {
			if s, ok := item.(string); ok {
				names = append(names, s)
			}
		}
		a.cfg.SetVpnIgnore(names)
	}
	if raw, ok := opts["strat_tier_toggles"].(map[string]interface{}); ok && len(raw) > 0 {
		toggles := make(map[string]bool, len(raw))
		for k, v := range raw {
			if b, ok := v.(bool); ok {
				toggles[k] = b
			}
		}
		if len(toggles) > 0 {
			if err := a.cfg.SetStratTierToggles(toggles); err != nil {
				a.log.Error("config", "strat_tier_toggles save: "+err.Error())
			}
		}
	}
	if v, ok := opts["notifications_enabled"].(bool); ok {
		a.cfg.SetNotificationsEnabled(v)
	}
	if v, ok := opts["resource_drop_pct"].(float64); ok {
		a.cfg.SetResourceDropPct(int(v))
	}
	if v, ok := opts["resource_check_interval"].(float64); ok {
		a.cfg.SetResourceCheckInterval(int(v))
	}
	// server: атомарная запись через UpdateServerConfig — единственный канал
	// для полей server.* (параллельные писатели: heartbeat, активация, 401).
	// Пустые строки url/key/account_token НЕ затирают
	// хранимые непустые значения (анти-клоббер: фронт присылает снимки
	// конфига, и потеря сессии/адреса из-за пустоты в снимке недопустима).
	// Легитимные очистки — целевые методы: AccountLogout (token), 401-обработка
	// в accountRequest (token), SetServerKey (key).
	if srv, ok := opts["server"].(map[string]interface{}); ok {
		if err := a.cfg.UpdateServerConfig(func(sc *config.ServerConfig) {
			if v, ok := srv["url"].(string); ok {
				if u := strings.TrimSpace(v); u != "" {
					sc.URL = u
				}
			}
			if v, ok := srv["account_token"].(string); ok {
				if t := strings.TrimSpace(v); t != "" {
					sc.AccountToken = t
				}
			}
		}); err != nil {
			a.log.Error("config", "server config save: "+err.Error())
		}
	}
	if err := a.cfg.Save(); err != nil {
		a.log.Error("config", "Save error: "+err.Error())
	} else {
		a.log.Info("config", "Config saved to JSON")
		// Синхронизация настроек через аккаунт (sync/user) в v4 не существует:
		// настройки локальные; LAN-синхронизация между своими установками —
		// отдельный механизм (app_api_sync.go).
	}
	return map[string]interface{}{"status": "ok"}
}

// updateAutostartRunValue — перезаписать Run-запись и задачу с актуальным exe
// и --hidden (смена «Запускать свёрнутым», переезд приложения). Ничего не
// делает, когда записи нет, и не трогает флаг отключения из Диспетчера задач.
func (a *App) updateAutostartRunValue() {
	if !autostartRunExists() {
		return
	}
	if exePath := getExePath(); exePath != "" {
		if err := a.setAutostartOn(exePath, a.cfg.StartMinimized); err != nil {
			a.log.Warn("autostart", "Run-value update failed: "+err.Error())
		}
	}
}

// syncAutostartSystem на старте приводит автозапуск к стандартной схеме:
// переносит состояние из задачи Планировщика прежних версий в Run-ключ,
// сносит устаревшие артефакты и канонизирует запись и задачу (путь exe
// после обновления/переезда, актуальный --hidden). Источник истины — реестр
// (Run + StartupApproved): его же читает Диспетчер задач; config.json
// состоянием автозапуска не является.
func (a *App) syncAutostartSystem() {
	// Прежняя схема (задача ONLOGON): состояние переносится в Run-ключ, задача сносится.
	if zsvc.TaskExists() {
		wasOn := zsvc.TaskEffectiveEnabled()
		zsvc.RemoveLegacyTask()
		if wasOn && !autostartRunExists() {
			if exePath := getExePath(); exePath != "" {
				if err := a.setAutostartOn(exePath, a.cfg.StartMinimized); err != nil {
					a.log.Error("autostart", "Migration task → Run-key failed: "+err.Error())
				} else {
					a.log.Info("autostart", "Scheduled task migrated to Run-key autostart")
				}
			}
		} else {
			a.log.Info("autostart", "Legacy scheduled task removed")
		}
	}
	// Запись есть — канонизируем пару «задача + значение»: задача пересоздаётся
	// под текущий exe и --hidden (обновление, переезд папки), значение записи
	// переводится на задачу (или на прямой запуск, если задачу создать не удалось).
	if _, ok := autostartRunValue(); ok {
		if exePath := getExePath(); exePath != "" {
			if err := a.setAutostartOn(exePath, a.cfg.StartMinimized); err != nil {
				a.log.Warn("autostart", "Autostart canonicalization failed: "+err.Error())
			} else if a.runValueDrifted(exePath) {
				a.log.Info("autostart", "Autostart updated to current exe/flags")
			}
		}
	}
	removeLegacyStartupShortcut()
}

// runValueDrifted — значение Run-записи отличается от обеих канонических форм
// (задача автозапуска или прямой запуск при актуальных exe и --hidden).
func (a *App) runValueDrifted(exePath string) bool {
	v, ok := autostartRunValue()
	if !ok {
		return false
	}
	return v != autostartCommand(exePath) && v != autostartDirectCommand(exePath, a.cfg.StartMinimized)
}
