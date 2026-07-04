package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	if err := a.zapret.Start(); err != nil {
		return errResp(err.Error())
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

// ============================================================
// GAME FILTER
// ============================================================

func (a *App) GetGameFilter() map[string]interface{} {
	mode, _, _ := a.zapret.LoadGameFilter()
	return map[string]interface{}{"mode": mode}
}

func (a *App) SetGameFilter(mode string) map[string]interface{} {
	if err := a.zapret.SetGameFilter(mode); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
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
	return downloadAndSave(url, listFile)
}

func (a *App) UpdateHosts() map[string]interface{} {
	url := "https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/refs/heads/main/.service/hosts"
	tmpFile := filepath.Join(os.TempDir(), "zapret_hosts.txt")
	return downloadAndSave(url, tmpFile)
}

// ============================================================
// FIRST RUN / ZAPRET MANAGEMENT
// ============================================================

func (a *App) HasLocalZapret() bool {
	winws := filepath.Join(a.cfg.GetZapretPath(), "bin", "winws.exe")
	_, err := os.Stat(winws)
	return err == nil
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

func (a *App) RunWizard() map[string]interface{} {
	exePath, err := os.Executable()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	exeDir := filepath.Dir(exePath)
	wizardPath := filepath.Join(exeDir, "wizard.exe")

	if _, err := os.Stat(wizardPath); err != nil {
		return map[string]interface{}{"error": "wizard.exe не найден"}
	}

	cmd := executil.HiddenCmd(wizardPath)
	if err := cmd.Start(); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	a.log.Info("app", "Wizard started (PID: "+strconv.Itoa(cmd.Process.Pid)+")")
	return map[string]interface{}{"status": "ok"}
}

func (a *App) CheckWizardDone() bool {
	return a.HasLocalZapret()
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
