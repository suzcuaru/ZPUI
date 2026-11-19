package database

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)


// === Session Devices ===

// UpsertDevice создаёт или обновляет устройство
func UpsertDevice(d *SessionDevice) error {
	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	_, err := DB().Exec(`
		INSERT INTO session_devices (id, mac, ip, hostname, first_seen, last_seen, total_dl, total_ul, is_online)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(mac) DO UPDATE SET
			ip = excluded.ip,
			hostname = excluded.hostname,
			last_seen = excluded.last_seen,
			total_dl = excluded.total_dl,
			total_ul = excluded.total_ul,
			is_online = excluded.is_online
	`, d.ID, d.MAC, d.IP, d.Hostname, d.FirstSeen, d.LastSeen, d.TotalDL, d.TotalUL, d.IsOnline)
	return err
}

// GetAllDevices возвращает все устройства сессии
func GetAllDevices() ([]SessionDevice, error) {
	rows, err := DB().Query(`SELECT id, mac, ip, hostname, first_seen, last_seen, total_dl, total_ul, is_online FROM session_devices ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []SessionDevice
	for rows.Next() {
		var d SessionDevice
		if err := rows.Scan(&d.ID, &d.MAC, &d.IP, &d.Hostname, &d.FirstSeen, &d.LastSeen, &d.TotalDL, &d.TotalUL, &d.IsOnline); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

// GetDeviceByMAC возвращает устройство по MAC адресу
func GetDeviceByMAC(mac string) (*SessionDevice, error) {
	var d SessionDevice
	err := DB().QueryRow(`SELECT id, mac, ip, hostname, first_seen, last_seen, total_dl, total_ul, is_online FROM session_devices WHERE mac = ?`, mac).
		Scan(&d.ID, &d.MAC, &d.IP, &d.Hostname, &d.FirstSeen, &d.LastSeen, &d.TotalDL, &d.TotalUL, &d.IsOnline)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// DeleteDevice удаляет устройство
func DeleteDevice(mac string) error {
	_, err := DB().Exec(`DELETE FROM session_devices WHERE mac = ?`, mac)
	return err
}

// SetAllDevicesOffline помечает все устройства как офлайн
func SetAllDevicesOffline() error {
	_, err := DB().Exec(`UPDATE session_devices SET is_online = FALSE`)
	return err
}

// ClearDevices очищает таблицу устройств (при старте сессии)
func ClearDevices() error {
	_, err := DB().Exec(`DELETE FROM session_devices`)
	return err
}

// === Device Connections ===

// InsertConnection создаёт запись о соединении
func InsertConnection(c *DeviceConnection) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	_, err := DB().Exec(`
		INSERT INTO device_connections (id, device_id, dst_host, dst_port, bytes_dl, bytes_ul, started_at, closed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID, c.DeviceID, c.DstHost, c.DstPort, c.BytesDL, c.BytesUL, c.StartedAt, c.ClosedAt)
	return err
}

// GetDeviceConnections возвращает соединения устройства с пагинацией
func GetDeviceConnections(deviceID string, limit, offset int) ([]DeviceConnection, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := DB().Query(`
		SELECT id, device_id, dst_host, dst_port, bytes_dl, bytes_ul, started_at, closed_at
		FROM device_connections
		WHERE device_id = (SELECT id FROM session_devices WHERE mac = ? OR id = ?)
		ORDER BY started_at DESC
		LIMIT ? OFFSET ?
	`, deviceID, deviceID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conns []DeviceConnection
	for rows.Next() {
		var c DeviceConnection
		if err := rows.Scan(&c.ID, &c.DeviceID, &c.DstHost, &c.DstPort, &c.BytesDL, &c.BytesUL, &c.StartedAt, &c.ClosedAt); err != nil {
			return nil, err
		}
		conns = append(conns, c)
	}
	return conns, rows.Err()
}

// === Traffic Snapshots ===

// InsertSnapshot сохраняет снапшот трафика
func InsertSnapshot(s *TrafficSnapshot) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	_, err := DB().Exec(`
		INSERT INTO traffic_snapshots (id, timestamp, dl_speed, ul_speed, total_dl, total_ul, conn_count)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, s.ID, s.Timestamp, s.DLSpeed, s.ULSpeed, s.TotalDL, s.TotalUL, s.ConnCount)
	return err
}

// GetSnapshots возвращает снапшоты за указанный период
func GetSnapshots(since time.Time) ([]TrafficSnapshot, error) {
	if db == nil {
		return nil, errors.New("database not initialized")
	}
	rows, err := db.Query(`
		SELECT id, timestamp, dl_speed, ul_speed, total_dl, total_ul, conn_count
		FROM traffic_snapshots
		WHERE timestamp >= ?
		ORDER BY timestamp ASC
	`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snaps []TrafficSnapshot
	for rows.Next() {
		var s TrafficSnapshot
		if err := rows.Scan(&s.ID, &s.Timestamp, &s.DLSpeed, &s.ULSpeed, &s.TotalDL, &s.TotalUL, &s.ConnCount); err != nil {
			return nil, err
		}
		snaps = append(snaps, s)
	}
	return snaps, rows.Err()
}

// CleanOldSnapshots удаляет снапшоты старше duration
func CleanOldSnapshots(maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)
	_, err := DB().Exec(`DELETE FROM traffic_snapshots WHERE timestamp < ?`, cutoff)
	return err
}

// === Yandex Network Metrics ===

// InsertYandexMetric сохраняет замер качества сети (инструменты Яндекса).
// Время локальное (как в traffic_snapshots): чтение и чистка сравнивают с
// time.Now() по месту — UTC-запись на восточных поясах выпадала из окна
// чтения, и футер/графики показывали «—» при свежих данных.
func InsertYandexMetric(pingMs int64, speedMbps float64) error {
	_, err := DB().Exec(`INSERT INTO yandex_metrics (timestamp, ping_ms, speed_mbps) VALUES (?, ?, ?)`,
		time.Now(), pingMs, speedMbps)
	return err
}

// GetYandexMetrics возвращает замеры за период (хронологически).
// Нулевые строки (замер не удался) не отдаются: старые версии приложения
// писали их, и они портили графики и отчёты «пинг 0 / скорость 0».
func GetYandexMetrics(since time.Time, limit int) ([]YandexMetric, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := DB().Query(`
		SELECT timestamp, ping_ms, speed_mbps
		FROM yandex_metrics
		WHERE timestamp >= ? AND (ping_ms > 0 OR speed_mbps > 0)
		ORDER BY timestamp ASC
		LIMIT ?
	`, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []YandexMetric
	for rows.Next() {
		var m YandexMetric
		if err := rows.Scan(&m.Timestamp, &m.PingMs, &m.SpeedMbps); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetLatestYandexMetric — последний НАСТОЯЩИЙ замер (nil — измерений ещё не
// было; нулевые строки от неудачных замеров не считаются).
func GetLatestYandexMetric() (*YandexMetric, error) {
	var m YandexMetric
	err := DB().QueryRow(`
		SELECT timestamp, ping_ms, speed_mbps
		FROM yandex_metrics
		WHERE ping_ms > 0 OR speed_mbps > 0
		ORDER BY timestamp DESC
		LIMIT 1
	`).Scan(&m.Timestamp, &m.PingMs, &m.SpeedMbps)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// CleanOldYandexMetrics удаляет замеры старше duration.
func CleanOldYandexMetrics(maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)
	_, err := DB().Exec(`DELETE FROM yandex_metrics WHERE timestamp < ?`, cutoff)
	return err
}

// CleanOldConnections удаляет соединения старше duration
func CleanOldConnections(maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)
	_, err := DB().Exec(`DELETE FROM device_connections WHERE closed_at IS NOT NULL AND closed_at < ?`, cutoff)
	return err
}
