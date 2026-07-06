package database

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
	mu sync.RWMutex
}

func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	d := &DB{DB: db}
	if err := d.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.DB.Close()
}

func (d *DB) migrate() error {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS module_data (
			module_id TEXT NOT NULL,
			key TEXT NOT NULL,
			value BLOB,
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (module_id, key)
		)`,
		`CREATE TABLE IF NOT EXISTS update_state (
			key TEXT PRIMARY KEY,
			value TEXT
		)`,
	}
	for _, s := range schema {
		if _, err := d.Exec(s); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

func (d *DB) SetModuleData(moduleID, key string, value []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.Exec(
		`INSERT OR REPLACE INTO module_data (module_id, key, value, updated_at) VALUES (?, ?, ?, ?)`,
		moduleID, key, value, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (d *DB) GetModuleData(moduleID, key string) ([]byte, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var val []byte
	err := d.QueryRow(`SELECT value FROM module_data WHERE module_id = ? AND key = ?`, moduleID, key).Scan(&val)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return val, err
}

func (d *DB) DeleteModuleData(moduleID, key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.Exec(`DELETE FROM module_data WHERE module_id = ? AND key = ?`, moduleID, key)
	return err
}

func (d *DB) ListModuleData(moduleID string) ([]string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	rows, err := d.Query(`SELECT key FROM module_data WHERE module_id = ? ORDER BY key`, moduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (d *DB) SetUpdateState(key, value string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.Exec(`INSERT OR REPLACE INTO update_state (key, value) VALUES (?, ?)`, key, value)
	return err
}

func (d *DB) GetUpdateState(key string) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var val string
	err := d.QueryRow(`SELECT value FROM update_state WHERE key = ?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}
