package app

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"

	"zpui/internal/executil"
	"zpui/internal/mcpserver"
)

// MCP: локальный сервер управления ИИ-агентами (спека
// Документы/Задачи/2026-11-12-mcp-server.md). По умолчанию выключен,
// включается тумблером в модалке «MCP» (сайдбар). Инструменты — тонкие
// обёртки над существующими биндингами: агент получает те же ответы,
// что и фронт (okResp/errResp).

func (a *App) buildMcpServer() *mcpserver.Server {
	return mcpserver.New(mcpserver.Options{
		Version:  a.mcpVersion(),
		Key:      a.cfg.GetMcpKey,
		Tools:    a.mcpTools,
		Dispatch: a.mcpDispatch,
		Notify: func(level, msg string) {
			if level == "warn" {
				a.log.Warn("mcp", msg)
				return
			}
			a.log.Info("mcp", msg)
		},
	})
}

func (a *App) mcpVersion() string {
	if v, ok := a.GetVersions()["zpui"].(string); ok {
		return v
	}
	return ""
}

// mcpTools — реестр инструментов агентов (полный контроль без опасного:
// никаких обновлений, удаления, телеметрии и аккаунта).
func (a *App) mcpTools() []mcpserver.Tool {
	obj := func(props map[string]interface{}, required ...string) map[string]interface{} {
		if props == nil {
			// Строгие MCP-клиенты (zod) требуют properties-объект: null в JSON
			// отвергается валидацией tools/list.
			props = map[string]interface{}{}
		}
		m := map[string]interface{}{"type": "object", "properties": props}
		if len(required) > 0 {
			m["required"] = required
		}
		return m
	}
	str := func(desc string) map[string]interface{} { return map[string]interface{}{"type": "string", "description": desc} }
	return []mcpserver.Tool{
		{Name: "get_status", Description: "Полный статус ZPUI: движок обхода (zapret), SOCKS-прокси, DNS, канал данных.", InputSchema: obj(nil)},
		{Name: "engine_start", Description: "Запустить движок обхода блокировок.", InputSchema: obj(nil)},
		{Name: "engine_stop", Description: "Остановить движок обхода блокировок.", InputSchema: obj(nil)},
		{Name: "engine_restart", Description: "Перезапустить движок обхода (остановка + запуск).", InputSchema: obj(nil)},
		{Name: "list_strategies", Description: "Список доступных стратегий движка с пометкой активной.", InputSchema: obj(nil)},
		{Name: "set_strategy", Description: "Переключить стратегию движка по имени файла.", InputSchema: obj(map[string]interface{}{
			"name": str("имя файла стратегии, например general (ALT13).bat"),
		}, "name")},
		{Name: "run_autoselect", Description: "Запустить автоподбор стратегии (асинхронно). Результат — события автоподбора; статус потом через get_status.", InputSchema: obj(map[string]interface{}{
			"mode": map[string]interface{}{"type": "string", "enum": []string{"smart", "full", "game", "startup"}, "description": "smart — умный подбор (по умолчанию), full — полный прогон, game — игровой, startup — стартовый"},
		})},
		{Name: "get_availability", Description: "История доступности ресурсов: стандартные/пользовательские/ключевые за период.", InputSchema: obj(map[string]interface{}{
			"hours": map[string]interface{}{"type": "integer", "description": "сколько часов назад (по умолчанию 24)"},
		})},
		{Name: "dns_enable", Description: "Включить или выключить замену DNS в системе.", InputSchema: obj(map[string]interface{}{
			"enabled": map[string]interface{}{"type": "boolean"},
		}, "enabled")},
		{Name: "proxy_enable", Description: "Включить или выключить локальный SOCKS5-прокси.", InputSchema: obj(map[string]interface{}{
			"enabled": map[string]interface{}{"type": "boolean"},
		}, "enabled")},
		{Name: "get_logs", Description: "Хвост журнала событий программы.", InputSchema: obj(map[string]interface{}{
			"lines": map[string]interface{}{"type": "integer", "description": "сколько строк (по умолчанию 50, максимум 500)"},
		})},
	}
}

