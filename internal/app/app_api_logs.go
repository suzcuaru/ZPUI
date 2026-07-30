package app

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zpui/internal/executil"
	"zpui/internal/logdb"
)

func (a *App) GetLogs(table string, level string, limit, offset int) map[string]interface{} {
	logTable := logdb.TableApp
	switch table {
	case "zapret":
		logTable = logdb.TableZapret
	case "updater", "selfupdate", "update":
		logTable = logdb.TableUpdater
	case "report":
		logTable = logdb.TableReport
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
	return map[string]interface{}{
		"total":   stats.Total,
		"errors":  stats.Errors,
		"db_size": stats.DbSize,
		"counts":  tableCounts,
	}
}

func (a *App) GetLogCount(table string, level string) map[string]interface{} {
	logTable := logdb.TableApp
	switch table {
	case "zapret":
		logTable = logdb.TableZapret
	case "updater", "selfupdate", "update":
		logTable = logdb.TableUpdater
	case "report":
		logTable = logdb.TableReport
	case "all":
		var total int64
		for _, t := range logdb.AllTables {
			total += a.log.Count(t, level)
		}
		return map[string]interface{}{"count": total}
	}
	return map[string]interface{}{"count": a.log.Count(logTable, level)}
}

func (a *App) ClearLogs() map[string]interface{} {
	a.log.ClearAll()
	a.log.Info("app", "All logs cleared")
	return okResp()
}

func (a *App) ClearLogBucket(table string) map[string]interface{} {
	logTable := logdb.TableApp
	switch table {
	case "zapret":
		logTable = logdb.TableZapret
	case "updater", "selfupdate", "update":
		logTable = logdb.TableUpdater
	case "report":
		logTable = logdb.TableReport
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

func (a *App) GetErrorSnapshots() map[string]interface{} {
	snapshots := a.log.ListErrorSnapshots()
	var files []map[string]interface{}
	for _, name := range snapshots {
		path := filepath.Join(a.cfg.LogsDir(), "errors", name)
		info, err := os.Stat(path)
		size := int64(0)
		date := ""
		if err == nil {
			size = info.Size()
			date = info.ModTime().Format("2006-01-02 15:04:05")
		}
		files = append(files, map[string]interface{}{
			"name": name,
			"size": size,
			"date": date,
		})
	}
	return map[string]interface{}{"files": files}
}

func (a *App) ReadErrorSnapshot(name string) map[string]interface{} {
	if name == "" {
		return errResp("name required")
	}
	content, err := a.log.ReadErrorSnapshot(name)
	if err != nil {
		return errResp(err.Error())
	}
	return map[string]interface{}{"content": content, "name": name}
}

func (a *App) DeleteErrorSnapshot(name string) map[string]interface{} {
	if name == "" {
		return errResp("name required")
	}
	clean := filepath.Base(name)
	path := filepath.Join(a.cfg.LogsDir(), "errors", clean)
	if err := os.Remove(path); err != nil {
		return errResp(err.Error())
	}
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
	dbDir := filepath.Join(a.cfg.GetZapretPath(), "..")
	dbFiles := []string{"config.json", filepath.Join(dbDir, "databases", "zpui.db")}
	for _, f := range dbFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		w, err := zw.Create(filepath.Base(f))
		if err != nil {
			continue
		}
		w.Write(data)
	}

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

func (a *App) GetLogTableNames() map[string]interface{} {
	tables := []map[string]string{
		{"id": "app", "name": "app_logs"},
		{"id": "zapret", "name": "zapret_logs"},
		{"id": "updater", "name": "updater_logs"},
		{"id": "report", "name": "report_logs"},
	}
	return map[string]interface{}{"tables": tables}
}

func (a *App) FindLogEntryByMsg(table, query string) map[string]interface{} {
	if query == "" {
		return errResp("query required")
	}

	logTable := logdb.TableApp
	switch table {
	case "zapret":
		logTable = logdb.TableZapret
	case "updater", "selfupdate", "update":
		logTable = logdb.TableUpdater
	case "report":
		logTable = logdb.TableReport
	}

	entries, err := a.log.ReadRecent(logTable, "", 5000, 0)
	if err != nil {
		return errResp(err.Error())
	}

	queryLower := strings.ToLower(query)
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Message), queryLower) {
			return map[string]interface{}{
				"found": true,
				"entry": e,
			}
		}
	}

	return map[string]interface{}{"found": false}
}

func (a *App) LogErrorCode(code, category, msg string) map[string]interface{} {
	a.log.ErrorCode(category, code, msg)
	return okResp()
}

func (a *App) SetLogDebug(category string, enabled bool) map[string]interface{} {
	if category == "" {
		return errResp("category required")
	}
	a.log.SetDebug(category, enabled)
	return okResp()
}

func (a *App) GetLogDebug() map[string]interface{} {
	return map[string]interface{}{"categories": a.log.GetDebugCategories()}
}
