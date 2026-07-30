package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zpui/internal/database"
	"zpui/internal/executil"
	"zpui/internal/notify"
	"zpui/internal/zapret"
)

// ============================================================
// ZAPRET CONTROL
// ============================================================

func (a *App) ZapretStart() map[string]interface{} {
	if a.zapret.IsAutoTestRunning() {
		return errResp("strategy test in progress")
	}
	if a.isRecoveryRunning() {
		return map[string]interface{}{"status": "recovering", "reason": "recovery already in progress"}
	}

	if err := a.zapretStartAndVerify(); err != nil {
		// Запуск не удался — не рапортуем об успехе, запускаем восстановление.
		a.log.Warn("zapret", "start failed, triggering recovery: "+err.Error())
		a.startZapretRecovery("start_failed: " + err.Error())
		return map[string]interface{}{"status": "recovering", "reason": err.Error()}
	}
	return okResp()
}

func (a *App) ZapretStop() map[string]interface{} {
	if a.zapret.IsAutoTestRunning() {
		return errResp("strategy test in progress")
	}
	a.zapret.Stop()
	return okResp()
}

func (a *App) ZapretRestart() map[string]interface{} {
	if a.zapret.IsAutoTestRunning() {
		return errResp("strategy test in progress")
	}
	if err := a.zapret.Restart(); err != nil {
		return errResp(err.Error())
	}
	return map[string]interface{}{"status": "restarted"}
}

// ============================================================
// STRATEGIES
// ============================================================

func (a *App) GetStrategies() map[string]interface{} {
	return map[string]interface{}{"strategies": a.zapret.ListStrategies()}
}

func (a *App) SetStrategy(filename string) map[string]interface{} {
	if filename == "" {
		return errResp("filename required")
	}
	if a.zapret.IsAutoTestRunning() {
		return errResp("strategy test in progress")
	}
	if err := a.zapret.SetStrategy(filename); err != nil {
		return errResp(err.Error())
	}
	a.zapret.LogStrategyChange(filename, "manual")

	op, _ := database.GetCurrentOperator()
	if op != nil && op.OperatorKey != "" {
		database.SaveOperatorStrategy(op.OperatorKey, filename, "")
		database.SetCurrentOperator(op.OperatorKey, op.OperatorName, filename)
	}

	return map[string]interface{}{"status": "ok", "strategy": filename}
}

// ============================================================
// WINDOWS SERVICE
// ============================================================

func (a *App) InstallService(strategy string) map[string]interface{} {
	if strategy == "" {
		strategy = a.zapret.GetCurrentStrategy()
	}
	if err := a.zapret.InstallService(strategy); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"status": "installed"}
}

func (a *App) RemoveService() map[string]interface{} {
	a.zapret.RemoveService()
	return map[string]interface{}{"status": "removed"}
}

func (a *App) GetServiceStatus() interface{} {
	return a.zapret.GetServiceStatus()
}

// InstallServiceLogged — устанавливает службу запрета, записывая процесс в
// logs/install.log (перезаписываемый), с проверкой что служба отвечает.
func (a *App) InstallServiceLogged(strategy string) map[string]interface{} {
	res, err := a.zapret.InstallServiceLogged(strategy)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{
		"success":  res.Success,
		"version":  res.Version,
		"strategy": res.Strategy,
		"running":  res.Running,
		"errors":   res.Errors,
	}
}

// GetInstallLog — содержимое logs/install.log (для показа ошибок пользователю).
func (a *App) GetInstallLog() map[string]interface{} {
	logPath := filepath.Join(a.cfg.LogsDir(), "install.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return map[string]interface{}{"lines": []string{}}
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return map[string]interface{}{"lines": []string{}}
	}
	return map[string]interface{}{"lines": strings.Split(content, "\n")}
}

// DefaultStrategy — стратегия по умолчанию (первый general ALT).
func (a *App) DefaultStrategy() map[string]interface{} {
	return map[string]interface{}{"strategy": a.zapret.DefaultStrategyName()}
}

// GetAutoTestResults — результаты последнего автоподбора для текущего оператора
// из БД (раньше лежали в logs/auto_test_results.json — теперь читаем из базы,
// куда их пишет AutoSelectAndApply через SaveOperatorStrategy).
func (a *App) GetAutoTestResults() map[string]interface{} {
	op, _ := database.GetCurrentOperator()
	if op == nil || op.OperatorKey == "" {
		return map[string]interface{}{"results": []interface{}{}}
	}
	data, err := database.GetOperatorTestResults(op.OperatorKey)
	if err != nil || data == "" {
		return map[string]interface{}{"results": []interface{}{}}
	}
	var results []map[string]interface{}
	if err := json.Unmarshal([]byte(data), &results); err != nil {
		return map[string]interface{}{"results": []interface{}{}}
	}
	return map[string]interface{}{"results": results}
}

