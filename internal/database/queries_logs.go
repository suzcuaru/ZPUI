package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)


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

// InsertSyncHistory пишет запись истории и подчищает просроченные строки
// (больше 30 дней не храним; удаление на каждой вставке — запрос раз в минуту).

// syncHistoryRetention — сколько хранить историю запросов канала (30 дней).
const syncHistoryRetention = 30 * 24 * time.Hour

func InsertSyncHistory(who, op, result string) error {
	_, err := DB().Exec(`INSERT INTO sync_history (ts, who, op, result) VALUES (?, ?, ?, ?)`,
		time.Now(), who, op, result)
	if err != nil {
		return err
	}
	_, err = DB().Exec(`DELETE FROM sync_history WHERE ts < ?`, time.Now().Add(-syncHistoryRetention))
	return err
}

// GetSyncHistory — записи истории с момента since, свежие сверху.
func GetSyncHistory(since time.Time, limit int) ([]SyncHistoryEntry, error) {
	if limit <= 0 {
		limit = 2000
	}
	rows, err := DB().Query(`
		SELECT ts, who, op, result
		FROM sync_history
		WHERE ts >= ?
		ORDER BY ts DESC
		LIMIT ?
	`, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SyncHistoryEntry
	for rows.Next() {
		var e SyncHistoryEntry
		var result sql.NullString
		if err := rows.Scan(&e.TS, &e.Who, &e.Op, &result); err != nil {
			return nil, err
		}
		e.Result = result.String
		out = append(out, e)
	}
	return out, rows.Err()
}
