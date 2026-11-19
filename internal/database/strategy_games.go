package database

import (
	"errors"
	"time"
)

// === Игровые баллы стратегий (значок «игровая») ===

// StrategyGame — сохранённый результат игрового прогона стратегии.
// GfTCP/GfUDP — порты игрового фильтра, с которыми стратегия показала
// этот результат (пусто = порты не подбирались).
type StrategyGame struct {
	Strategy string    `json:"strategy"`
	Score    float64   `json:"score"`
	PingMs   int64     `json:"ping_ms"`
	JitterMs int64     `json:"jitter_ms"`
	Ok       int       `json:"ok"`
	Total    int       `json:"total"`
	GfTCP    string    `json:"gf_tcp,omitempty"`
	GfUDP    string    `json:"gf_udp,omitempty"`
	TestedAt time.Time `json:"tested_at"`
}

// SaveStrategyGame — upsert игрового балла стратегии.
func SaveStrategyGame(g StrategyGame) error {
	_, err := DB().Exec(`
		INSERT INTO strategy_games (strategy, score, ping_ms, jitter_ms, ok, total, gf_tcp, gf_udp, tested_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(strategy) DO UPDATE SET
			score = excluded.score,
			ping_ms = excluded.ping_ms,
			jitter_ms = excluded.jitter_ms,
			ok = excluded.ok,
			total = excluded.total,
			gf_tcp = excluded.gf_tcp,
			gf_udp = excluded.gf_udp,
			tested_at = excluded.tested_at
	`, g.Strategy, g.Score, g.PingMs, g.JitterMs, g.Ok, g.Total, g.GfTCP, g.GfUDP, g.TestedAt)
	return err
}

// GetStrategyGames — все игровые баллы (свежие сверху).
func GetStrategyGames() ([]StrategyGame, error) {
	rows, err := DB().Query(`SELECT strategy, score, ping_ms, jitter_ms, ok, total, gf_tcp, gf_udp, tested_at FROM strategy_games ORDER BY score DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []StrategyGame
	for rows.Next() {
		var g StrategyGame
		var tested any
		if err := rows.Scan(&g.Strategy, &g.Score, &g.PingMs, &g.JitterMs, &g.Ok, &g.Total, &g.GfTCP, &g.GfUDP, &tested); err != nil {
			return nil, err
		}
		if t, ok := tested.(time.Time); ok {
			g.TestedAt = t
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GetStrategyGame — игровой балл одной стратегии (ok=false — записи нет).
func GetStrategyGame(strategy string) (StrategyGame, bool, error) {
	var g StrategyGame
	var tested any
	err := DB().QueryRow(`SELECT strategy, score, ping_ms, jitter_ms, ok, total, gf_tcp, gf_udp, tested_at FROM strategy_games WHERE strategy = ?`, strategy).
		Scan(&g.Strategy, &g.Score, &g.PingMs, &g.JitterMs, &g.Ok, &g.Total, &g.GfTCP, &g.GfUDP, &tested)
	if err != nil {
		return StrategyGame{}, false, nil
	}
	if t, ok := tested.(time.Time); ok {
		g.TestedAt = t
	}
	return g, true, nil
}

// === app_flags: мини-KV для служебных флагов (расписание промптов и пр.) ===

// SetFlag — записать значение флага.
func SetFlag(key, value string) error {
	if db == nil {
		return errors.New("database not open")
	}
	_, err := DB().Exec(`
		INSERT INTO app_flags (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

// GetFlag — прочитать флаг; "" и nil, если флага нет. Неоткрытая БД —
// тоже «флага нет» (балльный скоринг зовёт это до инициализации БД в тестах).
func GetFlag(key string) (string, error) {
	if db == nil {
		return "", nil
	}
	var v string
	err := DB().QueryRow(`SELECT value FROM app_flags WHERE key = ?`, key).Scan(&v)
	if err == nil {
		return v, nil
	}
	return "", nil
}