func (a *App) mcpDispatch(name string, args map[string]interface{}) (interface{}, error) {
	switch name {
	case "get_status":
		return a.GetStatus(), nil
	case "engine_start":
		return a.ZapretStart(), nil
	case "engine_stop":
		return a.ZapretStop(), nil
	case "engine_restart":
		if res := a.ZapretStop(); isErrorResp(res) {
			return res, nil
		}
		return a.ZapretStart(), nil
	case "list_strategies":
		return a.GetStrategies(), nil
	case "set_strategy":
		n, _ := args["name"].(string)
		if n == "" {
			return nil, fmt.Errorf("аргумент name обязателен")
		}
		return a.SetStrategy(n), nil
	case "run_autoselect":
		mode, _ := args["mode"].(string)
		if mode == "" {
			mode = "smart"
		}
		switch mode {
		case "smart", "full", "game", "startup":
		default:
			return nil, fmt.Errorf("mode: smart | full | game | startup")
		}
		a.RunAutoSelectStream(mode, "", false, false, "")
		return map[string]interface{}{"started": true, "mode": mode}, nil
	case "get_availability":
		hours := 24
		if v, ok := args["hours"].(float64); ok && v > 0 && v <= 720 {
			hours = int(v)
		}
		out := map[string]interface{}{}
		for _, typ := range []string{"standard", "user", "key"} {
			out[typ] = a.GetAvailabilityHistory(hours, typ)
		}
		return out, nil
	case "dns_enable":
		on, ok := args["enabled"].(bool)
		if !ok {
			return nil, fmt.Errorf("аргумент enabled обязателен (boolean)")
		}
		return a.SetDnsConfig(map[string]interface{}{"enabled": on}), nil
	case "proxy_enable":
		on, ok := args["enabled"].(bool)
		if !ok {
			return nil, fmt.Errorf("аргумент enabled обязателен (boolean)")
		}
		return a.SetSocksEnabled(on), nil
	case "get_logs":
		lines := 50
		if v, ok := args["lines"].(float64); ok && v > 0 && v <= 500 {
			lines = int(v)
		}
		return a.GetAllLogs("", lines, 0), nil
	}
	return nil, fmt.Errorf("неизвестный инструмент: %s", name)
}

// ---------- биндинги модалки ----------

// GetMcpInfo — состояние MCP-сервера для модалки: тумблер, порт, ключ,
// готовый JSON-конфиг для агента, активные сессии.
func (a *App) GetMcpInfo() map[string]interface{} {
	c := a.cfg.GetMcpConfig()
	running := a.mcp != nil && a.mcp.Running()
	port := c.Port
	if running {
		port = a.mcp.Port()
	}
	sessions := []map[string]interface{}{}
	if a.mcp != nil {
		for _, s := range a.mcp.Sessions() {
			sessions = append(sessions, map[string]interface{}{
				"name":     s.Name,
				"addr":     s.Addr,
				"since":    s.Since.Format("02.01 15:04"),
				"last":     s.Last.Format("15:04:05"),
				"requests": s.Requests,
			})
		}
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/mcp", port)
	cfg := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"zpui": map[string]interface{}{
				"url":     url,
				"headers": map[string]string{"Authorization": "Bearer " + c.Key},
			},
		},
	}
	cfgJSON, _ := json.MarshalIndent(cfg, "", "  ")
	lastErr := ""
	if a.mcp != nil {
		lastErr = a.mcp.LastError()
	}
	skillPath, err := mcpserver.MaterializeSkill(a.exeDir)
	if err != nil {
		a.log.Warn("mcp", "skill materialize: "+err.Error())
	}
	return okRespWithData(map[string]interface{}{
		"enabled":     c.Enabled,
		"running":     running,
		"port":        port,
		"key":         c.Key,
		"url":         url,
		"config_json": string(cfgJSON),
		"sessions":    sessions,
		"last_error":  lastErr,
		"skill_path":  skillPath,
		"skill_dir":   filepath.Dir(skillPath),
	})
}

