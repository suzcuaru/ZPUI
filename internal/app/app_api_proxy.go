package app

import "zpui/internal/monitor"

// ============================================================
// PROXY
// ============================================================

func (a *App) ProxyStart() map[string]interface{} {
	if err := a.proxy.Start(); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"status": "started"}
}

func (a *App) ProxyStop() map[string]interface{} {
	a.proxy.Stop()
	return map[string]interface{}{"status": "stopped"}
}

func (a *App) GetProxyStatus() map[string]interface{} {
	pcfg := a.cfg.GetProxyConfig()
	return map[string]interface{}{
		"running":    a.proxy.IsRunning(),
		"port":       pcfg.Port,
		"username":   pcfg.Username,
		"auto_start": pcfg.AutoStart,
		"stats":      a.proxy.GetStats(),
	}
}

func (a *App) GetProxyConnections() map[string]interface{} {
	conns := a.proxy.GetConnections()
	byClient := a.proxy.GetActiveConnectionsByClient()
	arpTable := getARPTable()

	deviceInfo := make(map[string]map[string]string)
	for ip := range byClient {
		deviceInfo[ip] = map[string]string{
			"hostname": resolveHostname(ip),
			"mac":      arpTable[ip],
		}
	}

	return map[string]interface{}{
		"connections": conns,
		"by_client":   byClient,
		"device_info": deviceInfo,
	}
}

func (a *App) GetProxyConfig() interface{} {
	return a.cfg.GetProxyConfig()
}

func (a *App) SetProxyConfig(opts map[string]interface{}) map[string]interface{} {
	a.log.Info("proxy", "SetProxyConfig called")
	pcfg := a.cfg.GetProxyConfig()
	if port, ok := opts["port"].(float64); ok {
		pcfg.Port = int(port)
	}
	if user, ok := opts["username"].(string); ok {
		pcfg.Username = user
	}
	if pass, ok := opts["password"].(string); ok {
		pcfg.Password = pass
	}
	if as, ok := opts["auto_start"].(bool); ok {
		pcfg.AutoStart = as
	}
	if bh, ok := opts["bind_host"].(string); ok {
		pcfg.BindHost = bh
	}
	a.cfg.SetProxyConfig(pcfg)
	if err := a.cfg.Save(); err != nil {
		a.log.Error("proxy", "Save error: "+err.Error())
	} else {
		a.log.Info("proxy", "Proxy config saved to JSON")
	}
	return map[string]interface{}{"status": "ok"}
}

func (a *App) GetProxyQRCode() map[string]interface{} {
	pcfg := a.cfg.GetProxyConfig()
	ips := getLocalIPs()
	if len(ips) == 0 {
		ips = []string{"127.0.0.1"}
	}
	return map[string]interface{}{
		"ips":      ips,
		"port":     pcfg.Port,
		"username": pcfg.Username,
		"password": pcfg.Password,
	}
}

// ============================================================
// MONITOR
// ============================================================

func (a *App) GetTraffic() map[string]interface{} {
	stats := a.monitor.GetCurrentStats()
	return map[string]interface{}{
		"download_bytes": stats.DownloadBytes,
		"upload_bytes":   stats.UploadBytes,
		"download_speed": stats.DownloadSpeed,
		"upload_speed":   stats.UploadSpeed,
		"download_fmt":   monitor.FormatBytes(stats.DownloadBytes),
		"upload_fmt":     monitor.FormatBytes(stats.UploadBytes),
		"dl_speed_fmt":   monitor.FormatSpeed(stats.DownloadSpeed),
		"ul_speed_fmt":   monitor.FormatSpeed(stats.UploadSpeed),
		"timestamp":      stats.Timestamp,
	}
}

func (a *App) GetMonitorDevices() map[string]interface{} {
	byClient := a.proxy.GetActiveConnectionsByClient()
	arpTable := getARPTable()

	type DeviceInfo struct {
		IP          string `json:"ip"`
		Hostname    string `json:"hostname"`
		MAC         string `json:"mac"`
		Connections int    `json:"connections"`
	}
	var devices []DeviceInfo
	for ip, conns := range byClient {
		hostname := resolveHostname(ip)
		mac := arpTable[ip]
		devices = append(devices, DeviceInfo{
			IP:          ip,
			Hostname:    hostname,
			MAC:         mac,
			Connections: len(conns),
		})
	}
	if devices == nil {
		devices = []DeviceInfo{}
	}
	return map[string]interface{}{"devices": devices}
}
