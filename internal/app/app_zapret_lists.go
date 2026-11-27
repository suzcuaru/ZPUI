package app

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"


	"zpui/internal/database"
	"zpui/internal/zapret"
)


// ============================================================
// IPSET
// ============================================================

// Режим ipset-фильтра хранится в config.json (ipset_mode) и применяется
// парсером стратегии при старте движка (заглушки ipset-mode-*.txt);
// реальный список ipset-all.txt не изменяется режимами — только обновлением.
// Смена режима = сохранить конфиг + ApplyIpsetMode (перезапуск/пересоздание
// службы), иначе режим ни на что не влияет.

func (a *App) ipsetFilePath() string { return filepath.Join(a.cfg.ListsDir(), "ipset-all.txt") }

// GetIpsetStatus — режим из конфига + состояние реального списка (записи,
// битые строки, свежесть скачивания).
func (a *App) GetIpsetStatus() map[string]interface{} {
	mode := a.cfg.GetIpsetMode()
	res := map[string]interface{}{"status": mode}
	data, err := os.ReadFile(a.ipsetFilePath())
	if err != nil {
		res["entries"] = 0
		return res
	}
	content := strings.TrimSpace(string(data))
	lines := strings.Split(content, "\n")
	valid, invalid := 0, 0
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if ip := net.ParseIP(l); ip != nil {
			valid++
			continue
		}
		if _, _, err := net.ParseCIDR(l); err == nil {
			valid++
			continue
		}
		invalid++
	}
	res["entries"] = valid
	res["invalid"] = invalid
	if fi, err := os.Stat(a.ipsetFilePath()); err == nil {
		res["age_days"] = int(time.Since(fi.ModTime()).Hours() / 24)
		res["updated_at"] = fi.ModTime().Unix()
	}
	return res
}

// migrateIpsetMode — миграция старого файлового механизма режимов: прежний
// ToggleIpset писал маркеры прямо в ipset-all.txt (пусто = any; служебная
// 203.0.113.113 = none). Переносим режим в config и восстанавливаем список
// скачиванием.
func (a *App) migrateIpsetMode() {
	data, err := os.ReadFile(a.ipsetFilePath())
	if err != nil {
		return
	}
	c := strings.TrimSpace(string(data))
	var mode string
	switch {
	case c == "":
		mode = zapret.IpsetModeAny
	case c == "203.0.113.113/32":
		mode = zapret.IpsetModeNone
	default:
		return // реальный список — мигрировать нечего
	}
	if err := a.cfg.SetIpsetMode(mode); err != nil {
		a.log.Warn("strategy", "ipset migrate: "+err.Error())
		return
	}
	a.log.Info("strategy", "ipset mode migrated from file marker: "+mode)
	go func() {
		if result := a.updateIpsetFile(); result["error"] == nil {
			a.emitStrategiesChanged()
		}
	}()
}

// ToggleIpset — смена режима ipset-фильтра (loaded|none|any).
func (a *App) ToggleIpset(mode string) map[string]interface{} {
	switch mode {
	case zapret.IpsetModeLoaded, zapret.IpsetModeNone, zapret.IpsetModeAny:
	default:
		return errResp("unknown mode: " + mode)
	}
	if err := a.cfg.SetIpsetMode(mode); err != nil {
		return errResp(err.Error())
	}
	a.log.Info("strategy", "Ipset filter set to: "+mode)
	restarted := true
	if st := a.zapret.GetStatus(); st == zapret.StatusRunning && !a.zapret.IsAutoTestRunning() {
		if err := a.zapret.ApplyIpsetMode(); err != nil {
			a.log.Warn("strategy", "ipset apply: "+err.Error())
			restarted = false
		}
	} else {
		restarted = false
	}
	a.emitStrategiesChanged()
	return map[string]interface{}{"status": "ok", "restarted": restarted}
}

// ============================================================
// AUTO UPDATE FLAG
// ============================================================