// OpenMcpSkillFolder — открыть папку скилла в проводнике с выделенным
// SKILL.md (POST /api/mcp/open-skill-folder).
func (a *App) OpenMcpSkillFolder() map[string]interface{} {
	path, err := mcpserver.MaterializeSkill(a.exeDir)
	if err != nil {
		a.log.Warn("mcp", "skill materialize: "+err.Error())
		return errResp(err.Error())
	}
	if err := executil.HiddenCmd("explorer.exe", "/select,\""+path+"\"").Start(); err != nil {
		return errResp(err.Error())
	}
	return okRespWithData(map[string]interface{}{"path": path})
}

// SetMcpEnabled — тумблер сервера (POST /api/mcp/enabled {on}).
func (a *App) SetMcpEnabled(on bool) map[string]interface{} {
	if err := a.cfg.SetMcpEnabled(on); err != nil {
		return errResp(err.Error())
	}
	if on {
		if err := a.mcpStart(); err != nil {
			m := okResp()
			m["warning"] = err.Error()
			return m
		}
	} else if a.mcp != nil {
		a.mcp.Stop()
	}
	a.log.Info("mcp", "enabled: "+map[bool]string{true: "on", false: "off"}[on])
	return okResp()
}

// SetMcpPort — порт сервера; работающий сервер перезапускается.
func (a *App) SetMcpPort(port int) map[string]interface{} {
	if port < 1 || port > 65535 {
		return errResp("порт должен быть 1..65535")
	}
	if port == a.cfg.GetMcpConfig().Port {
		return okResp()
	}
	if err := a.cfg.SetMcpPort(port); err != nil {
		return errResp(err.Error())
	}
	if a.mcp != nil && a.mcp.Running() {
		if err := a.mcpStart(); err != nil {
			m := okResp()
			m["warning"] = err.Error()
			return m
		}
	}
	return okResp()
}

// RegenerateMcpKey — новый ключ: старые конфиги агентов сразу перестают
// работать, сессии сбрасываются.
func (a *App) RegenerateMcpKey() map[string]interface{} {
	key := a.cfg.RegenerateMcpKey()
	if a.mcp != nil {
		a.mcp.ClearSessions()
	}
	a.log.Info("mcp", "key regenerated")
	return okRespWithData(map[string]interface{}{"key": key})
}

// ---------- жизненный цикл ----------

// mcpStart — запуск с авто-сдвигом порта +10 при занятости (до 10 попыток),
// выбранный порт сохраняется в config.json.
func (a *App) mcpStart() error {
	if a.mcp == nil {
		a.mcp = a.buildMcpServer()
	}
	port := a.cfg.GetMcpConfig().Port
	try := port
	var err error
	for i := 0; i < 10; i++ {
		if err = a.mcp.Start(try); err == nil {
			break
		}
		try += 10
	}
	if err != nil {
		a.log.Warn("mcp", "start failed: "+err.Error())
		return err
	}
	if try != port {
		_ = a.cfg.SetMcpPort(try)
		a.log.Info("mcp", "port " + strconv.Itoa(port) + " busy, moved to " + strconv.Itoa(try))
	}
	return nil
}

// startMcp — стартовый хук: ключ генерируется при первом чтении конфига.
func (a *App) startMcp() {
	if _, err := mcpserver.MaterializeSkill(a.exeDir); err != nil {
		a.log.Warn("mcp", "skill materialize: "+err.Error())
	}
	if !a.cfg.GetMcpConfig().Enabled {
		return
	}
	_ = a.mcpStart()
}
