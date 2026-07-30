package app

import (
	"fmt"
)

// ============================================================
// XBOX DNS (системный DNS через netsh)
// ============================================================

func (a *App) GetXboxDnsConfig() interface{} {
	return a.cfg.GetXboxDnsConfig()
}

// applyDnsEnabled переключает системный DNS через netsh:
// включено — primary/secondary DNS на всех адаптерах;
// выключено — восстановление DHCP.
func (a *App) applyDnsEnabled(enabled bool) error {
	cfg := a.cfg.GetXboxDnsConfig()
	a.xboxDns.Configure(cfg.PrimaryDNS, cfg.SecondaryDNS)

	if enabled {
		if err := a.xboxDns.Enable(); err != nil {
			return fmt.Errorf("system dns: %w", err)
		}
		a.log.Info("dns", "Xbox DNS enabled")
		return nil
	}

	if err := a.xboxDns.Disable(); err != nil {
		a.log.Warn("dns", "DHCP restore: "+err.Error())
	}
	a.log.Info("dns", "Xbox DNS disabled")
	return nil
}

func (a *App) SetXboxDnsConfig(opts map[string]interface{}) map[string]interface{} {
	cfg := a.cfg.GetXboxDnsConfig()
	wasEnabled := cfg.Enabled

	if v, ok := opts["enabled"].(bool); ok {
		cfg.Enabled = v
	}
	if v, ok := opts["xbox_enabled"].(bool); ok {
		cfg.XboxEnabled = v
	}
	if v, ok := opts["primary_dns"].(string); ok && v != "" {
		cfg.PrimaryDNS = v
	}
	if v, ok := opts["secondary_dns"].(string); ok && v != "" {
		cfg.SecondaryDNS = v
	}
	a.cfg.SetXboxDnsConfig(cfg)
	a.log.Info("dns", "DNS config saved")

	if err := a.applyDnsEnabled(cfg.Enabled); err != nil {
		if cfg.Enabled && !wasEnabled {
			if err2 := a.cfg.SetXboxDnsEnabled(false); err2 != nil {
				a.log.Warn("dns", "rollback SetXboxDnsEnabled: "+err2.Error())
			}
		}
		return errResp("DNS: " + err.Error())
	}
	return okResp()
}

func (a *App) ToggleXboxDns(enabled bool) map[string]interface{} {
	if err := a.cfg.SetXboxDnsEnabled(enabled); err != nil {
		a.log.Warn("dns", "SetXboxDnsEnabled: "+err.Error())
	}
	if err := a.applyDnsEnabled(enabled); err != nil {
		if err2 := a.cfg.SetXboxDnsEnabled(!enabled); err2 != nil {
			a.log.Warn("dns", "rollback SetXboxDnsEnabled: "+err2.Error())
		}
		return errResp("DNS: " + err.Error())
	}
	return okResp()
}
