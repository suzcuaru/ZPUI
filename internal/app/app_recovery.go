package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/zapret"
)

// ============================================================
// ZAPRET SERVICE RECOVERY
// ============================================================
//
// Когда zapret не удаётся запустить (кнопкой, автозапуском или после краша
// службы Windows), запускается единая процедура восстановления. Она пошагово
// пытается вернуть службу к жизни и пушит прогресс во фронтенд через Wails
// events, чтобы показывать пользователю жёлтый тост с этапами.
//
// Шаги:
//  1. files   — проверка обязательных файлов; если их нет — авто-скачивание.
//  2. restart — попытка запуска/перезапуска службы (net start / Start()).
//  3. reinstall — если служба не отвечает — переустановка с текущей стратегией.
//  4. verify  — финальная проверка живости.
//  5. diagnostics собираются для отчёта при провале.
//
// По завершении пушится событие zapret:recovery:done с полным отчётом.
// При успехе счётчик крашей сбрасывается.

const (
	recoveryPollStep    = 1 * time.Second
	recoveryLivenessTO  = 8 * time.Second
	recoveryRestartWait = 4 * time.Second
)

// RecoveryStep — один этап процедуры восстановления (для отчёта и событий).
type RecoveryStep struct {
	Key     string `json:"key"`
	Message string `json:"message"`
	Status  string `json:"status"` // start | done | failed | skipped
}

// RecoveryReport — итоговый отчёт процедуры восстановления.
type RecoveryReport struct {
	Success     bool                   `json:"success"`
	ErrorCode   string                 `json:"error_code"`
	Reason      string                 `json:"reason"`
	Steps       []RecoveryStep         `json:"steps"`
	Diagnostics map[string]interface{} `json:"diagnostics"`
	FinalStatus string                 `json:"final_status"`
	InstallLog  []string               `json:"install_log"`
	Strategy    string                 `json:"strategy"`
}

var (
	recoveryMu      sync.Mutex
	recoveryRunning bool
)

// startZapretRecovery запускает процедуру восстановления в фоновой горутине,
// если она ещё не выполняется. Безопасна для вызова из любого триггера
// (кнопка, автозапуск, OnCrash, health monitor).
func (a *App) startZapretRecovery(reason string) {
	recoveryMu.Lock()
	if recoveryRunning {
		recoveryMu.Unlock()
		a.log.Info("recovery", "recovery already in progress, skip (reason: "+reason+")")
		return
	}
	recoveryRunning = true
	recoveryMu.Unlock()

	a.safeGo(func() {
		defer func() {
			recoveryMu.Lock()
			recoveryRunning = false
			recoveryMu.Unlock()
		}()
		a.recoverZapretService(reason)
	})
}

// isRecoveryRunning — экспонируется для фронта/других методов.
func (a *App) isRecoveryRunning() bool {
	recoveryMu.Lock()
	defer recoveryMu.Unlock()
	return recoveryRunning
}

func (a *App) emitRecoveryEvent(step RecoveryStep) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "zapret:recovery:step", step)
}

// waitForZapretRunning опрашивает статус zapret до тех пор, пока не увидит
// running или не истечёт таймаут. Возвращает true, если служба ожила.
func (a *App) waitForZapretRunning(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if a.zapret.GetStatus() == zapret.StatusRunning {
			return true
		}
		time.Sleep(recoveryPollStep)
	}
	return a.zapret.GetStatus() == zapret.StatusRunning
}

// zapretStartAndVerify запускает zapret и проверяет, что он действительно ожил.
// Используется кнопкой «Запустить» и автозапуском, чтобы не рапортовать об
// успехе, если служба упала в первые секунды после старта.
func (a *App) zapretStartAndVerify() error {
	if err := a.zapret.Start(); err != nil {
		return err
	}
	// Start() для сервисного режима возвращает nil сразу после `net start`,
	// без проверки живости — поэтому добираем опросом здесь.
	if !a.waitForZapretRunning(recoveryLivenessTO) {
		return fmt.Errorf("служба запрета не отвечает после запуска")
	}
	return nil
}

