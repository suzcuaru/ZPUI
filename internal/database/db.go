package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var (
	db    *sql.DB
	mu    sync.Mutex
	ready bool
)

// Init открывает SQLite базу данных, включает WAL mode, создаёт таблицы.
// Повторяемая инициализация: неудачная попытка НЕ блокирует последующие
// вызовы — они ретраят открытие (sync.Once сюда не подходит: первый сбой
// съедается навсегда, и DB() возвращал бы nil).
func Init(dbPath string) error {
	mu.Lock()
	defer mu.Unlock()
	if ready {
		return nil
	}

	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}

	// busy_timeout, journal_mode, synchronous, foreign_keys и cache_size
	// заданы через DSN (_pragma), чтобы применялись к КАЖДОМУ соединению
	// пула database/sql: PRAGMA через Exec действует только на одно
	// соединение. См. modernc.org/sqlite, параметры _pragma.
	dsn := dbPath +
		"?_pragma=busy_timeout(15000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(ON)" +
		"&_pragma=cache_size(-2000)"
	opened, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	// WAL даёт параллельные читатели при одном писателе; краткие записи
	// с busy_timeout(5000) не ловят SQLITE_BUSY. Пул из нескольких
	// соединений (вместо одного) убирает очередь «все операции ждут
	// одну» — иначе под нагрузкой /api/status ждал освобождения
	// единственного соединения, а вложенный запрос при итерации по
	// строкам другого запроса зависал навсегда (до перезапуска).
	opened.SetMaxOpenConns(4)

	if err := migrate(opened); err != nil {
		opened.Close()
		return fmt.Errorf("migrate: %w", err)
	}

	db = opened
	ready = true
	return nil
}

// DB возвращает текущее соединение
func DB() *sql.DB {
	return db
}

// busyRetry повторяет операцию при SQLITE_BUSY: на старте запись конкурирует
// с ротацией/мониторами, и одиночная коллизия не должна превращаться в
// ошибку в журнале. Пауза нарастает: 200/400/600 мс.
func busyRetry(fn func() error) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		if err = fn(); err == nil || !strings.Contains(err.Error(), "SQLITE_BUSY") {
			return err
		}
		time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
	}
	return err
}

// Close закрывает базу данных
func Close() error {
	if db != nil {
		return db.Close()
	}
	return nil
}

// migrate создаёт таблицы если их нет
func migrate(d *sql.DB) error {
	_, err := d.Exec(`
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

		CREATE TABLE IF NOT EXISTS yandex_metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			ping_ms INTEGER DEFAULT 0,
			speed_mbps REAL DEFAULT 0
		);

		CREATE INDEX IF NOT EXISTS idx_yandex_ts ON yandex_metrics(timestamp);

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

		CREATE TABLE IF NOT EXISTS app_metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME NOT NULL,
			cpu_percent REAL DEFAULT 0,
			wall_ms INTEGER DEFAULT 0,
			cpu_ms INTEGER DEFAULT 0,
			working_set_mb REAL DEFAULT 0,
			private_mb REAL DEFAULT 0,
			threads INTEGER DEFAULT 0,
			handles INTEGER DEFAULT 0,
			children INTEGER DEFAULT 0,
			power_saving INTEGER DEFAULT 0
		);

		CREATE INDEX IF NOT EXISTS idx_app_metrics_ts ON app_metrics(timestamp);

		CREATE TABLE IF NOT EXISTS strategy_games (
			strategy TEXT PRIMARY KEY,
			score REAL DEFAULT 0,
			ping_ms INTEGER DEFAULT 0,
			jitter_ms INTEGER DEFAULT 0,
			ok INTEGER DEFAULT 0,
			total INTEGER DEFAULT 0,
			gf_tcp TEXT DEFAULT '',
			gf_udp TEXT DEFAULT '',
			tested_at DATETIME
		);

		CREATE TABLE IF NOT EXISTS app_flags (
			key TEXT PRIMARY KEY,
			value TEXT
		);

		CREATE TABLE IF NOT EXISTS sync_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ts DATETIME NOT NULL,
			who TEXT NOT NULL,
			op TEXT NOT NULL,
			result TEXT DEFAULT ''
		);

		CREATE INDEX IF NOT EXISTS idx_sync_history_ts ON sync_history(ts);
	`)
	if err != nil {
		return err
	}

	// Вкладка «Сервисы» (Detector404) удалена — её таблицы больше не нужны.
	for _, stmt := range []string{
		`DROP TABLE IF EXISTS detector404_state`,
		`DROP TABLE IF EXISTS detector404_events`,
	} {
		if _, err := d.Exec(stmt); err != nil {
			return fmt.Errorf("drop table: %w", err)
		}
	}

	for _, stmt := range []string{
		`ALTER TABLE operator_strategies ADD COLUMN display_name TEXT`,
		`ALTER TABLE operator_strategies ADD COLUMN availability_pct REAL DEFAULT -1`,
		`ALTER TABLE operator_strategies ADD COLUMN is_active BOOLEAN DEFAULT 0`,
		`ALTER TABLE operator_strategies ADD COLUMN use_count INTEGER DEFAULT 0`,
		`ALTER TABLE operator_strategies ADD COLUMN last_source TEXT DEFAULT ''`,
		`ALTER TABLE strategy_games ADD COLUMN gf_tcp TEXT DEFAULT ''`,
		`ALTER TABLE strategy_games ADD COLUMN gf_udp TEXT DEFAULT ''`,
	} {
		if _, err := d.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("alter table: %w", err)
		}
	}

	return nil
}
