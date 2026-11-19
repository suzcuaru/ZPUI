package database

import (
	"database/sql"
	"strings"
	"time"

)


// === Operator Info ===

// UpsertOperatorInfo создаёт или обновляет информацию об операторе.
func UpsertOperatorInfo(o *OperatorInfo) error {
	_, err := DB().Exec(`
		INSERT INTO operator_info (key, name, isp, asn, city, org, first_seen, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			name = excluded.name,
			isp = excluded.isp,
			asn = excluded.asn,
			city = excluded.city,
			org = excluded.org,
			last_seen = excluded.last_seen
	`, o.Key, o.Name, o.ISP, o.ASN, o.City, o.Org, o.FirstSeen, o.LastSeen)
	return err
}

// GetOperatorInfo возвращает информацию об операторе по ключу.
func GetOperatorInfo(key string) (*OperatorInfo, error) {
	var o OperatorInfo
	err := DB().QueryRow(`
		SELECT key, name, isp, asn, city, org, first_seen, last_seen
		FROM operator_info WHERE key = ?
	`, key).Scan(&o.Key, &o.Name, &o.ISP, &o.ASN, &o.City, &o.Org, &o.FirstSeen, &o.LastSeen)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// GetAllOperatorInfo возвращает список всех известных операторов.
func GetAllOperatorInfo() ([]OperatorInfo, error) {
	rows, err := DB().Query(`
		SELECT key, name, isp, asn, city, org, first_seen, last_seen
		FROM operator_info ORDER BY last_seen DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var operators []OperatorInfo
	for rows.Next() {
		var o OperatorInfo
		if err := rows.Scan(&o.Key, &o.Name, &o.ISP, &o.ASN, &o.City, &o.Org, &o.FirstSeen, &o.LastSeen); err != nil {
			return nil, err
		}
		operators = append(operators, o)
	}
	return operators, rows.Err()
}

// === Operator Strategies ===

// SaveOperatorStrategy сохраняет стратегию для оператора.
// auto_test_results обновляется только при непустом значении (т.е. только
// при реальном автоподборе) — ручная установка стратегии не должна затирать
// ранее сохранённые результаты автоподбора.
func SaveOperatorStrategy(operatorKey, strategy, testResults string) error {
	now := time.Now()
	_, err := DB().Exec(`
		INSERT INTO operator_strategies (operator_key, strategy, auto_test_results, tested_at, applied_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(operator_key, strategy) DO UPDATE SET
			auto_test_results = COALESCE(NULLIF(excluded.auto_test_results, ''), operator_strategies.auto_test_results),
			tested_at = CASE WHEN excluded.auto_test_results IS NOT NULL AND excluded.auto_test_results != '' THEN excluded.tested_at ELSE operator_strategies.tested_at END,
			applied_at = excluded.applied_at
	`, operatorKey, strategy, testResults, now, now)
	return err
}

// GetOperatorStrategy возвращает сохранённую стратегию для оператора.
func GetOperatorStrategy(operatorKey string) (string, error) {
	var strategy string
	err := DB().QueryRow(`
		SELECT strategy FROM operator_strategies
		WHERE operator_key = ? ORDER BY applied_at DESC LIMIT 1
	`, operatorKey).Scan(&strategy)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return strategy, err
}

// EnsureOperatorStrategy создаёт строку стратегии для оператора если её ещё нет.
// Новые стратегии из папки добавляются, существующие не перезаписываются.
func EnsureOperatorStrategy(operatorKey, strategy, displayName string) error {
	return busyRetry(func() error {
		_, err := DB().Exec(`
			INSERT INTO operator_strategies (operator_key, strategy, display_name)
			VALUES (?, ?, ?)
			ON CONFLICT(operator_key, strategy) DO UPDATE SET
				display_name = COALESCE(NULLIF(excluded.display_name, ''), operator_strategies.display_name)
		`, operatorKey, strategy, displayName)
		return err
	})
}

// EnsureOperatorStrategiesBatch — все стратегии оператора одной транзакцией:
// синхронизация на старте писала каждую стратегию отдельной записью, и те
// конкурировали за блокировку с ротацией/мониторами, изредка ловя SQLITE_BUSY.
func EnsureOperatorStrategiesBatch(operatorKey string, filenames, displayNames []string) error {
	return busyRetry(func() error {
		tx, err := DB().Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for i, strategy := range filenames {
			display := ""
			if i < len(displayNames) {
				display = displayNames[i]
			}
			if _, err := tx.Exec(`
				INSERT INTO operator_strategies (operator_key, strategy, display_name)
				VALUES (?, ?, ?)
				ON CONFLICT(operator_key, strategy) DO UPDATE SET
					display_name = COALESCE(NULLIF(excluded.display_name, ''), operator_strategies.display_name)
			`, operatorKey, strategy, display); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// MarkActiveStrategy отмечает одну стратегию как активную для оператора, остальные — нет.
func MarkActiveStrategy(operatorKey, strategy, source string) error {
	now := time.Now()
	tx, err := DB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE operator_strategies SET is_active = 0 WHERE operator_key = ?`, operatorKey); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		UPDATE operator_strategies SET
			is_active = 1,
			use_count = use_count + 1,
			last_source = ?,
			applied_at = ?
		WHERE operator_key = ? AND strategy = ?
	`, source, now, operatorKey, strategy); err != nil {
		return err
	}
	return tx.Commit()
}

// MarkObservedStrategy отражает факт «в движке сейчас эта стратегия» без
// создания истории применения: use_count/applied_at не трогаются. Строки,
// помеченные только так, «базой оператора» для стартового подбора не
// считаются — на чистой системе должен запускаться полный перебор.
func MarkObservedStrategy(operatorKey, strategy string) error {
	_, err := DB().Exec(`
		UPDATE operator_strategies SET is_active = (strategy = ?)
		WHERE operator_key = ?
	`, strategy, operatorKey)
	return err
}

// GetOperatorStrategies возвращает все стратегии оператора с метаданными.
// display_name/last_source допускают NULL (SaveOperatorStrategy вставляет
// строку без них) — COALESCE, иначе один NULL ломает всю выдачу оператора.
func GetOperatorStrategies(operatorKey string) ([]OperatorStrategy, error) {
	rows, err := DB().Query(`
		SELECT id, operator_key, strategy, COALESCE(display_name, ''), availability_pct,
		       is_active, use_count, COALESCE(last_source, ''), auto_test_results, tested_at, applied_at
		FROM operator_strategies
		WHERE operator_key = ?
		ORDER BY is_active DESC, use_count DESC, strategy ASC
	`, operatorKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []OperatorStrategy
	for rows.Next() {
		var s OperatorStrategy
		var testedAt, appliedAt sql.NullTime
		var autoTest sql.NullString
		if err := rows.Scan(&s.ID, &s.OperatorKey, &s.Strategy, &s.DisplayName,
			&s.AvailabilityPct, &s.IsActive, &s.UseCount, &s.LastSource,
			&autoTest, &testedAt, &appliedAt); err != nil {
			return nil, err
		}
		s.AutoTestResults = autoTest.String
		if testedAt.Valid {
			s.TestedAt = &testedAt.Time
		}
		if appliedAt.Valid {
			s.AppliedAt = &appliedAt.Time
		}
		result = append(result, s)
	}
	return result, nil
}

// UpdateStrategyAvailability обновляет процент доступности стратегии.
func UpdateStrategyAvailability(operatorKey, strategy string, pct float64) error {
	_, err := DB().Exec(`
		UPDATE operator_strategies SET availability_pct = ? WHERE operator_key = ? AND strategy = ?
	`, pct, operatorKey, strategy)
	return err
}

// ClearOperatorStrategyEra стирает данные проверок (auto_test_results,
// tested_at, availability_pct) у стратегий оператора вне списка keep —
// данные подбора соответствуют только последнему ручному прогону.
// История применений (use_count/is_active/last_source/applied_at) и
// display_name не трогаются; таблицы провайдера/ресурсов не затрагиваются.
func ClearOperatorStrategyEra(operatorKey string, keep []string) error {
	if operatorKey == "" {
		return nil
	}
	query := `UPDATE operator_strategies SET auto_test_results = '', tested_at = NULL, availability_pct = -1 WHERE operator_key = ?`
	args := []interface{}{operatorKey}
	if len(keep) > 0 {
		ph := strings.TrimRight(strings.Repeat("?,", len(keep)), ",")
		query += ` AND strategy NOT IN (` + ph + `)`
		for _, k := range keep {
			args = append(args, k)
		}
	}
	_, err := DB().Exec(query, args...)
	return err
}

// MigrateUnknownOperatorStrategies переносит результаты автоподбора, сохранённые
// под fallback-оператором "unknown" (детект оператора не удался), реальному
// оператору. Существующие строки реального оператора не перезаписываются.
func MigrateUnknownOperatorStrategies(toKey string) (int64, error) {
	if toKey == "" || toKey == "unknown" {
		return 0, nil
	}
	tx, err := DB().Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
		INSERT OR IGNORE INTO operator_strategies
			(operator_key, strategy, display_name, availability_pct, is_active, use_count, last_source, auto_test_results, tested_at, applied_at)
		SELECT ?, strategy, display_name, availability_pct, is_active, use_count, last_source, auto_test_results, tested_at, applied_at
		FROM operator_strategies WHERE operator_key = 'unknown'
	`, toKey)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()

	// Активной остаётся только последняя применённая стратегия оператора.
	if _, err := tx.Exec(`
		UPDATE operator_strategies SET is_active = 0
		WHERE operator_key = ? AND is_active = 1 AND id NOT IN (
			SELECT id FROM operator_strategies WHERE operator_key = ? ORDER BY applied_at DESC LIMIT 1
		)
	`, toKey, toKey); err != nil {
		return 0, err
	}

	if _, err := tx.Exec(`DELETE FROM operator_strategies WHERE operator_key = 'unknown'`); err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

// ResolveOperatorByASN — каноничный ключ оператора для ASN (самая старая
// запись operator_info: у неё больше всего истории) и остальные ключи с тем
// же ASN. Разные geo-IP источники зовут одну сеть по-разному («PJSC
// Rostelecom» / «Rostelecom networks» / региональные юрлица) — без привязки
// к ASN ключ оператора прыгал между вариантами и терял историю автотестов.
func ResolveOperatorByASN(asn string) (canonical string, aliases []string, err error) {
	if asn == "" {
		return "", nil, nil
	}
	rows, err := DB().Query(`SELECT key FROM operator_info WHERE asn = ? ORDER BY first_seen ASC, key ASC`, asn)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return "", nil, err
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	if len(keys) == 0 {
		return "", nil, nil
	}
	return keys[0], keys[1:], nil
}

// MergeOperatorStrategies переносит строки автоподбора с ключей-алиасов на
// каноничный ключ оператора. Строки каноничного ключа не затираются
// (INSERT OR IGNORE); активной остаётся последняя применённая стратегия.
func MergeOperatorStrategies(aliasKeys []string, toKey string) (int64, error) {
	clean := make([]string, 0, len(aliasKeys))
	for _, k := range aliasKeys {
		if k != "" && k != toKey {
			clean = append(clean, k)
		}
	}
	if len(clean) == 0 || toKey == "" || toKey == "unknown" {
		return 0, nil
	}
	tx, err := DB().Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	ph := strings.TrimRight(strings.Repeat("?,", len(clean)), ",")
	args := make([]interface{}, 0, len(clean)+1)
	args = append(args, toKey)
	for _, k := range clean {
		args = append(args, k)
	}

	res, err := tx.Exec(`
		INSERT OR IGNORE INTO operator_strategies
			(operator_key, strategy, display_name, availability_pct, is_active, use_count, last_source, auto_test_results, tested_at, applied_at)
		SELECT ?, strategy, display_name, availability_pct, is_active, use_count, last_source, auto_test_results, tested_at, applied_at
		FROM operator_strategies WHERE operator_key IN (`+ph+`)
	`, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()

	// Активной остаётся только последняя применённая стратегия оператора.
	if _, err := tx.Exec(`
		UPDATE operator_strategies SET is_active = 0
		WHERE operator_key = ? AND is_active = 1 AND id NOT IN (
			SELECT id FROM operator_strategies WHERE operator_key = ? ORDER BY applied_at DESC LIMIT 1
		)
	`, toKey, toKey); err != nil {
		return 0, err
	}

	delArgs := make([]interface{}, 0, len(clean))
	for _, k := range clean {
		delArgs = append(delArgs, k)
	}
	if _, err := tx.Exec(`DELETE FROM operator_strategies WHERE operator_key IN (`+ph+`)`, delArgs...); err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

// === Current Operator ===

// SetCurrentOperator устанавливает текущего оператора.
func SetCurrentOperator(key, name, strategy string) error {
	now := time.Now()
	_, err := DB().Exec(`
		INSERT INTO current_operator (id, operator_key, operator_name, detected_at, strategy)
		VALUES (1, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			operator_key = excluded.operator_key,
			operator_name = excluded.operator_name,
			detected_at = excluded.detected_at,
			strategy = excluded.strategy
	`, key, name, now, strategy)
	return err
}

// GetCurrentOperator возвращает текущего оператора.
func GetCurrentOperator() (*CurrentOperator, error) {
	var o CurrentOperator
	err := DB().QueryRow(`
		SELECT id, operator_key, operator_name, detected_at, strategy, zapret_just_updated
		FROM current_operator WHERE id = 1
	`).Scan(&o.ID, &o.OperatorKey, &o.OperatorName, &o.DetectedAt, &o.Strategy, &o.ZapretJustUpdated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}