// recoverZapretService — основная процедура восстановления.
func (a *App) recoverZapretService(reason string) {
	a.log.Warn("recovery", "recovery started: "+reason)
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "zapret:recovery:start", map[string]interface{}{"reason": reason})
	}

	report := &RecoveryReport{
		Reason:   reason,
		Strategy: a.zapret.GetCurrentStrategy(),
	}
	steps := []RecoveryStep{}

	emit := func(key, msg, status string) {
		s := RecoveryStep{Key: key, Message: msg, Status: status}
		steps = append(steps, s)
		report.Steps = steps
		a.emitRecoveryEvent(s)
		a.log.Info("recovery", "["+key+"/"+status+"] "+msg)
	}

	// Шаг 1: проверка файлов.
	emit("files", "Проверка файлов zapret…", "start")
	vr := a.zapret.VerifyFiles()
	if vr.AllPresent {
		emit("files", "Файлы на месте", "skipped")
	} else {
		missing := []string{}
		for _, f := range vr.Files {
			if !f.Exists {
				missing = append(missing, f.Path)
			}
		}
		emit("files", fmt.Sprintf("Отсутствуют %d файлов — скачивание…", len(missing)), "start")
		if err := a.zapret.DownloadAndInstall(nil); err != nil {
			emit("files", "Скачивание не удалось: "+err.Error(), "failed")
			report.ErrorCode = "R-100"
			a.finishRecovery(report, false)
			return
		}
		a.zapret.RefreshVersion()
		if vr2 := a.zapret.VerifyFiles(); !vr2.AllPresent {
			emit("files", "После скачивания файлы всё ещё отсутствуют", "failed")
			report.ErrorCode = "R-101"
			a.finishRecovery(report, false)
			return
		}
		emit("files", "Файлы восстановлены", "done")
	}

	// Шаг 2: попытка (пере)запуска службы/процесса.
	emit("restart", "Попытка запуска службы…", "start")
	_ = a.zapret.Start() // Best-effort: в сервисном режиме делает net start.
	time.Sleep(recoveryRestartWait)
	if a.zapret.GetStatus() == zapret.StatusRunning {
		emit("restart", "Служба отвечает", "done")
		a.finishRecovery(report, true)
		return
	}
	emit("restart", "Служба не отвечает", "failed")

	// Шаг 3: переустановка службы с текущей стратегией.
	strategy := a.zapret.GetCurrentStrategy()
	if strategy == "" {
		strategy = a.zapret.DefaultStrategyName()
	}
	emit("reinstall", fmt.Sprintf("Переустановка службы (стратегия %s)…", strings.TrimSuffix(strategy, ".bat")), "start")
	res, err := a.zapret.InstallServiceLogged(strategy)
	if err != nil || res == nil || !res.Success {
		msg := "ошибка переустановки"
		if err != nil {
			msg = err.Error()
		} else if res != nil && len(res.Errors) > 0 {
			msg = strings.Join(res.Errors, "; ")
		}
		emit("reinstall", msg, "failed")
		report.ErrorCode = "R-300"
		a.finishRecovery(report, false)
		return
	}
	if res.Running && a.zapret.GetStatus() == zapret.StatusRunning {
		emit("reinstall", "Служба установлена и отвечает", "done")
		a.finishRecovery(report, true)
		return
	}
	emit("reinstall", "Служба установлена, но не отвечает", "failed")

	// Шаг 4: финальная проверка живости.
	emit("verify", "Финальная проверка…", "start")
	if a.waitForZapretRunning(recoveryLivenessTO) {
		emit("verify", "Служба ожила", "done")
		a.finishRecovery(report, true)
		return
	}
	emit("verify", "Служба не отвечает", "failed")
	report.ErrorCode = "R-400"
	a.finishRecovery(report, false)
}

// finishRecovery завершает процедуру: собирает диагностику/лог, пушит итоговое
// событие и сбрасывает счётчик крашей при успехе.
func (a *App) finishRecovery(report *RecoveryReport, success bool) {
	report.Success = success
	report.FinalStatus = string(a.zapret.GetStatus())
	if report.Strategy == "" {
		report.Strategy = a.zapret.GetCurrentStrategy()
	}

	// Диагностика и install.log — для отчёта при провале (и для успеха тоже,
	// но фронт показывает модалку только при провале).
	func() {
		defer func() {
			if r := recover(); r != nil {
				a.log.Error("recovery", fmt.Sprintf("diagnostics panic: %v\n%s", r, debug.Stack()))
			}
		}()
		report.Diagnostics = a.RunDiagnostics()
	}()
	report.InstallLog = a.readInstallLogLines()

	if success {
		a.cfg.SetServiceCrashCount(0)
		a.log.Info("recovery", "recovery SUCCESS — crash counter reset")
	} else {
		a.log.Error("recovery", "recovery FAILED ("+report.ErrorCode+")")
	}

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "zapret:recovery:done", map[string]interface{}{
			"success":    success,
			"error_code": report.ErrorCode,
			"reason":     report.Reason,
			"report": map[string]interface{}{
				"success":      report.Success,
				"error_code":   report.ErrorCode,
				"reason":       report.Reason,
				"steps":        report.Steps,
				"diagnostics":  report.Diagnostics,
				"final_status": report.FinalStatus,
				"install_log":  report.InstallLog,
				"strategy":     report.Strategy,
			},
		})
	}
}

func (a *App) readInstallLogLines() []string {
	logPath := filepath.Join(a.cfg.LogsDir(), "install.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return []string{}
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return []string{}
	}
	return strings.Split(content, "\n")
}

// RecoveryStatus — эндпоинт для фронта: идёт ли восстановление.
func (a *App) RecoveryStatus() map[string]interface{} {
	return map[string]interface{}{
		"running": a.isRecoveryRunning(),
	}
}
