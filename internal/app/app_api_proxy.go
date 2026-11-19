package app

// ============================================================
// MONITOR (трафик); SOCKS5-прокси выведен из продукта.
// ============================================================

func (a *App) GetMonitorDevices() map[string]interface{} {
	tgByClient := a.tgproxy.ClientsByIP()
	arpTable := getARPTable()

	type DeviceInfo struct {
		IP            string `json:"ip"`
		Hostname      string `json:"hostname"`
		MAC           string `json:"mac"`
		TgConnections int    `json:"tg_connections"`
	}

	var devices []DeviceInfo
	for ip := range tgByClient {
		devices = append(devices, DeviceInfo{
			IP:            ip,
			Hostname:      resolveHostname(ip),
			MAC:           arpTable[ip],
			TgConnections: tgByClient[ip],
		})
	}
	if devices == nil {
		devices = []DeviceInfo{}
	}
	return map[string]interface{}{"devices": devices}
}
