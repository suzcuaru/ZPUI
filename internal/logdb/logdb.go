package logdb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var (
	db   *sql.DB
	mu   sync.RWMutex
)

// Ready сообщает, инициализирована ли БД логов и можно ли писать через Insert.
// Используется логгером, чтобы не дублировать записи в boot.log после того,
// как БД уже доступна.
func Ready() bool {
	mu.RLock()
	defer mu.RUnlock()
	return db != nil
}

type LogTable string

const (
	TableApp     LogTable = "app_logs"
	TableZapret  LogTable = "zapret_logs"
	TableUpdater LogTable = "updater_logs"
	TableReport  LogTable = "report_logs"
)

var AllTables = []LogTable{TableApp, TableZapret, TableUpdater, TableReport}

func tableCategory(category string) LogTable {
	switch category {
	case "app", "config", "proxy", "monitor", "xboxdns", "tray", "system", "network", "availability":
		return TableApp
	case "zapret", "service", "strategy", "install", "winws":
		return TableZapret
	case "updater", "selfupdate", "update":
		return TableUpdater
	case "report":
		return TableReport
	default:
		return TableApp
	}
}

type LogEntry struct {
	ID        int64  `json:"id"`
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Category  string `json:"category,omitempty"`
	Table     string `json:"table,omitempty"`
}

type LogStats struct {
	Total  int64 `json:"total"`
	Errors int64 `json:"errors"`
	DbSize int64 `json:"db_size"`
}

func Init(dbDir string) error {
	mu.Lock()
	defer mu.Unlock()

	if db != nil {
		return nil
	}

	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("create logdb dir: %w", err)
	}

	dbPath := filepath.Join(dbDir, "logs.db")
	var err error
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open logdb: %w", err)
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-1000",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("logdb pragma %s: %w", p, err)
		}
	}

	if err := migrate(); err != nil {
		return fmt.Errorf("logdb migrate: %w", err)
	}

	return nil
}

