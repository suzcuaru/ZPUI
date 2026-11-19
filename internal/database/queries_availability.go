package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)


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

// GetResourceDailySince — суточные агрегаты по всем ресурсам с даты
// (включительно); для секции data.resources.
func GetResourceDailySince(since string) ([]ResourceDaily, error) {
	rows, err := DB().Query(`SELECT date, operator_key, host, checks_total, checks_ok, pct
	      FROM resource_availability_daily WHERE date >= ?
	      ORDER BY date ASC, host ASC`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ResourceDaily
	for rows.Next() {
		var r ResourceDaily
		if err := rows.Scan(&r.Date, &r.OperatorKey, &r.Host, &r.ChecksTotal, &r.ChecksOK, &r.Pct); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CleanOldResourceAvailability удаляет сырые per-resource данные старше duration.
func CleanOldResourceAvailability(maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)
	_, err := DB().Exec(`DELETE FROM resource_availability WHERE timestamp < ?`, cutoff)
	return err
}
