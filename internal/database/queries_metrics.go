package database

import (
	"time"
)

// AppMetric — один замер потребления самим приложением (30-секундное окно
// при запуске). Хранится 7 дней, попадает в диагностический отчёт.
type AppMetric struct {
	ID           int64     `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	CPUPercent   float64   `json:"cpu_percent"`
	WallMs       int64     `json:"wall_ms"`
	CPUMs        int64     `json:"cpu_ms"`
	WorkingSetMB float64   `json:"working_set_mb"`
	PrivateMB    float64   `json:"private_mb"`
	Threads      int       `json:"threads"`
	Handles      int       `json:"handles"`
	Children     int       `json:"children"`
	PowerSaving  bool      `json:"power_saving"`
}

func InsertAppMetric(m *AppMetric) error {
	_, err := DB().Exec(`
		INSERT INTO app_metrics (timestamp, cpu_percent, wall_ms, cpu_ms, working_set_mb, private_mb, threads, handles, children, power_saving)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, m.Timestamp, m.CPUPercent, m.WallMs, m.CPUMs, m.WorkingSetMB, m.PrivateMB, m.Threads, m.Handles, m.Children, m.PowerSaving)
	return err
}

func GetAppMetrics(since time.Time, limit int) ([]AppMetric, error) {
	rows, err := DB().Query(`
		SELECT id, timestamp, cpu_percent, wall_ms, cpu_ms, working_set_mb, private_mb, threads, handles, children, power_saving
		FROM app_metrics WHERE timestamp >= ? ORDER BY timestamp DESC LIMIT ?
	`, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AppMetric
	for rows.Next() {
		var m AppMetric
		var power int
		if err := rows.Scan(&m.ID, &m.Timestamp, &m.CPUPercent, &m.WallMs, &m.CPUMs,
			&m.WorkingSetMB, &m.PrivateMB, &m.Threads, &m.Handles, &m.Children, &power); err != nil {
			return nil, err
		}
		m.PowerSaving = power != 0
		out = append(out, m)
	}
	return out, rows.Err()
}

func CleanOldAppMetrics(maxAge time.Duration) error {
	_, err := DB().Exec(`DELETE FROM app_metrics WHERE timestamp < ?`, time.Now().Add(-maxAge))
	return err
}
