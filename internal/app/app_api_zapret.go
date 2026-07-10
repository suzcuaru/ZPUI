package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/database"
	"zpui/internal/executil"
	"zpui/internal/notify"
	"zpui/internal/wizard"
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

// GetAutoTestResults — результаты последнего автотеста/автоподбора из JSON.
func (a *App) GetAutoTestResults() map[string]interface{} {
	jsonPath := filepath.Join(a.cfg.LogsDir(), "auto_test_results.json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return map[string]interface{}{"results": []interface{}{}}
	}
	var results []map[string]interface{}
	if err := json.Unmarshal(data, &results); err != nil {
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

var wizardRunning atomic.Bool

func (a *App) RunWizard() map[string]interface{} {
	if !wizardRunning.CompareAndSwap(false, true) {
		return errResp("wizard уже запущен")
	}

	go func() {
		defer wizardRunning.Store(false)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		_, err := wizard.Run(ctx, wizard.Options{
			ExeDir: a.exeDir,
			Config: a.cfg,
			Log:    a.log,
			OnProgress: func(p wizard.Progress) {
				if a.ctx != nil {
					runtime.EventsEmit(a.ctx, "wizard:progress", p)
				}
			},
		})

		if a.ctx == nil {
			return
		}
		if err != nil {
			a.log.Error("wizard", "Wizard failed: "+err.Error())
			runtime.EventsEmit(a.ctx, "wizard:done", map[string]interface{}{"error": err.Error()})
			return
		}
		runtime.EventsEmit(a.ctx, "wizard:done", map[string]interface{}{"status": "ok"})
	}()

	a.log.Info("app", "Wizard started (in-process)")
	return okResp()
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
