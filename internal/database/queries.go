package database

import (
	"database/sql"
	"fmt"
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
	rows, err := DB().Query(`
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

// CleanOldConnections удаляет соединения старше duration
func CleanOldConnections(maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)
	_, err := DB().Exec(`DELETE FROM device_connections WHERE closed_at IS NOT NULL AND closed_at < ?`, cutoff)
	return err
}

// === Action Logs ===

// InsertActionLog записывает лог действия
func InsertActionLog(l *ActionLog) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	if l.Timestamp.IsZero() {
		l.Timestamp = time.Now()
	}
	_, err := DB().Exec(`
		INSERT INTO action_logs (id, timestamp, category, action, details)
		VALUES (?, ?, ?, ?, ?)
	`, l.ID, l.Timestamp, l.Category, l.Action, l.Details)
	return err
}

// GetActionLogs возвращает логи с фильтрами
func GetActionLogs(category string, limit, offset int) ([]ActionLog, error) {
	if limit <= 0 {
		limit = 100
	}

	var rows *sql.Rows
	var err error

	if category != "" {
		rows, err = DB().Query(`
			SELECT id, timestamp, category, action, details
			FROM action_logs
			WHERE category = ?
			ORDER BY timestamp DESC
			LIMIT ? OFFSET ?
		`, category, limit, offset)
	} else {
		rows, err = DB().Query(`
			SELECT id, timestamp, category, action, details
			FROM action_logs
			ORDER BY timestamp DESC
			LIMIT ? OFFSET ?
		`, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []ActionLog
	for rows.Next() {
		var l ActionLog
		var details sql.NullString
		if err := rows.Scan(&l.ID, &l.Timestamp, &l.Category, &l.Action, &details); err != nil {
			return nil, err
		}
		if details.Valid {
			l.Details = details.String
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// BatchInsertActionLogs пакетная вставка логов
func BatchInsertActionLogs(logs []ActionLog) error {
	if len(logs) == 0 {
		return nil
	}

	tx, err := DB().Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO action_logs (id, timestamp, category, action, details) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, l := range logs {
		if l.ID == "" {
			l.ID = uuid.New().String()
		}
		if l.Timestamp.IsZero() {
			l.Timestamp = time.Now()
		}
		if _, err := stmt.Exec(l.ID, l.Timestamp, l.Category, l.Action, l.Details); err != nil {
			return fmt.Errorf("exec: %w", err)
		}
	}

	return tx.Commit()
}

// === Zapret Backup ===

// SaveZapretBackup сохраняет JSON-слепок состояния zapret (перезапись).
func SaveZapretBackup(data string) error {
	_, err := DB().Exec(`
		INSERT INTO zapret_backup (id, data, updated_at)
		VALUES (1, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET data = excluded.data, updated_at = CURRENT_TIMESTAMP
	`, data)
	return err
}

// GetZapretBackup возвращает сохранённый слепок состояния zapret.
func GetZapretBackup() (string, error) {
	var data string
	err := DB().QueryRow(`SELECT data FROM zapret_backup WHERE id = 1`).Scan(&data)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return data, err
}

// DeleteZapretBackup удаляет слепок состояния zapret.
func DeleteZapretBackup() error {
	_, err := DB().Exec(`DELETE FROM zapret_backup WHERE id = 1`)
	return err
}