package app

import (
	"strings"
)

// ============================================================
// TELEGRAM MTPROTO PROXY (встроенный мост на Go)
// ============================================================

func (a *App) TgProxyStart() map[string]interface{} {
	if err := a.tgproxy.Start(); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"status": "started"}
}

func (a *App) TgProxyStop() map[string]interface{} {
	a.tgproxy.Stop()
	return map[string]interface{}{"status": "stopped"}
}

func (a *App) GetTgProxyStatus() map[string]interface{} {
	dd, ee := a.tgproxy.GetLinks()
	return okRespWithData(map[string]interface{}{
		"running":   a.tgproxy.IsRunning(),
		"status":    a.tgproxy.GetStatus(),
		"installed": a.tgproxy.ComponentInstalled(),
		"version":   a.tgproxy.Version(),
		"stats":     a.tgproxy.GetStats(),
		"link":      dd,
		"link_ee":   ee,
	})
}

func (a *App) GetTgProxyConfig() interface{} {
	return a.cfg.GetTgProxyConfig()
}

func (a *App) SetTgProxyConfig(opts map[string]interface{}) map[string]interface{} {
	a.log.Info("tgproxy", "SetTgProxyConfig called")
	pc := a.cfg.GetTgProxyConfig()
	if v, ok := opts["enabled"].(bool); ok {
		pc.Enabled = v
	}
	if v, ok := opts["auto_start"].(bool); ok {
		pc.AutoStart = v
	}
	if port, ok := opts["port"].(float64); ok {
		pc.Port = int(port)
	}
	if bh, ok := opts["bind_host"].(string); ok {
		pc.BindHost = bh
	}
	if s, ok := opts["secret"].(string); ok && s != "" {
		pc.Secret = s
	}
	if s, ok := opts["fake_tls_domain"].(string); ok {
		pc.FakeTLSDomain = s
	}
	if v, ok := opts["proxy_protocol"].(bool); ok {
		pc.ProxyProtocol = v
	}
	if v, ok := opts["fallback_cf"].(bool); ok {
		pc.FallbackCF = v
	}
	if list, ok := opts["cf_user_domains"].([]interface{}); ok {
		pc.CFUserDomains = ifaceToStrings(list)
	} else if s, ok := opts["cf_user_domains"].(string); ok {
		pc.CFUserDomains = splitDomains(s)
	}
	if list, ok := opts["cf_worker_domains"].([]interface{}); ok {
		pc.CFWorkerDomains = ifaceToStrings(list)
	} else if s, ok := opts["cf_worker_domains"].(string); ok {
		pc.CFWorkerDomains = splitDomains(s)
	}
	if v, ok := opts["no_secure"].(bool); ok {
		pc.NoSecure = v
	}
	if list, ok := opts["dc_ip"].([]interface{}); ok {
		pc.DCIP = ifaceToStrings(list)
	}
	if v, ok := opts["no_h2"].(bool); ok {
		pc.NoH2 = v
	}
	if v, ok := opts["force_test_dc"].(bool); ok {
		pc.ForceTestDC = v
	}
	if v, ok := opts["verbose"].(bool); ok {
		pc.Verbose = v
	}
	if v, ok := opts["buf_kb"].(float64); ok {
		pc.BufKB = int(v)
	}
	if v, ok := opts["pool_size"].(float64); ok {
		pc.PoolSize = int(v)
	}
	if err := a.cfg.SetTgProxyConfig(pc); err != nil {
		a.log.Error("tgproxy", "Save error: "+err.Error())
		return errResp("save: " + err.Error())
	}
	// Применяем новый порт/bind/настройки «на лету», если модуль запущен.
	if a.tgproxy.IsRunning() {
		if err := a.tgproxy.Restart(); err != nil {
			a.log.Warn("tgproxy", "apply-on-the-fly restart failed: "+err.Error())
		}
	}
	return okResp()
}

func ifaceToStrings(list []interface{}) []string {
	out := make([]string, 0, len(list))
	for _, v := range list {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// splitDomains разбирает строку с доменами (запятая/пробел/точка с запятой).
func splitDomains(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t' || r == '\r'
	}) {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// RegenerateTgProxySecret генерирует новый secret (и перезапускает мост, если он работал).
func (a *App) RegenerateTgProxySecret() map[string]interface{} {
	secret, err := a.tgproxy.RegenerateSecret()
	if err != nil {
		return errResp(err.Error())
	}
	return okRespWithData(map[string]interface{}{
		"secret": secret,
		"link":   a.tgproxy.GetLink(),
	})
}
