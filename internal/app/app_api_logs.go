package app

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"zpui/internal/executil"
)

// ============================================================
// LOGS
// ============================================================

func (a *App) GetLogs(category string, lines int) map[string]interface{} {
	if category == "" {
		category = "zapret"
	}
	if lines <= 0 {
		lines = 100
	}
	logLines := a.log.ReadRecent(category, lines)
	return map[string]interface{}{"lines": logLines}
}

func (a *App) GetLogFiles() map[string]interface{} {
	return map[string]interface{}{"files": a.log.ListLogFiles()}
}

func (a *App) ClearLogs() map[string]interface{} {
	a.log.Clear()
	return okResp()
}

// ClearLogBucket clears a specific log category (in-memory + file).
// category = "zapret" / "network" / "app" / "availability" / "tray" / "xboxdns" / ...
// If category = "all" - clears all log files.
func (a *App) ClearLogBucket(category string) map[string]interface{} {
	if category == "" {
		return errResp("category required")
	}
	if category == "all" {
		if err := a.log.ClearAll(); err != nil {
			return errResp(err.Error())
		}
		a.log.Info("app", "All logs cleared")
		return okResp()
	}
	bucket := category
	switch category {
	case "network", "proxy":
		bucket = "network"
	case "zapret", "service", "strategy", "updater", "install":
		bucket = "zapret"
	}
	if err := a.log.ClearBucket(bucket); err != nil {
		return errResp(err.Error())
	}
	a.log.Info("app", "Log bucket cleared: "+bucket)
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

func (a *App) GetErrorSnapshots() map[string]interface{} {
	errorsDir := filepath.Join(a.cfg.LogsDir(), "errors")
	return map[string]interface{}{"files": listLogDir(errorsDir)}
}

func (a *App) ReadErrorSnapshot(name string) map[string]interface{} {
	if name == "" {
		return errResp("name required")
	}
	path := filepath.Join(a.cfg.LogsDir(), "errors", name)
	data, err := os.ReadFile(path)
	if err != nil {
		return errResp(err.Error())
	}
	return map[string]interface{}{"content": string(data), "name": name}
}

func (a *App) DeleteErrorSnapshot(name string) map[string]interface{} {
	if name == "" {
		return errResp("name required")
	}
	path := filepath.Join(a.cfg.LogsDir(), "errors", name)
	if err := os.Remove(path); err != nil {
		return errResp(err.Error())
	}
	return okResp()
}

func (a *App) GetArchiveLogs() map[string]interface{} {
	archiveDir := filepath.Join(a.cfg.LogsDir(), "archive")
	return map[string]interface{}{"files": listLogDir(archiveDir)}
}

func (a *App) ReadArchiveLog(name string) map[string]interface{} {
	if name == "" {
		return errResp("name required")
	}
	path := filepath.Join(a.cfg.LogsDir(), "archive", name)
	data, err := os.ReadFile(path)
	if err != nil {
		return errResp(err.Error())
	}
	return map[string]interface{}{"content": string(data), "name": name}
}

// ============================================================
// CACHE CLEAR
// ============================================================

func (a *App) ClearCache(target string) map[string]interface{} {
	cleared := []string{}

	if target == "discord" || target == "all" {
		discordPaths := []string{
			os.Getenv("APPDATA") + `\discord\Cache`,
			os.Getenv("APPDATA") + `\discord\Code Cache`,
			os.Getenv("APPDATA") + `\discord\GPUCache`,
			os.Getenv("APPDATA") + `\discord\Service Worker\CacheStorage`,
			os.Getenv("APPDATA") + `\discord\Service Worker\ScriptCache`,
		}
		for _, p := range discordPaths {
			if err := os.RemoveAll(p); err == nil {
				cleared = append(cleared, filepath.Base(p))
			}
		}
	}

	if target == "network" || target == "all" {
		executil.HiddenCmd("ipconfig", "/flushdns").Run()
		executil.HiddenCmd("netsh", "winsock", "reset").Run()
		executil.HiddenCmd("netsh", "int", "ip", "reset").Run()
		cleared = append(cleared, "DNS cache", "Winsock", "IP stack")
	}

	if len(cleared) > 0 {
		return map[string]interface{}{"status": "ok", "cleared": cleared}
	}
	return map[string]interface{}{"status": "nothing", "cleared": []string{}}
}

// ============================================================
// LISTS
// ============================================================

func (a *App) GetLists() map[string]interface{} {
	listsDir := a.cfg.ListsDir()
	listFiles := []string{
		"list-general.txt",
		"list-general-user.txt",
		"list-exclude.txt",
		"list-exclude-user.txt",
		"ipset-all.txt",
		"ipset-exclude.txt",
		"ipset-exclude-user.txt",
	}
	type ListInfo struct {
		Name     string   `json:"name"`
		Lines    []string `json:"lines"`
		Count    int      `json:"count"`
		Editable bool     `json:"editable"`
	}
	var result []ListInfo
	for _, f := range listFiles {
		path := filepath.Join(listsDir, f)
		data, err := os.ReadFile(path)
		lines := []string{}
		if err == nil {
			for _, l := range strings.Split(string(data), "\n") {
				l = strings.TrimSpace(l)
				if l != "" && !strings.HasPrefix(l, "#") {
					lines = append(lines, l)
				}
			}
		}
		editable := strings.HasSuffix(f, "-user.txt")
		result = append(result, ListInfo{Name: f, Lines: lines, Count: len(lines), Editable: editable})
	}
	return map[string]interface{}{"lists": result}
}

func (a *App) SaveList(name string, content string) map[string]interface{} {
	if name == "" {
		return errResp("name required")
	}
	if !strings.HasSuffix(name, "-user.txt") {
		return errResp("only user lists are editable")
	}
	listsDir := a.cfg.ListsDir()
	path := filepath.Join(listsDir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
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
	addToZip := func(fullPath, zipName string) {
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return
		}
		w, err := zw.Create(zipName)
		if err != nil {
			return
		}
		w.Write(data)
	}

	addDir := func(dir, prefix string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
				continue
			}
			addToZip(filepath.Join(dir, e.Name()), prefix+e.Name())
		}
	}

	addDir(logsDir, "")
	addDir(filepath.Join(logsDir, "errors"), "errors/")
	addDir(filepath.Join(logsDir, "archive"), "archive/")

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