// ============================================================
// UPDATE IPSET / HOSTS
// ============================================================

// ipsetServiceURL — источник ipset-списка (авто-обновление).
const ipsetServiceURL = "https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/refs/heads/main/.service/ipset-service.txt"

// updateIpsetFile — скачивание свежего ipset-all.txt (без побочных эффектов).
func (a *App) updateIpsetFile() map[string]interface{} {
	listFile := a.ipsetFilePath()
	return downloadAndSave(ipsetServiceURL, listFile)
}

// ============================================================
// LISTS
// ============================================================

func (a *App) GetLists() map[string]interface{} {
	listsDir := a.cfg.ListsDir()
	listFiles := []string{
		"list-general.txt", "list-general-user.txt",
		"list-exclude.txt", "list-exclude-system.txt", "list-exclude-user.txt",
		"ipset-all.txt", "ipset-exclude.txt", "ipset-exclude-user.txt",
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
	return map[string]interface{}{"lists": result, "server": a.serverListsInfoMap()}
}

// getUserList — содержимое пользовательского списка обхода для MCP
// (браузерное расширение показывает «уже в списке» и дедупит).
func (a *App) getUserList() map[string]interface{} {
	path := filepath.Join(a.cfg.ListsDir(), "list-general-user.txt")
	hosts := []string{}
	if data, err := os.ReadFile(path); err == nil {
		for _, l := range strings.Split(string(data), "\n") {
			l = strings.TrimSpace(l)
			if l != "" && !strings.HasPrefix(l, "#") {
				hosts = append(hosts, l)
			}
		}
	}
	return map[string]interface{}{"hosts": hosts, "count": len(hosts)}
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

// ============================================================
// SKIP RESOURCES
// ============================================================

func (a *App) GetSkipResources() map[string]interface{} {
	path := a.cfg.GetSkipResourcesFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]interface{}{"content": "", "lines": []string{}, "system": a.systemSkipBlock()}
	}
	content := string(data)
	lines := []string{}
	for _, l := range strings.Split(content, "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "#") {
			lines = append(lines, l)
		}
	}
	return map[string]interface{}{
		"content": content,
		"lines":   lines,
		"count":   len(lines),
		"system":  a.systemSkipBlock(),
	}
}

// systemSkipBlock — read-only содержимое системного skip-файла для UI:
// версия набора и время последней сверки с сервером (заголовок панели),
// домены пользователя из системного списка убраны — дубликаты не показываются.
func (a *App) systemSkipBlock() map[string]interface{} {
	sys := a.cfg.GetSkipResourcesSystem()
	if sys == nil {
		sys = []string{}
	}
	userSet := map[string]bool{}
	for _, u := range a.cfg.GetSkipResources() {
		userSet[strings.ToLower(strings.TrimSpace(u))] = true
	}
	covered := func(s string) bool {
		if userSet[s] {
			return true
		}
		for u := range userSet {
			if strings.HasSuffix(s, "."+u) {
				return true
			}
		}
		return false
	}
	lines := make([]string, 0, len(sys))
	for _, s := range sys {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" || covered(s) {
			continue
		}
		lines = append(lines, s)
	}
	synced, _ := database.GetFlag("exclusions_sync_at")
	lastErr, _ := database.GetFlag("serverlists_last_error")
	return map[string]interface{}{
		"lines":      lines,
		"count":      len(lines),
		"version":    exclusionsLocalVersion(),
		"synced_at":  synced,
		"last_error": lastErr,
	}
}

func (a *App) SaveSkipResources(content string) map[string]interface{} {
	path := a.cfg.GetSkipResourcesFilePath()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return errResp(err.Error())
	}
	return okResp()
}

func (a *App) AddSkipResource(host string) map[string]interface{} {
	if err := a.cfg.AddSkipResource(host); err != nil {
		return errResp(err.Error())
	}
	return okResp()
}