// ============================================================
// GAME FILTER
// ============================================================

func (a *App) GetGameFilter() map[string]interface{} {
	mode, _, _ := a.zapret.LoadGameFilter()
	return map[string]interface{}{"mode": mode}
}

func (a *App) SetGameFilter(mode string) map[string]interface{} {
	if err := a.zapret.SetGameFilter(mode); err != nil {
		a.log.Error("strategy", "Game filter set failed: "+err.Error())
		return map[string]interface{}{"error": err.Error()}
	}
	a.log.Info("strategy", "Game filter set to: "+mode)
	return map[string]interface{}{"status": "ok"}
}

// ============================================================
// UPDATES
// ============================================================

func (a *App) CheckForUpdates() interface{} {
	info, err := a.zapret.CheckForUpdates()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return info
}

func (a *App) ApplyUpdate() map[string]interface{} {
	a.saveBackupToDB()
	database.SetZapretJustUpdated(true)
	progress := make(chan zapret.UpdateProgress, 20)
	go a.zapret.PerformUpdate(progress)
	return map[string]interface{}{"status": "started"}
}

// ============================================================
// AUTO TEST (STRATEGY)
// ============================================================

func (a *App) StartAutoTest() map[string]interface{} {
	if a.zapret.IsAutoTestRunning() {
		return map[string]interface{}{"error": "Автотест уже запущен"}
	}
	return map[string]interface{}{"status": "started"}
}

func (a *App) CancelAutoTest() map[string]interface{} {
	a.zapret.CancelAutoTest()
	return map[string]interface{}{"status": "cancelled"}
}

// ============================================================
// ZAPRET INSTALL
// ============================================================

func (a *App) InstallZapret(sourceDir string) map[string]interface{} {
	if sourceDir == "" {
		return map[string]interface{}{"error": "source_dir required"}
	}
	a.saveBackupToDB()
	progress := make(chan zapret.UpdateProgress, 20)
	go a.zapret.InstallZapret(sourceDir, progress)
	return map[string]interface{}{"status": "started"}
}

// saveBackupToDB сохраняет слепок состояния zapret в базу данных перед обновлением.
// При следующем запуске, если zapret повреждён, состояние будет восстановлено.
func (a *App) saveBackupToDB() {
	snap := a.zapret.CaptureState()
	if data, err := json.Marshal(snap); err == nil {
		if err := database.SaveZapretBackup(string(data)); err != nil {
			a.log.Warn("app", "Не удалось сохранить backup в базу: "+err.Error())
		}
	}
}

// ============================================================
// IPSET
// ============================================================

func (a *App) GetIpsetStatus() map[string]interface{} {
	listFile := filepath.Join(a.cfg.ListsDir(), "ipset-all.txt")
	data, err := os.ReadFile(listFile)
	if err != nil {
		return map[string]interface{}{"status": "any"}
	}
	content := strings.TrimSpace(string(data))
	lines := strings.Split(content, "\n")
	nonEmpty := 0
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			nonEmpty++
		}
	}
	status := "loaded"
	if nonEmpty == 0 {
		status = "any"
	} else if nonEmpty == 1 && strings.Contains(content, "203.0.113.113") {
		status = "none"
	}
	return map[string]interface{}{"status": status}
}

func (a *App) ToggleIpset(mode string) map[string]interface{} {
	listFile := filepath.Join(a.cfg.ListsDir(), "ipset-all.txt")
	backupFile := listFile + ".backup"

	switch mode {
	case "none":
		os.WriteFile(backupFile, mustReadFile(listFile), 0644)
		os.WriteFile(listFile, []byte("203.0.113.113/32\n"), 0644)
	case "any":
		os.WriteFile(listFile, []byte(""), 0644)
	case "loaded":
		if backup, err := os.ReadFile(backupFile); err == nil {
			os.WriteFile(listFile, backup, 0644)
		}
	}
	a.log.Info("strategy", "Ipset filter set to: "+mode)
	return map[string]interface{}{"status": "ok"}
}

// ============================================================
// AUTO UPDATE FLAG
// ============================================================

func (a *App) GetAutoUpdateStatus() map[string]interface{} {
	flagFile := filepath.Join(a.cfg.GetZapretPath(), "utils", "check_updates.enabled")
	_, err := os.Stat(flagFile)
	return map[string]interface{}{"enabled": err == nil}
}

