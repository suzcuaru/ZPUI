package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"zpui/internal/database"
	"zpui/internal/executil"
)

// handleDevicesAPI — роутер для /api/devices/*
func (s *Server) handleDevicesAPI(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// GET /api/devices — список всех устройств
	if path == "/api/devices" {
		if r.Method == "GET" {
			s.handleGetDevices(w, r)
		} else if r.Method == "DELETE" {
			// DELETE /api/devices — очистить все
			s.handleDeleteDevice(w, r)
		} else {
			http.Error(w, "Method not allowed", 405)
		}
		return
	}

	// /api/devices/{mac}/... — подмаршруты
	sub := strings.TrimPrefix(path, "/api/devices/")
	parts := strings.Split(sub, "/")
	mac := parts[0]

	if len(parts) == 1 {
		// GET /api/devices/{mac} — детали устройства
		if r.Method == "GET" {
			r.URL.Path = "/api/devices/" + mac
			s.handleGetDevice(w, r)
		} else if r.Method == "DELETE" {
			r.URL.Path = "/api/devices/" + mac
			s.handleDeleteDevice(w, r)
		} else {
			http.Error(w, "Method not allowed", 405)
		}
		return
	}

	if len(parts) == 2 {
		switch parts[1] {
		case "connections":
			if r.Method == "GET" {
				s.handleGetDeviceConnections(w, r)
			} else {
				http.Error(w, "Method not allowed", 405)
			}
		case "ping":
			if r.Method == "POST" {
				s.handlePingDevice(w, r)
			} else {
				http.Error(w, "Method not allowed", 405)
			}
		default:
			writeJSON(w, map[string]interface{}{"error": "not found"})
		}
		return
	}

	writeJSON(w, map[string]interface{}{"error": "not found"})
}

// handleGetDevices — GET /api/devices — все устройства сессии
func (s *Server) handleGetDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := database.GetAllDevices()
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	if devices == nil {
		devices = []database.SessionDevice{}
	}
	writeJSON(w, map[string]interface{}{
		"devices": devices,
		"count":   len(devices),
	})
}

// handleGetDevice — GET /api/devices/{mac} — детали устройства
func (s *Server) handleGetDevice(w http.ResponseWriter, r *http.Request) {
	mac := strings.TrimPrefix(r.URL.Path, "/api/devices/")
	mac = strings.TrimSpace(mac)
	if mac == "" {
		writeJSON(w, map[string]interface{}{"error": "mac required"})
		return
	}

	device, err := database.GetDeviceByMAC(mac)
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	if device == nil {
		writeJSON(w, map[string]interface{}{"error": "device not found"})
		return
	}

	// Получаем недавние соединения
	conns, _ := database.GetDeviceConnections(mac, 20, 0)
	if conns == nil {
		conns = []database.DeviceConnection{}
	}

	writeJSON(w, map[string]interface{}{
		"device":      device,
		"connections": conns,
	})
}

// handleGetDeviceConnections — GET /api/devices/{mac}/connections
func (s *Server) handleGetDeviceConnections(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	// /api/devices/{mac}/connections
	parts := strings.Split(strings.TrimPrefix(path, "/api/devices/"), "/")
	if len(parts) < 1 {
		writeJSON(w, map[string]interface{}{"error": "mac required"})
		return
	}
	mac := parts[0]

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 50
	}

	conns, err := database.GetDeviceConnections(mac, limit, offset)
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	if conns == nil {
		conns = []database.DeviceConnection{}
	}
	writeJSON(w, map[string]interface{}{
		"connections": conns,
		"count":       len(conns),
	})
}

// handlePingDevice — POST /api/devices/{mac}/ping
func (s *Server) handlePingDevice(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	parts := strings.Split(strings.TrimPrefix(path, "/api/devices/"), "/")
	if len(parts) < 1 {
		writeJSON(w, map[string]interface{}{"error": "mac required"})
		return
	}
	mac := parts[0]

	device, err := database.GetDeviceByMAC(mac)
	if err != nil || device == nil {
		writeJSON(w, map[string]interface{}{"error": "device not found"})
		return
	}

	ip := device.IP
	if ip == "" {
		writeJSON(w, map[string]interface{}{"error": "device has no IP"})
		return
	}

	// Выполняем ping
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
		// Парсим среднее время из вывода ping
		avgMs := parsePingAvg(string(output))
		result["success"] = true
		result["avg_ms"] = avgMs
		result["duration_ms"] = duration.Milliseconds()
		result["output"] = string(output)
	}

	// Логируем действие
	database.InsertActionLog(&database.ActionLog{
		Category: "device",
		Action:   "ping",
		Details:  fmt.Sprintf(`{"mac":"%s","ip":"%s","avg_ms":%v}`, mac, ip, result["avg_ms"]),
	})

	writeJSON(w, result)
}

