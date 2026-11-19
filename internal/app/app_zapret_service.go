package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"


	"zpui/internal/database"
	"zpui/internal/executil"
)


// ============================================================
// WINDOWS SERVICE
// ============================================================

func (a *App) InstallService(strategy string) map[string]interface{} {
	if strategy == "" {
		strategy = a.zapret.GetCurrentStrategy()
	}
	a.opGen.Add(1)
	if err := a.zapret.InstallService(strategy); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"status": "installed"}
}

func (a *App) RemoveService() map[string]interface{} {
	a.opGen.Add(1)
	// Пользователь явно демонтирует службу — движок поднимать не нужно.
	a.zapretExpected.Store(false)
	a.zapret.RemoveService()
	return map[string]interface{}{"status": "removed"}
}

// GetZapretServiceAutostart — включён ли автозапуск службы запрета.
func (a *App) GetZapretServiceAutostart() map[string]interface{} {
	return okRespWithData(map[string]interface{}{"autostart": a.zapret.GetServiceAutostart()})
}

// SetZapretServiceAutostart — тумблер автозапуска службы запрета.
func (a *App) SetZapretServiceAutostart(enabled bool) map[string]interface{} {
	if err := a.zapret.SetServiceAutostart(enabled); err != nil {
		return errResp(err.Error())
	}
	return okRespWithData(map[string]interface{}{"autostart": enabled})
}

// InstallServiceLogged — устанавливает службу запрета, записывая процесс в
// logs/install.log (перезаписываемый), с проверкой что служба отвечает.
func (a *App) InstallServiceLogged(strategy string) map[string]interface{} {
	a.opGen.Add(1)
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
// FIRST RUN / ZAPRET MANAGEMENT
// ============================================================

func (a *App) HasLocalZapret() bool {
	return a.zapret.VerifyFiles().AllPresent
}

func (a *App) HasSystemZapretService() bool {
	output, err := executil.RunWithTimeout(5*time.Second, "sc", "query", "zapret")
	if err != nil {
		return false
	}
	return strings.Contains(output, "zapret")
}

func (a *App) RemoveSystemZapretService() map[string]interface{} {
	a.writeStartupLog("SETUP", "Удаление сторонней службы zapret...")
	a.zapret.Teardown()
	a.writeStartupLog("SETUP", "Сторонняя служба zapret удалена")
	return map[string]interface{}{"status": "ok"}
}

// ============================================================
// EXCLUSIONS (list-exclude-user.txt)
// ============================================================

// ============================================================
// FULL REINSTALL
// ============================================================

// FullReinstall — полная переустановка zapret: удаляет папку, скачивает заново.
// Пользовательские списки бекапятся и восстанавливаются.
func (a *App) FullReinstall() map[string]interface{} {
	a.log.Info("zapret", "Full reinstall started")
	// Деструктивная операция высшего приоритета: гасит идущие попытки
	// запуска/восстановления и переставляет движок с чистого листа.
	a.opGen.Add(1)
	a.zapretExpected.Store(false)
	database.SetZapretJustUpdated(true)

	snap := a.zapret.CaptureState()

	zapretDir := a.cfg.GetZapretPath()
	a.log.Info("zapret", "Removing zapret directory: "+zapretDir)

	// Снос движения целиком — через менеджер: служба + процессы + драйверы.
	a.zapret.Teardown()

	time.Sleep(2 * time.Second)

	if err := os.RemoveAll(zapretDir); err != nil {
		a.log.Warn("zapret", "Failed to remove zapret dir: "+err.Error())
	}

	if err := os.MkdirAll(zapretDir, 0755); err != nil {
		return errResp("не удалось создать папку: " + err.Error())
	}

	a.log.Info("zapret", "Reinstalling zapret...")
	installed := false
	// Сначала сохранённая копия последнего скачанного архива: переустановка
	// работает даже без сети. Не вышло — качаем заново.
	if zipPath := a.zapret.StoredArchivePath(); zipPath != "" {
		a.log.Info("zapret", "Using stored archive: "+zipPath)
		if err := a.zapret.InstallFromZip(zipPath, nil); err != nil {
			a.log.Warn("zapret", "Stored archive install failed: "+err.Error())
		} else {
			installed = true
		}
	}
	if !installed {
		a.log.Info("zapret", "Downloading fresh zapret...")
		if err := a.zapret.DownloadAndInstall(nil); err != nil {
			a.log.Error("zapret", "Download failed: "+err.Error())
			a.zapret.RestoreState(snap)
			return errResp("скачивание не удалось: " + err.Error())
		}
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
	} else {
		// Перестановка завершена запуском движка — дальше он «должен работать».
		a.zapretExpected.Store(true)
	}

	return map[string]interface{}{
		"status":   "ok",
		"version":  a.zapret.GetVersion(),
		"strategy": strategy,
	}
}