func (a *App) ToggleAutoUpdate(enabled bool) map[string]interface{} {
	flagFile := filepath.Join(a.cfg.GetZapretPath(), "utils", "check_updates.enabled")
	utilsDir := filepath.Join(a.cfg.GetZapretPath(), "utils")
	os.MkdirAll(utilsDir, 0755)
	if enabled {
		os.WriteFile(flagFile, []byte("ENABLED"), 0644)
	} else {
		os.Remove(flagFile)
	}
	return map[string]interface{}{"status": "ok"}
}

// ============================================================
// UPDATE IPSET / HOSTS
// ============================================================

func (a *App) UpdateIpset() map[string]interface{} {
	listFile := filepath.Join(a.cfg.ListsDir(), "ipset-all.txt")
	url := "https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/refs/heads/main/.service/ipset-service.txt"
	result := downloadAndSave(url, listFile)
	if result["error"] == nil {
		a.log.Info("strategy", "Ipset list updated from remote")
	} else {
		a.log.Error("strategy", "Ipset update failed: "+result["error"].(string))
	}
	return result
}

func (a *App) UpdateHosts() map[string]interface{} {
	url := "https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/refs/heads/main/.service/hosts"
	tmpFile := filepath.Join(os.TempDir(), "zapret_hosts.txt")
	result := downloadAndSave(url, tmpFile)
	if result["error"] == nil {
		a.log.Info("strategy", "Hosts list updated from remote")
	} else {
		a.log.Error("strategy", "Hosts update failed: "+result["error"].(string))
	}
	return result
}

// ============================================================
// FIRST RUN / ZAPRET MANAGEMENT
// ============================================================

func (a *App) HasLocalZapret() bool {
	return a.zapret.VerifyFiles().AllPresent
}

func (a *App) HasSystemZapretService() bool {
	cmd := executil.HiddenCmd("sc", "query", "zapret")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "zapret")
}

func (a *App) RemoveSystemZapretService() map[string]interface{} {
	a.log.Info("zapret", "Removing system zapret service...")
	a.zapret.Teardown()
	a.log.Info("zapret", "System zapret service removed")
	return map[string]interface{}{"status": "ok"}
}

func (a *App) VerifyZapretFiles() map[string]interface{} {
	vr := a.zapret.VerifyFiles()
	return map[string]interface{}{
		"all_present": vr.AllPresent,
		"version":     vr.Version,
		"files":       vr.Files,
	}
}

func (a *App) SendTestNotification() map[string]interface{} {
	if !a.cfg.GetNotificationsEnabled() {
		return errResp("notifications disabled")
	}
	lang := a.cfg.GetLanguage()
	if err := notify.Show(tr(lang, "test_title"), tr(lang, "test_body")); err != nil {
		return errResp(err.Error())
	}
	return okResp()
}

// ============================================================
// LISTS
// ============================================================

func (a *App) GetLists() map[string]interface{} {
	listsDir := a.cfg.ListsDir()
	listFiles := []string{
		"list-general.txt", "list-general-user.txt",
		"list-exclude.txt", "list-exclude-user.txt",
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

// ============================================================
// SKIP RESOURCES
// ============================================================

func (a *App) GetSkipResources() map[string]interface{} {
	path := a.cfg.GetSkipResourcesFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]interface{}{"content": "", "lines": []string{}}
	}
	content := string(data)
	lines := []string{}
	for _, l := range strings.Split(content, "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "#") {
			lines = append(lines, l)
		}
	}
	return map[string]interface{}{"content": content, "lines": lines, "count": len(lines)}
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

// ============================================================
// FULL REINSTALL
// ============================================================

