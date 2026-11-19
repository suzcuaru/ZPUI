package database

import (
	"database/sql"

)


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
