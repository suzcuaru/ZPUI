package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/database"
	"zpui/internal/executil"
	"zpui/internal/notify"
	"zpui/internal/zapret"
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

// ============================================================
// ACTION LOGS
// ============================================================

// GetActionLogs — логи действий (замена GET /api/logs/actions).
func (a *App) GetActionLogs(category string, limit int, offset int) map[string]interface{} {
	logs, err := database.GetActionLogs(category, limit, offset)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	if logs == nil {
		logs = []database.ActionLog{}
	}
	return map[string]interface{}{
		"logs":  logs,
		"count": len(logs),
	}
}

// FrontendLogs — batch логов от фронтенда (замена POST /api/logs/frontend).
func (a *App) FrontendLogs(events []interface{}) map[string]interface{} {
	if len(events) > 100 {
		events = events[:100]
	}

	var logs []database.ActionLog
	for _, ev := range events {
		event, ok := ev.(map[string]interface{})
		if !ok {
			continue
		}

		details, _ := json.Marshal(map[string]interface{}{
			"category": event["category"],
			"action":   event["action"],
			"details":  event["details"],
		})

		logs = append(logs, database.ActionLog{
			Category: "frontend",
			Action:   fmt.Sprintf("%v", event["action"]),
			Details:  string(details),
		})
	}

	if err := database.BatchInsertActionLogs(logs); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	return map[string]interface{}{
		"status": "ok",
		"count":  len(logs),
	}
}

// ============================================================
// TRAFFIC SNAPSHOTS
// ============================================================

// GetTrafficSnapshots — снапшоты трафика (замена GET /api/monitor/snapshots).
func (a *App) GetTrafficSnapshots(minutes int) map[string]interface{} {
	if minutes <= 0 {
		minutes = 30
	}

	since := time.Now().Add(-time.Duration(minutes) * time.Minute)
	snaps, err := database.GetSnapshots(since)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	if snaps == nil {
		snaps = []database.TrafficSnapshot{}
	}
	return map[string]interface{}{
		"snapshots": snaps,
		"count":     len(snaps),
	}
}

func (a *App) notifyOnTestDone() {
	if a.cfg.NotifyStrategyTest {
		lang := a.cfg.GetLanguage()
		notify.Show("ZPUI", tr(lang, "test_complete"))
	}
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
// STREAMING (SSE → Wails Events)
// ============================================================

// RunAutoTestStream — запуск автотеста с потоковой передачей результатов.
// Эмитит события: strategy:event (каждый результат), strategy:done (завершение).
func (a *App) RunAutoTestStream() {
	if a.zapret.IsAutoTestRunning() {
		runtime.EventsEmit(a.ctx, "strategy:done", map[string]interface{}{"error": "Автотест уже запущен"})
		return
	}

	runtime.EventsEmit(a.ctx, "strategy:testing", true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results := make(chan zapret.AutoTestResult, 50)
	done := make(chan struct{})
	go a.zapret.RunAutoTest(ctx, results, done)

	for {
		select {
		case result, ok := <-results:
			if !ok {
				a.notifyOnTestDone()
				runtime.EventsEmit(a.ctx, "strategy:done", map[string]interface{}{})
				runtime.EventsEmit(a.ctx, "strategy:testing", false)
				return
			}
			runtime.EventsEmit(a.ctx, "strategy:event", result)
		case <-done:
			a.notifyOnTestDone()
			runtime.EventsEmit(a.ctx, "strategy:done", map[string]interface{}{})
			runtime.EventsEmit(a.ctx, "strategy:testing", false)
			return
		}
	}
}

// RunAutoSelectStream — автоподбор лучшей стратегии с применением.
// Эмитит события: autoselect:event (каждый результат), autoselect:done (завершение).
func (a *App) RunAutoSelectStream() {
	if a.zapret.IsAutoTestRunning() {
		runtime.EventsEmit(a.ctx, "autoselect:done", map[string]interface{}{"error": "Подбор уже запущен"})
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	results := make(chan zapret.AutoTestResult, 50)
	done := make(chan struct{})
	go a.zapret.AutoSelectAndApply(ctx, results, done)

	for {
		select {
		case result, ok := <-results:
			if !ok {
				runtime.EventsEmit(a.ctx, "autoselect:done", map[string]interface{}{})
				return
			}
			runtime.EventsEmit(a.ctx, "autoselect:event", result)
		case <-done:
			runtime.EventsEmit(a.ctx, "autoselect:done", map[string]interface{}{})
			return
		}
	}
}

// RunUpdateStream — запуск обновления с потоковой передачей прогресса.
// Эмитит события: update:progress (каждый шаг), update:done (завершение).
func (a *App) RunUpdateStream() {
	progress := make(chan zapret.UpdateProgress, 20)
	go a.zapret.PerformUpdate(progress)

	for p := range progress {
		runtime.EventsEmit(a.ctx, "update:progress", p)
	}
	runtime.EventsEmit(a.ctx, "update:done", nil)
}

// ============================================================
// DEVICE TRACKER (background workers)
// ============================================================

// startDeviceTracker — периодическое обновление устройств из ARP-таблицы.
func (a *App) startDeviceTracker() {
	// Первоначальное сканирование
	a.updateDevicesFromNetwork()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			a.updateDevicesFromNetwork()
		case <-a.stopCh:
			return
		}
	}
}

func (a *App) updateDevicesFromNetwork() {
	arp := getARPTable()

	for ip, mac := range arp {
		mac = strings.ReplaceAll(strings.ToLower(mac), "-", ":")
		if mac == "" || len(mac) < 17 {
			continue
		}

		hostname := resolveHostname(ip)

		now := time.Now()
		device := &database.SessionDevice{
			MAC:      mac,
			IP:       ip,
			Hostname: hostname,
			LastSeen: now,
			IsOnline: true,
		}

		existing, _ := database.GetDeviceByMAC(mac)
		if existing != nil {
			device.ID = existing.ID
			device.FirstSeen = existing.FirstSeen
			device.TotalDL = existing.TotalDL
			device.TotalUL = existing.TotalUL
		} else {
			device.FirstSeen = now
		}

		database.UpsertDevice(device)
	}

	// Помечаем устройства не в ARP как офлайн
	devices, _ := database.GetAllDevices()
	for _, d := range devices {
		found := false
		for _, mac := range arp {
			normalMac := strings.ReplaceAll(strings.ToLower(mac), "-", ":")
			if normalMac == d.MAC {
				found = true
				break
			}
		}
		if !found && d.IsOnline {
			database.UpsertDevice(&database.SessionDevice{
				ID:        d.ID,
				MAC:       d.MAC,
				IP:        d.IP,
				Hostname:  d.Hostname,
				FirstSeen: d.FirstSeen,
				LastSeen:  d.LastSeen,
				TotalDL:   d.TotalDL,
				TotalUL:   d.TotalUL,
				IsOnline:  false,
			})
		}
	}
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