// FullReinstall — полная переустановка zapret: удаляет папку, скачивает заново.
// Пользовательские списки бекапятся и восстанавливаются.
func (a *App) FullReinstall() map[string]interface{} {
	a.log.Info("zapret", "Full reinstall started")
	database.SetZapretJustUpdated(true)

	snap := a.zapret.CaptureState()

	zapretDir := a.cfg.GetZapretPath()
	a.log.Info("zapret", "Removing zapret directory: "+zapretDir)

	a.zapret.Stop()
	a.zapret.RemoveService()

	executil.HiddenCmd("taskkill", "/IM", "winws.exe", "/F").Run()
	executil.HiddenCmd("sc", "stop", "WinDivert").Run()
	executil.HiddenCmd("sc", "stop", "WinDivert14").Run()
	executil.HiddenCmd("sc", "delete", "WinDivert").Run()
	executil.HiddenCmd("sc", "delete", "WinDivert14").Run()

	time.Sleep(2 * time.Second)

	if err := os.RemoveAll(zapretDir); err != nil {
		a.log.Warn("zapret", "Failed to remove zapret dir: "+err.Error())
	}

	if err := os.MkdirAll(zapretDir, 0755); err != nil {
		return errResp("не удалось создать папку: " + err.Error())
	}

	a.log.Info("zapret", "Downloading fresh zapret...")
	if err := a.zapret.DownloadAndInstall(nil); err != nil {
		a.log.Error("zapret", "Download failed: "+err.Error())
		a.zapret.RestoreState(snap)
		return errResp("скачивание не удалось: " + err.Error())
	}

	a.zapret.RefreshVersion()

	strategy := snap.Strategy
	if strategy == "" {
		strategy = a.zapret.DefaultStrategyName()
	}
	a.cfg.SetCurrentStrategy(strategy)

	a.log.Info("zapret", "Restoring user lists and starting service...")
	a.zapret.RestoreState(snap)

	a.zapret.EnsureUserLists()

	if err := a.zapret.SetStrategy(strategy); err != nil {
		a.log.Warn("zapret", "Strategy apply failed: "+err.Error())
	}

	return map[string]interface{}{
		"status":   "ok",
		"version":  a.zapret.GetVersion(),
		"strategy": strategy,
	}
}

// IsServiceInstalled — проверяет, установлена ли служба zapret.
func (a *App) IsServiceInstalled() map[string]interface{} {
	return map[string]interface{}{"installed": a.HasSystemZapretService()}
}

// GetAutoSelectLastCheck — дата последнего автоподбора и сколько дней назад.
func (a *App) GetAutoSelectLastCheck() map[string]interface{} {
	lastStr := a.cfg.GetLastAutoSelectTime()
	if lastStr == "" {
		return map[string]interface{}{
			"last_check": "",
			"days_ago":   -1,
		}
	}
	t, err := time.Parse(time.RFC3339, lastStr)
	if err != nil {
		return map[string]interface{}{
			"last_check": lastStr,
			"days_ago":   -1,
		}
	}
	daysAgo := int(time.Since(t).Hours() / 24)
	return map[string]interface{}{
		"last_check": lastStr,
		"days_ago":   daysAgo,
	}
}

// ============================================================
// AUTOSWITCH
// ============================================================

func (a *App) GetAutoSwitchStatus() map[string]interface{} {
	return map[string]interface{}{
		"enabled":   a.cfg.GetAutoSwitchEnabled(),
		"threshold": a.cfg.GetAutoSwitchThresholdPct(),
		"interval":  a.cfg.GetAutoSwitchIntervalMin(),
	}
}

func (a *App) SetAutoSwitchEnabled(enabled bool) map[string]interface{} {
	if err := a.cfg.SetAutoSwitchEnabled(enabled); err != nil {
		return errResp(err.Error())
	}
	return okResp()
}

func (a *App) SetAutoSwitchConfig(threshold, interval int) map[string]interface{} {
	if threshold >= 10 && threshold <= 100 {
		if err := a.cfg.SetAutoSwitchThresholdPct(threshold); err != nil {
			return errResp(err.Error())
		}
	}
	if interval >= 1 && interval <= 60 {
		if err := a.cfg.SetAutoSwitchIntervalMin(interval); err != nil {
			return errResp(err.Error())
		}
	}
	return okResp()
}

func (a *App) GetAutoSwitchHistory() map[string]interface{} {
	jsonPath := filepath.Join(a.cfg.LogsDir(), "auto_switch_history.json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return map[string]interface{}{"records": []interface{}{}}
	}
	var records []map[string]interface{}
	if err := json.Unmarshal(data, &records); err != nil {
		return map[string]interface{}{"records": []interface{}{}}
	}
	return map[string]interface{}{"records": records}
}

func (a *App) GetStrategyChangeHistory() map[string]interface{} {
	logs, err := database.GetActionLogs("zapret", 100, 0)
	if err != nil {
		return errResp(err.Error())
	}
	var changes []map[string]interface{}
	for _, l := range logs {
		if l.Action == "strategy_applied" {
			parts := strings.SplitN(l.Details, "|", 2)
			strategy := ""
			source := ""
			if len(parts) > 0 {
				strategy = parts[0]
			}
			if len(parts) > 1 {
				source = parts[1]
			}
			changes = append(changes, map[string]interface{}{
				"timestamp": l.Timestamp,
				"strategy":  strategy,
				"source":    source,
			})
		}
	}
	return map[string]interface{}{"changes": changes}
}
