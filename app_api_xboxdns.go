package main

// ============================================================
// XBOX DNS CONFIG
// ============================================================

func (a *App) GetXboxDnsConfig() interface{} {
	return a.cfg.GetXboxDnsConfig()
}

func (a *App) SetXboxDnsConfig(opts map[string]interface{}) map[string]interface{} {
	cfg := a.cfg.GetXboxDnsConfig()
	wasEnabled := cfg.Enabled
	if v, ok := opts["enabled"].(bool); ok {
		cfg.Enabled = v
	}
	if v, ok := opts["primary_dns"].(string); ok {
		cfg.PrimaryDNS = v
	}
	if v, ok := opts["secondary_dns"].(string); ok {
		cfg.SecondaryDNS = v
	}
	a.cfg.SetXboxDnsConfig(cfg)
	a.log.Info("xbox_dns", "Xbox DNS config saved")

	if cfg.Enabled && !wasEnabled {
		a.xboxDns.Configure(cfg.PrimaryDNS, cfg.SecondaryDNS)
		go func() {
			if err := a.xboxDns.Enable(); err != nil {
				a.log.Error("xbox_dns", "Enable failed: "+err.Error())
			}
		}()
	} else if !cfg.Enabled && wasEnabled {
		go func() {
			if err := a.xboxDns.Disable(); err != nil {
				a.log.Error("xbox_dns", "Disable failed: "+err.Error())
			}
		}()
	}

	return map[string]interface{}{"status": "ok"}
}

func (a *App) ToggleXboxDns(enabled bool) map[string]interface{} {
	cfg := a.cfg.GetXboxDnsConfig()
	wasEnabled := cfg.Enabled
	cfg.Enabled = enabled
	a.cfg.SetXboxDnsConfig(cfg)

	if enabled && !wasEnabled {
		a.xboxDns.Configure(cfg.PrimaryDNS, cfg.SecondaryDNS)
		go func() {
			if err := a.xboxDns.Enable(); err != nil {
				a.log.Error("xbox_dns", "Enable failed: "+err.Error())
			}
		}()
		return map[string]interface{}{"status": "starting"}
	} else if !enabled && wasEnabled {
		go func() {
			if err := a.xboxDns.Disable(); err != nil {
				a.log.Error("xbox_dns", "Disable failed: "+err.Error())
			}
		}()
		return map[string]interface{}{"status": "stopping"}
	}
	return map[string]interface{}{"status": "nochange"}
}
