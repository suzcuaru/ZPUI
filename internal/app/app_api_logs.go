package app

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"zpui/internal/executil"
	"zpui/internal/logdb"
	"zpui/internal/zsvc"
)

func (a *App) GetLogs(table string, level string, limit, offset int) map[string]interface{} {
	logTable := logdb.TableApp
	switch table {
	case "service":
		return map[string]interface{}{"lines": readServiceLog(level, limit, offset)}
	case "zapret":
		logTable = logdb.TableZapret
	case "updater", "selfupdate", "update":
		logTable = logdb.TableUpdater
	case "report":
		logTable = logdb.TableReport
	case "tgproxy":
		logTable = logdb.TableTgProxy
	default:
		logTable = logdb.TableApp
	}

	logLines, err := a.log.ReadRecent(logTable, level, limit, offset)
	if err != nil {
		return errResp(err.Error())
	}
	return map[string]interface{}{"lines": logLines}
}

func (a *App) GetAllLogs(level string, limit, offset int) map[string]interface{} {
	var all []logdb.LogEntry
	for _, table := range logdb.AllTables {
		entries, err := a.log.ReadRecent(table, level, limit, offset)
		if err != nil {
			continue
		}
		for _, e := range entries {
			e.Table = string(table)
			all = append(all, e)
		}
	}

	// Каждая таблица отдала свои «последние N» независимо: без общей
	// сортировки по времени наверху оказывались только логи приложения,
	// а ошибки zapret/tgproxy оставались за лимитом и не показывались.
	// ВАЖНО: сортировка по ID не годится — автоинкремент у каждой таблицы
	// свой, и записи zapret_logs (ID ~сотни) всегда проигрывали app_logs
	// (ID ~тысячи) и целиком срезались лимитом. Сортируем по времени.
	sort.Slice(all, func(i, j int) bool {
		if all[i].Timestamp != all[j].Timestamp {
			return all[i].Timestamp > all[j].Timestamp
		}
		return all[i].ID > all[j].ID
	})

	if len(all) > limit {
		all = all[:limit]
	}

	return map[string]interface{}{"lines": all}
}

func (a *App) GetLogStats() map[string]interface{} {
	stats := a.log.GetStats()
	tableCounts := make(map[string]int64)
	for _, table := range logdb.AllTables {
		tableCounts[string(table)] = a.log.Count(table, "")
	}
	tableCounts["service"] = int64(countServiceLogLines())
	return map[string]interface{}{
		"total":   stats.Total,
		"errors":  stats.Errors,
		"db_size": stats.DbSize,
		"counts":  tableCounts,
	}
}

func (a *App) ClearLogBucket(table string) map[string]interface{} {
	if table == "service" {
		if err := clearServiceLog(); err != nil {
			return errResp(err.Error())
		}
		a.log.Info("app", "Service log cleared")
		return okResp()
	}
	logTable := logdb.TableApp
	switch table {
	case "zapret":
		logTable = logdb.TableZapret
	case "updater", "selfupdate", "update":
		logTable = logdb.TableUpdater
	case "report":
		logTable = logdb.TableReport
	case "tgproxy":
		logTable = logdb.TableTgProxy
	case "all", "":
		if err := a.log.ClearAll(); err != nil {
			return errResp(err.Error())
		}
		a.log.Info("app", "All logs cleared")
		return okResp()
	default:
		logTable = logdb.TableApp
	}
	if err := a.log.Clear(logTable); err != nil {
		return errResp(err.Error())
	}
	a.log.Info("app", "Log table cleared: "+table)
	return okResp()
}

func (a *App) ExportLogs() map[string]interface{} {
	logsDir := a.cfg.LogsDir()

	downloads := getDownloadsDir()
	if downloads == "" {
		return errResp("cannot find Downloads folder")
	}

	stamp := time.Now().Format("2006-01-02_150405")
	zipPath := filepath.Join(downloads, fmt.Sprintf("zpui-logs-%s.zip", stamp))

	zipFile, err := os.Create(zipPath)
	if err != nil {
		return errResp(err.Error())
	}
	defer zipFile.Close()

	zw := zip.NewWriter(zipFile)

	entries, err := os.ReadDir(logsDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(logsDir, e.Name()))
			if err != nil {
				continue
			}
			w, err := zw.Create(e.Name())
			if err != nil {
				continue
			}
			w.Write(data)
		}
	}

	dbLogs := logdb.ExportAll()
	for table, text := range dbLogs {
		w, err := zw.Create("db-" + table + ".log")
		if err != nil {
			continue
		}
		w.Write([]byte(text))
	}

	zw.Close()

	executil.HiddenCmd("explorer.exe", "/select,\""+zipPath+"\"").Start()

	return map[string]interface{}{
		"status": "ok",
		"path":   zipPath,
	}
}

func getDownloadsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	candidates := []string{
		filepath.Join(home, "Downloads"),
		filepath.Join(os.Getenv("USERPROFILE"), "Downloads"),
		filepath.Join(home, "Загрузки"),
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	return ""
}

func (a *App) LogErrorCode(code, category, msg string) map[string]interface{} {
	a.log.ErrorCode(category, code, msg)
	return okResp()
}

// LogUiError — ошибка, которую пользователь уже увидел тостом: пишем ERROR в
// журнал (logdb → журнал «Приложение» и секция «Logs & Errors» отчёта) и
// ставим в очередь телеметрии на сервер. Хук onError не зовётся — повторное
// OS-уведомление дублировало бы тост; отправка — в горутине, биндинг не ждёт
// сеть.
func (a *App) LogUiError(code, msg string) map[string]interface{} {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return errResp("empty_message")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		code = "ui_error"
	}
	if r := []rune(msg); len(r) > 512 {
		msg = string(r[:512])
	}
	a.log.ErrorCodeShown("frontend", code, msg)
	return okResp()
}

// ============================================================
// ЖУРНАЛ СЛУЖБЫ ZPUI (logs\service.log + ротация .1/.2)
// ============================================================
//
// Служба пишет свой журнал файлом, а не в общую SQLite: работает от SYSTEM
// независимо от приложения, БД приложения ей не принадлежит. Формат строки:
// «2026-10-23 09:20:13.977 INFO сообщение», ротация при 512KB (до трёх файлов).

// serviceLogPath — журнал службы в каталоге установки (реестр), при
// недоступности — logs рядом с exe приложения.
func serviceLogPath() string {
	dir := zsvc.InstallDir()
	if dir == "" {
		if exe, err := os.Executable(); err == nil {
			dir = filepath.Dir(exe)
		}
	}
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "logs", "service.log")
}

// readServiceLog — последние записи журнала службы в формате таблиц журнала.
// level — фильтр («», «ALL» — всё); offset отсекает самые свежие, limit
// ограничивает выдачу; наверху — свежие, как в остальных таблицах.
func readServiceLog(level string, limit, offset int) []logdb.LogEntry {
	out := []logdb.LogEntry{}
	path := serviceLogPath()
	if path == "" {
		return out
	}
	entries := make([]logdb.LogEntry, 0, 128)
	// Хронологический порядок: ротированные .2 → .1 → текущий.
	for _, p := range []string{path + ".2", path + ".1", path} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, ln := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
			if strings.TrimSpace(ln) == "" {
				continue
			}
			e := parseServiceLogLine(ln)
			if level != "" && level != "ALL" && !strings.EqualFold(e.Level, level) {
				continue
			}
			entries = append(entries, e)
		}
	}
	if offset > 0 {
		if offset >= len(entries) {
			return out
		}
		entries = entries[:len(entries)-offset]
	}
	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries
}

// parseServiceLogLine разбирает строку журнала; неразобранное — INFO без времени.
func parseServiceLogLine(ln string) logdb.LogEntry {
	e := logdb.LogEntry{Level: "INFO", Table: "service"}
	// «2026-10-23 09:20:13.977 INFO сообщение»: 23 символа даты-времени + пробел.
	if len(ln) > 24 && ln[23] == ' ' && ln[4] == '-' && ln[13] == ':' {
		e.Timestamp = ln[0:19]
		rest := ln[24:]
		if sp := strings.IndexByte(rest, ' '); sp > 0 {
			switch rest[:sp] {
			case "INFO", "WARN", "ERROR", "DEBUG":
				e.Level = rest[:sp]
				e.Message = strings.TrimSpace(rest[sp+1:])
				return e
			}
		}
		e.Message = strings.TrimSpace(rest)
		return e
	}
	e.Message = ln
	return e
}

// countServiceLogLines — число строк журнала службы (бейдж подвкладки).
func countServiceLogLines() int {
	path := serviceLogPath()
	if path == "" {
		return 0
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n := 0
	for _, ln := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(ln) != "" {
			n++
		}
	}
	return n
}

// clearServiceLog — очистка журнала службы (усечение файла).
func clearServiceLog() error {
	path := serviceLogPath()
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	return os.Truncate(path, 0)
}
