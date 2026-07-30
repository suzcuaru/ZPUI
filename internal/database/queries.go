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

// === Availability History ===

// InsertAvailabilitySnapshot сохраняет запись о доступности ресурсов
func InsertAvailabilitySnapshot(r *AvailabilityRecord) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	if r.Timestamp.IsZero() {
		r.Timestamp = time.Now()
	}
	_, err := DB().Exec(`
		INSERT INTO availability_history (id, timestamp, type, total_resources, ok_resources, pct)
		VALUES (?, ?, ?, ?, ?, ?)
	`, r.ID, r.Timestamp, r.Type, r.TotalResources, r.OKResources, r.Pct)
	return err
}

// GetAvailabilityHistory возвращает записи доступности за указанный период, агрегированные по часам
func GetAvailabilityHistory(since time.Time) ([]AvailabilityRecord, error) {
	rows, err := DB().Query(`
		SELECT id, timestamp, type, total_resources, ok_resources, pct
		FROM availability_history
		WHERE timestamp >= ?
		ORDER BY timestamp ASC
	`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []AvailabilityRecord
	for rows.Next() {
		var r AvailabilityRecord
		if err := rows.Scan(&r.ID, &r.Timestamp, &r.Type, &r.TotalResources, &r.OKResources, &r.Pct); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// CleanOldAvailability удаляет записи доступности старше duration
func CleanOldAvailability(maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)
	_, err := DB().Exec(`DELETE FROM availability_history WHERE timestamp < ?`, cutoff)
	return err
}

// === Per-Resource Availability ===

// InsertResourceAvailability сохраняет результат проверки одного ресурса.
func InsertResourceAvailability(r *ResourceAvailability) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	if r.Timestamp.IsZero() {
		r.Timestamp = time.Now()
	}
	var okInt int
	if r.Ok {
		okInt = 1
	}
	_, err := DB().Exec(`
		INSERT INTO resource_availability (id, timestamp, operator_key, host, type, ok, verdict, latency_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, r.ID, r.Timestamp, r.OperatorKey, r.Host, r.Type, okInt, r.Verdict, r.LatencyMs)
	return err
}

// GetResourceAvailabilityHistory возвращает per-resource данные за период.
func GetResourceAvailabilityHistory(since time.Time, host string) ([]ResourceAvailability, error) {
	q := `SELECT id, timestamp, operator_key, host, type, ok, verdict, latency_ms
	      FROM resource_availability WHERE timestamp >= ?`
	args := []interface{}{since}
	if host != "" {
		q += ` AND host = ?`
		args = append(args, host)
	}
	q += ` ORDER BY timestamp ASC`
	rows, err := DB().Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []ResourceAvailability
	for rows.Next() {
		var r ResourceAvailability
		var okInt int
		var verdict sql.NullString
		if err := rows.Scan(&r.ID, &r.Timestamp, &r.OperatorKey, &r.Host, &r.Type, &okInt, &verdict, &r.LatencyMs); err != nil {
			return nil, err
		}
		r.Ok = okInt != 0
		r.Verdict = verdict.String
		records = append(records, r)
	}
	return records, rows.Err()
}

// RollupResourceDaily агрегирует сырые per-resource данные за указанную дату в daily-таблицу.
func RollupResourceDaily(date string) error {
	dayStart, err := time.Parse("2006-01-02", date)
	if err != nil {
		return fmt.Errorf("invalid date %s: %w", date, err)
	}
	dayEnd := dayStart.Add(24 * time.Hour)

	_, err = DB().Exec(`
		INSERT INTO resource_availability_daily (date, operator_key, host, checks_total, checks_ok, pct)
		SELECT
			? AS date,
			operator_key,
			host,
			COUNT(*) AS checks_total,
			SUM(ok) AS checks_ok,
			CAST(SUM(ok) AS REAL) / CAST(COUNT(*) AS REAL) * 100 AS pct
		FROM resource_availability
		WHERE timestamp >= ? AND timestamp < ?
		GROUP BY operator_key, host
		ON CONFLICT(date, operator_key, host) DO UPDATE SET
			checks_total = excluded.checks_total,
			checks_ok = excluded.checks_ok,
			pct = excluded.pct
	`, date, dayStart, dayEnd)
	return err
}

// GetResourceDailyHistory возвращает daily-данные по ресурсам за период.
func GetResourceDailyHistory(host string, days int) ([]ResourceDaily, error) {
	since := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	q := `SELECT date, operator_key, host, checks_total, checks_ok, pct
	      FROM resource_availability_daily WHERE date >= ?`
	args := []interface{}{since}
	if host != "" {
		q += ` AND host = ?`
		args = append(args, host)
	}
	q += ` ORDER BY date ASC, host ASC`
	rows, err := DB().Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []ResourceDaily
	for rows.Next() {
		var r ResourceDaily
		if err := rows.Scan(&r.Date, &r.OperatorKey, &r.Host, &r.ChecksTotal, &r.ChecksOK, &r.Pct); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// CleanOldResourceAvailability удаляет сырые per-resource данные старше duration.
func CleanOldResourceAvailability(maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)
	_, err := DB().Exec(`DELETE FROM resource_availability WHERE timestamp < ?`, cutoff)
	return err
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

// === Component Versions ===

// SaveComponentVersion сохраняет или обновляет версию компонента.
func SaveComponentVersion(v *ComponentVersion) error {
	if v.LocalUpdatedAt.IsZero() {
		v.LocalUpdatedAt = time.Now()
	}
	_, err := DB().Exec(`
		INSERT INTO component_versions (id, installed_version, remote_version, remote_source, remote_updated_at, local_updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			installed_version = excluded.installed_version,
			remote_version = COALESCE(excluded.remote_version, component_versions.remote_version),
			remote_source = COALESCE(excluded.remote_source, component_versions.remote_source),
			remote_updated_at = COALESCE(excluded.remote_updated_at, component_versions.remote_updated_at),
			local_updated_at = excluded.local_updated_at
	`, v.ID, v.InstalledVersion, v.RemoteVersion, v.RemoteSource, v.RemoteUpdatedAt, v.LocalUpdatedAt)
	return err
}

// GetComponentVersion возвращает версию компонента по ID.
func GetComponentVersion(id string) (*ComponentVersion, error) {
	var v ComponentVersion
	err := DB().QueryRow(`
		SELECT id, installed_version, remote_version, remote_source, remote_updated_at, local_updated_at
		FROM component_versions WHERE id = ?
	`, id).Scan(&v.ID, &v.InstalledVersion, &v.RemoteVersion, &v.RemoteSource, &v.RemoteUpdatedAt, &v.LocalUpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// GetAllComponentVersions возвращает все версии компонентов.
func GetAllComponentVersions() ([]ComponentVersion, error) {
	rows, err := DB().Query(`
		SELECT id, installed_version, remote_version, remote_source, remote_updated_at, local_updated_at
		FROM component_versions ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []ComponentVersion
	for rows.Next() {
		var v ComponentVersion
		if err := rows.Scan(&v.ID, &v.InstalledVersion, &v.RemoteVersion, &v.RemoteSource, &v.RemoteUpdatedAt, &v.LocalUpdatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// === Operator Info ===

// UpsertOperatorInfo создаёт или обновляет информацию об операторе.
func UpsertOperatorInfo(o *OperatorInfo) error {
	_, err := DB().Exec(`
		INSERT INTO operator_info (key, name, isp, asn, city, org, first_seen, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			name = excluded.name,
			isp = excluded.isp,
			asn = excluded.asn,
			city = excluded.city,
			org = excluded.org,
			last_seen = excluded.last_seen
	`, o.Key, o.Name, o.ISP, o.ASN, o.City, o.Org, o.FirstSeen, o.LastSeen)
	return err
}

// GetOperatorInfo возвращает информацию об операторе по ключу.
func GetOperatorInfo(key string) (*OperatorInfo, error) {
	var o OperatorInfo
	err := DB().QueryRow(`
		SELECT key, name, isp, asn, city, org, first_seen, last_seen
		FROM operator_info WHERE key = ?
	`, key).Scan(&o.Key, &o.Name, &o.ISP, &o.ASN, &o.City, &o.Org, &o.FirstSeen, &o.LastSeen)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// GetAllOperatorInfo возвращает список всех известных операторов.
func GetAllOperatorInfo() ([]OperatorInfo, error) {
	rows, err := DB().Query(`
		SELECT key, name, isp, asn, city, org, first_seen, last_seen
		FROM operator_info ORDER BY last_seen DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var operators []OperatorInfo
	for rows.Next() {
		var o OperatorInfo
		if err := rows.Scan(&o.Key, &o.Name, &o.ISP, &o.ASN, &o.City, &o.Org, &o.FirstSeen, &o.LastSeen); err != nil {
			return nil, err
		}
		operators = append(operators, o)
	}
	return operators, rows.Err()
}

// === Operator Strategies ===

// SaveOperatorStrategy сохраняет стратегию для оператора.
// auto_test_results обновляется только при непустом значении (т.е. только
// при реальном автоподборе) — ручная установка стратегии не должна затирать
// ранее сохранённые результаты автоподбора.
func SaveOperatorStrategy(operatorKey, strategy, testResults string) error {
	now := time.Now()
	_, err := DB().Exec(`
		INSERT INTO operator_strategies (operator_key, strategy, auto_test_results, tested_at, applied_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(operator_key, strategy) DO UPDATE SET
			auto_test_results = COALESCE(NULLIF(excluded.auto_test_results, ''), operator_strategies.auto_test_results),
			tested_at = CASE WHEN excluded.auto_test_results IS NOT NULL AND excluded.auto_test_results != '' THEN excluded.tested_at ELSE operator_strategies.tested_at END,
			applied_at = excluded.applied_at
	`, operatorKey, strategy, testResults, now, now)
	return err
}

// GetOperatorStrategy возвращает сохранённую стратегию для оператора.
func GetOperatorStrategy(operatorKey string) (string, error) {
	var strategy string
	err := DB().QueryRow(`
		SELECT strategy FROM operator_strategies
		WHERE operator_key = ? ORDER BY applied_at DESC LIMIT 1
	`, operatorKey).Scan(&strategy)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return strategy, err
}

// GetOperatorTestResults возвращает результаты автотеста для оператора.
func GetOperatorTestResults(operatorKey string) (string, error) {
	var results sql.NullString
	err := DB().QueryRow(`
		SELECT auto_test_results FROM operator_strategies
		WHERE operator_key = ? AND auto_test_results IS NOT NULL AND auto_test_results != ''
		ORDER BY tested_at DESC LIMIT 1
	`, operatorKey).Scan(&results)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return results.String, nil
}

// EnsureOperatorStrategy создаёт строку стратегии для оператора если её ещё нет.
// Новые стратегии из папки добавляются, существующие не перезаписываются.
func EnsureOperatorStrategy(operatorKey, strategy, displayName string) error {
	_, err := DB().Exec(`
		INSERT INTO operator_strategies (operator_key, strategy, display_name)
		VALUES (?, ?, ?)
		ON CONFLICT(operator_key, strategy) DO UPDATE SET
			display_name = COALESCE(NULLIF(excluded.display_name, ''), operator_strategies.display_name)
	`, operatorKey, strategy, displayName)
	return err
}

// MarkActiveStrategy отмечает одну стратегию как активную для оператора, остальные — нет.
func MarkActiveStrategy(operatorKey, strategy, source string) error {
	now := time.Now()
	tx, err := DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE operator_strategies SET is_active = 0 WHERE operator_key = ?`, operatorKey); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		UPDATE operator_strategies SET
			is_active = 1,
			use_count = use_count + 1,
			last_source = ?,
			applied_at = ?
		WHERE operator_key = ? AND strategy = ?
	`, source, now, operatorKey, strategy); err != nil {
		return err
	}
	return tx.Commit()
}

// GetOperatorStrategies возвращает все стратегии оператора с метаданными.
func GetOperatorStrategies(operatorKey string) ([]OperatorStrategy, error) {
	rows, err := DB().Query(`
		SELECT id, operator_key, strategy, display_name, availability_pct,
		       is_active, use_count, last_source, auto_test_results, tested_at, applied_at
		FROM operator_strategies
		WHERE operator_key = ?
		ORDER BY is_active DESC, use_count DESC, strategy ASC
	`, operatorKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []OperatorStrategy
	for rows.Next() {
		var s OperatorStrategy
		var testedAt, appliedAt sql.NullTime
		var autoTest sql.NullString
		if err := rows.Scan(&s.ID, &s.OperatorKey, &s.Strategy, &s.DisplayName,
			&s.AvailabilityPct, &s.IsActive, &s.UseCount, &s.LastSource,
			&autoTest, &testedAt, &appliedAt); err != nil {
			return nil, err
		}
		s.AutoTestResults = autoTest.String
		if testedAt.Valid {
			s.TestedAt = &testedAt.Time
		}
		if appliedAt.Valid {
			s.AppliedAt = &appliedAt.Time
		}
		result = append(result, s)
	}
	return result, nil
}

// UpdateStrategyAvailability обновляет процент доступности стратегии.
func UpdateStrategyAvailability(operatorKey, strategy string, pct float64) error {
	_, err := DB().Exec(`
		UPDATE operator_strategies SET availability_pct = ? WHERE operator_key = ? AND strategy = ?
	`, pct, operatorKey, strategy)
	return err
}

// === Current Operator ===

// SetCurrentOperator устанавливает текущего оператора.
func SetCurrentOperator(key, name, strategy string) error {
	now := time.Now()
	_, err := DB().Exec(`
		INSERT INTO current_operator (id, operator_key, operator_name, detected_at, strategy)
		VALUES (1, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			operator_key = excluded.operator_key,
			operator_name = excluded.operator_name,
			detected_at = excluded.detected_at,
			strategy = excluded.strategy
	`, key, name, now, strategy)
	return err
}

// GetCurrentOperator возвращает текущего оператора.
func GetCurrentOperator() (*CurrentOperator, error) {
	var o CurrentOperator
	err := DB().QueryRow(`
		SELECT id, operator_key, operator_name, detected_at, strategy, zapret_just_updated
		FROM current_operator WHERE id = 1
	`).Scan(&o.ID, &o.OperatorKey, &o.OperatorName, &o.DetectedAt, &o.Strategy, &o.ZapretJustUpdated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// SetZapretJustUpdated устанавливает/сбрасывает флаг «zapret только что обновлён».
func SetZapretJustUpdated(updated bool) error {
	_, err := DB().Exec(`
		INSERT INTO current_operator (id, zapret_just_updated)
		VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET zapret_just_updated = excluded.zapret_just_updated
	`, updated)
	return err
}

// GetZapretJustUpdated возвращает флаг «zapret только что обновлён».
func GetZapretJustUpdated() bool {
	var v bool
	err := DB().QueryRow(`SELECT zapret_just_updated FROM current_operator WHERE id = 1`).Scan(&v)
	if err != nil {
		return false
	}
	return v
}