package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	db   *sql.DB
	once sync.Once
)

// Init открывает SQLite базу данных, включает WAL mode, создаёт таблицы
func Init(dbPath string) error {
	var initErr error
	once.Do(func() {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			initErr = fmt.Errorf("create db dir: %w", err)
			return
		}

		var err error
		db, err = sql.Open("sqlite", dbPath)
		if err != nil {
			initErr = fmt.Errorf("open db: %w", err)
			return
		}

		// Настройки производительности и надёжности
		pragmas := []string{
			"PRAGMA journal_mode=WAL",
			"PRAGMA synchronous=NORMAL",
			"PRAGMA cache_size=-2000",    // 2MB cache
			"PRAGMA foreign_keys=ON",
			"PRAGMA busy_timeout=5000",   // 5s timeout при блокировке
		}
		for _, p := range pragmas {
			if _, err := db.Exec(p); err != nil {
				initErr = fmt.Errorf("pragma %s: %w", p, err)
				return
			}
		}

		// Создание таблиц
		if err := migrate(); err != nil {
			initErr = fmt.Errorf("migrate: %w", err)
			return
		}
	})
	return initErr
}

// DB возвращает текущее соединение
func DB() *sql.DB {
	return db
}

// Close закрывает базу данных
func Close() error {
	if db != nil {
		return db.Close()
	}
	return nil
}

// migrate создаёт таблицы если их нет
func migrate() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS session_devices (
			id TEXT PRIMARY KEY,
			mac TEXT NOT NULL,
			ip TEXT,
			hostname TEXT,
			first_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
			total_dl BIGINT DEFAULT 0,
			total_ul BIGINT DEFAULT 0,
			is_online BOOLEAN DEFAULT FALSE
		);

		CREATE TABLE IF NOT EXISTS device_connections (
			id TEXT PRIMARY KEY,
			device_id TEXT,
			dst_host TEXT,
			dst_port INTEGER,
			bytes_dl BIGINT DEFAULT 0,
			bytes_ul BIGINT DEFAULT 0,
			started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			closed_at DATETIME
		);

		CREATE TABLE IF NOT EXISTS action_logs (
			id TEXT PRIMARY KEY,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			category TEXT,
			action TEXT,
			details TEXT
		);

		CREATE TABLE IF NOT EXISTS traffic_snapshots (
			id TEXT PRIMARY KEY,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			dl_speed REAL DEFAULT 0,
			ul_speed REAL DEFAULT 0,
			total_dl BIGINT DEFAULT 0,
			total_ul BIGINT DEFAULT 0,
			conn_count INTEGER DEFAULT 0
		);

		CREATE INDEX IF NOT EXISTS idx_dev_mac ON session_devices(mac);
		CREATE INDEX IF NOT EXISTS idx_conn_device ON device_connections(device_id);
		CREATE INDEX IF NOT EXISTS idx_log_ts ON action_logs(timestamp);
		CREATE INDEX IF NOT EXISTS idx_snap_ts ON traffic_snapshots(timestamp);

		CREATE TABLE IF NOT EXISTS availability_history (
			id TEXT PRIMARY KEY,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			type TEXT DEFAULT 'all',
			total_resources INTEGER DEFAULT 0,
			ok_resources INTEGER DEFAULT 0,
			pct REAL DEFAULT 0
		);

		CREATE INDEX IF NOT EXISTS idx_avail_ts ON availability_history(timestamp);

		CREATE TABLE IF NOT EXISTS zapret_backup (
			id INTEGER PRIMARY KEY DEFAULT 1,
			data TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS component_versions (
			id TEXT PRIMARY KEY,
			installed_version TEXT,
			remote_version TEXT,
			remote_source TEXT,
			remote_updated_at DATETIME,
			local_updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS operator_info (
			key TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			isp TEXT,
			asn TEXT,
			city TEXT,
			org TEXT,
			first_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_seen DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS operator_strategies (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			operator_key TEXT NOT NULL,
			strategy TEXT NOT NULL,
			auto_test_results TEXT,
			tested_at DATETIME,
			applied_at DATETIME,
			UNIQUE(operator_key, strategy)
		);

		CREATE TABLE IF NOT EXISTS current_operator (
			id INTEGER PRIMARY KEY DEFAULT 1,
			operator_key TEXT,
			operator_name TEXT,
			detected_at DATETIME,
			strategy TEXT,
			zapret_just_updated BOOLEAN DEFAULT FALSE
		);

		CREATE TABLE IF NOT EXISTS resource_availability (
			id TEXT PRIMARY KEY,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			operator_key TEXT,
			host TEXT NOT NULL,
			type TEXT DEFAULT 'standard',
			ok INTEGER DEFAULT 0,
			verdict TEXT,
			latency_ms INTEGER DEFAULT 0
		);

		CREATE INDEX IF NOT EXISTS idx_ravail_ts ON resource_availability(timestamp);
		CREATE INDEX IF NOT EXISTS idx_ravail_host ON resource_availability(host);
		CREATE INDEX IF NOT EXISTS idx_ravail_host_ts ON resource_availability(host, timestamp);

		CREATE TABLE IF NOT EXISTS resource_availability_daily (
			date TEXT NOT NULL,
			operator_key TEXT,
			host TEXT NOT NULL,
			checks_total INTEGER DEFAULT 0,
			checks_ok INTEGER DEFAULT 0,
			pct REAL DEFAULT 0,
			UNIQUE(date, operator_key, host)
		);
	`)
	if err != nil {
		return err
	}

	for _, stmt := range []string{
		`ALTER TABLE operator_strategies ADD COLUMN display_name TEXT`,
		`ALTER TABLE operator_strategies ADD COLUMN availability_pct REAL DEFAULT -1`,
		`ALTER TABLE operator_strategies ADD COLUMN is_active BOOLEAN DEFAULT 0`,
		`ALTER TABLE operator_strategies ADD COLUMN use_count INTEGER DEFAULT 0`,
		`ALTER TABLE operator_strategies ADD COLUMN last_source TEXT DEFAULT ''`,
	} {
		if _, err := db.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("alter table: %w", err)
		}
	}

	return nil
}