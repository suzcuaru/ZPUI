package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"


	"zpui/internal/database"
	"zpui/internal/executil"
)


// ============================================================
// DEVICE API METHODS
// ============================================================

// GetDevices — все устройства сессии (замена GET /api/devices).
func (a *App) GetDevices() map[string]interface{} {
	devices, err := database.GetAllDevices()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	if devices == nil {
		devices = []database.SessionDevice{}
	}
	return map[string]interface{}{
		"devices": devices,
		"count":   len(devices),
	}
}

// GetDevice — детали устройства (замена GET /api/devices/{mac}).
func (a *App) GetDevice(mac string) map[string]interface{} {
	mac = strings.TrimSpace(mac)
	if mac == "" {
		return map[string]interface{}{"error": "mac required"}
	}

	device, err := database.GetDeviceByMAC(mac)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	if device == nil {
		return map[string]interface{}{"error": "device not found"}
	}

	conns, _ := database.GetDeviceConnections(mac, 20, 0)
	if conns == nil {
		conns = []database.DeviceConnection{}
	}

	return map[string]interface{}{
		"device":      device,
		"connections": conns,
	}
}

// GetDeviceConnections — соединения устройства (замена GET /api/devices/{mac}/connections).
func (a *App) GetDeviceConnections(mac string, limit int, offset int) map[string]interface{} {
	if limit <= 0 {
		limit = 50
	}

	conns, err := database.GetDeviceConnections(mac, limit, offset)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	if conns == nil {
		conns = []database.DeviceConnection{}
	}
	return map[string]interface{}{
		"connections": conns,
		"count":       len(conns),
	}
}

// PingDevice — пинг устройства (замена POST /api/devices/{mac}/ping).
func (a *App) PingDevice(mac string) map[string]interface{} {
	device, err := database.GetDeviceByMAC(mac)
	if err != nil || device == nil {
		return map[string]interface{}{"error": "device not found"}
	}

	ip := device.IP
	if ip == "" {
		return map[string]interface{}{"error": "device has no IP"}
	}

	start := time.Now()
	cmd := executil.HiddenCmd("ping", "-n", "4", "-w", "1000", ip)
	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	result := map[string]interface{}{
		"mac":       mac,
		"ip":        ip,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	if err != nil {
		result["success"] = false
		result["error"] = "ping failed"
		result["avg_ms"] = -1
	} else {
		avgMs := parsePingAvg(string(output))
		result["success"] = true
		result["avg_ms"] = avgMs
		result["duration_ms"] = duration.Milliseconds()
		result["output"] = string(output)
	}

	database.InsertActionLog(&database.ActionLog{
		Category: "device",
		Action:   "ping",
		Details:  fmt.Sprintf(`{"mac":"%s","ip":"%s","avg_ms":%v}`, mac, ip, result["avg_ms"]),
	})

	return result
}

// DeleteDevice — удалить устройство (замена DELETE /api/devices/{mac}).
func (a *App) DeleteDevice(mac string) map[string]interface{} {
	mac = strings.TrimSpace(mac)
	if mac == "" {
		return map[string]interface{}{"error": "mac required"}
	}

	if err := database.DeleteDevice(mac); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	database.InsertActionLog(&database.ActionLog{
		Category: "device",
		Action:   "delete",
		Details:  fmt.Sprintf(`{"mac":"%s"}`, mac),
	})

	return map[string]interface{}{"status": "ok"}
}

func (a *App) saveTrafficSnapshot(dlSpeed, ulSpeed float64, totalDL, totalUL int64, connCount int) {
	database.InsertSnapshot(&database.TrafficSnapshot{
		Timestamp: time.Now(),
		DLSpeed:   dlSpeed,
		ULSpeed:   ulSpeed,
		TotalDL:   totalDL,
		TotalUL:   totalUL,
		ConnCount: connCount,
	})
}

// ============================================================
// DATA ROTATION HELPERS
// ============================================================

func cleanOldSnapshots(maxAge time.Duration) {
	database.CleanOldSnapshots(maxAge)
}

func cleanOldConnections(maxAge time.Duration) {
	database.CleanOldConnections(maxAge)
}

// ============================================================
// PING PARSER
// ============================================================

// parsePingAvg парсит среднее время пинга из вывода Windows ping.
func parsePingAvg(output string) float64 {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Average") || strings.Contains(line, "Среднее") {
			parts := strings.Split(line, ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if strings.Contains(p, "Average") || strings.Contains(p, "Среднее") {
					valStr := p[strings.Index(p, "=")+1:]
					valStr = strings.TrimSpace(valStr)
					valStr = strings.TrimSuffix(valStr, "ms")
					valStr = strings.TrimSuffix(valStr, "мс")
					valStr = strings.TrimSpace(valStr)
					if val, err := strconv.ParseFloat(valStr, 64); err == nil {
						return val
					}
				}
			}
		}
	}
	return -1
}