// handleDeleteDevice — DELETE /api/devices/{mac}
func (s *Server) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	mac := strings.TrimPrefix(r.URL.Path, "/api/devices/")
	mac = strings.TrimSpace(mac)
	if mac == "" {
		writeJSON(w, map[string]interface{}{"error": "mac required"})
		return
	}

	if err := database.DeleteDevice(mac); err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}

	database.InsertActionLog(&database.ActionLog{
		Category: "device",
		Action:   "delete",
		Details:  fmt.Sprintf(`{"mac":"%s"}`, mac),
	})

	writeJSON(w, map[string]interface{}{"status": "ok"})
}

// handleGetActionLogs — GET /api/logs/actions
func (s *Server) handleGetActionLogs(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	logs, err := database.GetActionLogs(category, limit, offset)
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	if logs == nil {
		logs = []database.ActionLog{}
	}
	writeJSON(w, map[string]interface{}{
		"logs":  logs,
		"count": len(logs),
	})
}

// handleFrontendLogs — POST /api/logs/frontend — batch логов от фронтенда
func (s *Server) handleFrontendLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}

	data, _ := readJSON(r)
	eventsRaw, ok := data["events"]
	if !ok {
		writeJSON(w, map[string]interface{}{"error": "events required"})
		return
	}

	eventsSlice, ok := eventsRaw.([]interface{})
	if !ok {
		writeJSON(w, map[string]interface{}{"error": "events must be array"})
		return
	}

	// Лимит 100 событий за запрос
	if len(eventsSlice) > 100 {
		eventsSlice = eventsSlice[:100]
	}

	var logs []database.ActionLog
	for _, ev := range eventsSlice {
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
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}

	writeJSON(w, map[string]interface{}{
		"status": "ok",
		"count":  len(logs),
	})
}

// handleGetTrafficSnapshots — GET /api/monitor/snapshots
func (s *Server) handleGetTrafficSnapshots(w http.ResponseWriter, r *http.Request) {
	minutes, _ := strconv.Atoi(r.URL.Query().Get("minutes"))
	if minutes <= 0 {
		minutes = 30
	}

	since := time.Now().Add(-time.Duration(minutes) * time.Minute)
	snaps, err := database.GetSnapshots(since)
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	if snaps == nil {
		snaps = []database.TrafficSnapshot{}
	}
	writeJSON(w, map[string]interface{}{
		"snapshots": snaps,
		"count":     len(snaps),
	})
}

// parsePingAvg парсит среднее время пинга из вывода Windows ping
func parsePingAvg(output string) float64 {
	// Windows: "Minimum = 1ms, Maximum = 2ms, Average = 1ms"
	// Или: "Минимум = 1мс, Максимум = 2мс, Среднее = 1мс"
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Average") || strings.Contains(line, "Среднее") {
			parts := strings.Split(line, ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if strings.Contains(p, "Average") || strings.Contains(p, "Среднее") {
					// "Average = 1ms" or "Среднее = 1мс"
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

	// Fallback: пробуем найти первое "= Xms"
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "time=") || strings.Contains(line, "время=") {
			continue
		}
	}

	return -1
}

// UpdateDevicesFromNetwork обновляет/devices из сетевых данных
func (s *Server) UpdateDevicesFromNetwork() {
	// Получаем ARP таблицу
	arp := getARPTable()

	// Для каждого IP/MAC в ARP — обновляем БД
	for ip, mac := range arp {
		mac = strings.ReplaceAll(strings.ToLower(mac), "-", ":")
		if mac == "" || len(mac) < 17 {
			continue
		}

		hostname := resolveHostname(ip)

		now := time.Now()
		device := &database.SessionDevice{
			MAC:       mac,
			IP:        ip,
			Hostname:  hostname,
			LastSeen:  now,
			IsOnline:  true,
		}

		// Проверяем существует ли устройство
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

// StartDeviceTracker запускает периодическое обновление устройств
func (s *Server) StartDeviceTracker() {
	// Первоначальное сканирование
	s.UpdateDevicesFromNetwork()

	// Периодическое обновление каждые 10 секунд
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			s.UpdateDevicesFromNetwork()
		}
	}()

	// Сохранение снапшотов трафика каждые 5 секунд
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			stats := s.monitor.GetCurrentStats()
			s.SaveTrafficSnapshot(
				stats.DownloadSpeed,
				stats.UploadSpeed,
				int64(stats.DownloadBytes),
				int64(stats.UploadBytes),
				len(s.proxy.GetConnections()),
			)
		}
	}()

	// Ротация старых данных каждый час
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			database.CleanOldSnapshots(24 * time.Hour)
			database.CleanOldConnections(7 * 24 * time.Hour)
		}
	}()
}

// SaveTrafficSnapshot сохраняет текущий снапшот трафика в БД
func (s *Server) SaveTrafficSnapshot(dlSpeed, ulSpeed float64, totalDL, totalUL int64, connCount int) {
	database.InsertSnapshot(&database.TrafficSnapshot{
		Timestamp: time.Now(),
		DLSpeed:   dlSpeed,
		ULSpeed:   ulSpeed,
		TotalDL:   totalDL,
		TotalUL:   totalUL,
		ConnCount: connCount,
	})
}