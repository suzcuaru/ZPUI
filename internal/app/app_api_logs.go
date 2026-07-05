package app

import (
	"os"
	"path/filepath"
	"strings"
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