func migrate() error {
	for _, table := range AllTables {
		query := fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				timestamp TEXT NOT NULL,
				level TEXT NOT NULL,
				code TEXT DEFAULT '',
				message TEXT NOT NULL,
				category TEXT DEFAULT ''
			);
			CREATE INDEX IF NOT EXISTS idx_%s_ts ON %s(timestamp);
			CREATE INDEX IF NOT EXISTS idx_%s_level ON %s(level);
		`, table, table, table, table, table)
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}
	for _, table := range AllTables {
		db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN category TEXT DEFAULT ''", table))
	}
	return nil
}

func Insert(category, level, code, message string) {
	mu.RLock()
	d := db
	mu.RUnlock()
	if d == nil {
		return
	}

	table := tableCategory(category)
	ts := time.Now().Format("2006-01-02 15:04:05")
	_, err := d.Exec(fmt.Sprintf("INSERT INTO %s (timestamp, level, code, message, category) VALUES (?, ?, ?, ?, ?)", table),
		ts, level, code, message, category)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logdb insert error: %v\n", err)
	}
}

func Query(table LogTable, level string, limit, offset int) ([]LogEntry, error) {
	mu.RLock()
	d := db
	mu.RUnlock()
	if d == nil {
		return nil, fmt.Errorf("logdb not initialized")
	}

	if limit <= 0 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	var rows *sql.Rows
	var err error

	if level != "" && level != "ALL" {
		q := fmt.Sprintf("SELECT id, timestamp, level, COALESCE(code,''), message, COALESCE(category,'') FROM %s WHERE level = ? ORDER BY id DESC LIMIT ? OFFSET ?", table)
		rows, err = d.Query(q, level, limit, offset)
	} else {
		q := fmt.Sprintf("SELECT id, timestamp, level, COALESCE(code,''), message, COALESCE(category,'') FROM %s ORDER BY id DESC LIMIT ? OFFSET ?", table)
		rows, err = d.Query(q, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []LogEntry
	for rows.Next() {
		var e LogEntry
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.Level, &e.Code, &e.Message, &e.Category); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func QueryByID(table LogTable, id int64) (*LogEntry, error) {
	mu.RLock()
	d := db
	mu.RUnlock()
	if d == nil {
		return nil, fmt.Errorf("logdb not initialized")
	}

	q := fmt.Sprintf("SELECT id, timestamp, level, COALESCE(code,''), message, COALESCE(category,'') FROM %s WHERE id = ?", table)
	var e LogEntry
	err := d.QueryRow(q, id).Scan(&e.ID, &e.Timestamp, &e.Level, &e.Code, &e.Message, &e.Category)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func GetStats() LogStats {
	mu.RLock()
	d := db
	mu.RUnlock()
	if d == nil {
		return LogStats{}
	}

	var total, errs int64
	for _, table := range AllTables {
		var cnt int64
		d.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&cnt)
		total += cnt

		var ecnt int64
		d.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE level = 'ERROR'", table)).Scan(&ecnt)
		errs += ecnt
	}

	var dbSize int64
	var pageCount, pageSize int
	d.QueryRow("PRAGMA page_count").Scan(&pageCount)
	d.QueryRow("PRAGMA page_size").Scan(&pageSize)
	dbSize = int64(pageCount) * int64(pageSize)

	return LogStats{Total: total, Errors: errs, DbSize: dbSize}
}

func Count(table LogTable, level string) int64 {
	mu.RLock()
	d := db
	mu.RUnlock()
	if d == nil {
		return 0
	}

	var cnt int64
	if level != "" && level != "ALL" {
		d.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE level = ?", table), level).Scan(&cnt)
	} else {
		d.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&cnt)
	}
	return cnt
}

// QueryRecentErrors collects recent ERROR/WARN entries from all log tables,
// sorted by timestamp descending.  Each entry gets a .Table field set.
func QueryRecentErrors(limit int) ([]LogEntry, error) {
	mu.RLock()
	d := db
	mu.RUnlock()
	if d == nil {
		return nil, fmt.Errorf("logdb not initialized")
	}
	if limit <= 0 {
		limit = 50
	}

	var result []LogEntry
	for _, table := range AllTables {
		q := fmt.Sprintf("SELECT id, timestamp, level, COALESCE(code,''), message, COALESCE(category,'') FROM %s WHERE level IN ('ERROR','WARN') ORDER BY id DESC LIMIT ?", table)
		rows, err := d.Query(q, limit)
		if err != nil {
			continue
		}
		for rows.Next() {
			var e LogEntry
			if err := rows.Scan(&e.ID, &e.Timestamp, &e.Level, &e.Code, &e.Message, &e.Category); err != nil {
				rows.Close()
				continue
			}
			e.Table = string(table)
			result = append(result, e)
		}
		rows.Close()
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp > result[j].Timestamp
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

// CountErrors returns total ERROR count across all tables.
func CountErrors() int64 {
	mu.RLock()
	d := db
	mu.RUnlock()
	if d == nil {
		return 0
	}
	var total int64
	for _, table := range AllTables {
		var cnt int64
		d.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE level = 'ERROR'", table)).Scan(&cnt)
		total += cnt
	}
	return total
}

func Clear(table LogTable) error {
	mu.RLock()
	d := db
	mu.RUnlock()
	if d == nil {
		return fmt.Errorf("logdb not initialized")
	}

	if table == "all" {
		for _, t := range AllTables {
			if _, err := d.Exec(fmt.Sprintf("DELETE FROM %s", t)); err != nil {
				return err
			}
		}
		return nil
	}

	_, err := d.Exec(fmt.Sprintf("DELETE FROM %s", table))
	if err == nil {
		d.Exec("VACUUM")
	}
	return err
}

func CleanOld(maxAge time.Duration) {
	mu.RLock()
	d := db
	mu.RUnlock()
	if d == nil {
		return
	}

	cutoff := time.Now().Add(-maxAge).Format("2006-01-02 15:04:05")
	for _, table := range AllTables {
		if _, err := d.Exec(fmt.Sprintf("DELETE FROM %s WHERE timestamp < ?", table), cutoff); err != nil {
			return
		}
	}
	d.Exec("VACUUM")
}

func Vacuum() {
	mu.RLock()
	d := db
	mu.RUnlock()
	if d == nil {
		return
	}
	d.Exec("VACUUM")
}

func Close() error {
	mu.RLock()
	d := db
	mu.RUnlock()
	if d == nil {
		return nil
	}
	return d.Close()
}

// ExportAll reads all log entries from all tables and returns a map
// of table name -> formatted text lines (one per entry).
func ExportAll() map[string]string {
	mu.RLock()
	d := db
	mu.RUnlock()
	if d == nil {
		return nil
	}

	result := make(map[string]string, len(AllTables))
	for _, table := range AllTables {
		q := fmt.Sprintf("SELECT id, timestamp, level, COALESCE(code,''), message, COALESCE(category,'') FROM %s ORDER BY id ASC", table)
		rows, err := d.Query(q)
		if err != nil {
			continue
		}
		var buf strings.Builder
		for rows.Next() {
			var e LogEntry
			if err := rows.Scan(&e.ID, &e.Timestamp, &e.Level, &e.Code, &e.Message, &e.Category); err != nil {
				continue
			}
			catPart := ""
			if e.Category != "" {
				catPart = " [" + e.Category + "]"
			}
			codePart := ""
			if e.Code != "" {
				codePart = " [" + e.Code + "]"
			}
			fmt.Fprintf(&buf, "[%s] [%s]%s%s %s\n", e.Timestamp, e.Level, codePart, catPart, e.Message)
		}
		rows.Close()
		if buf.Len() > 0 {
			result[string(table)] = buf.String()
		}
	}
	return result
}
