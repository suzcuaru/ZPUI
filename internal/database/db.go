package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
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
	`)
	return err
}