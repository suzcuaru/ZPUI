package app

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"zpui/internal/executil"
	"zpui/internal/logdb"
	"zpui/internal/notify"
	"zpui/internal/updater"
)

const (
	strategyVerifyTimeout = 10 * time.Second
	// fullRestoreCooldown — минимальный интервал между полными переустановками
	// запрета (фаза 3: снос папки + скачивание). Без него любая причина, из-за
	// которой движок не стартует, превращает каждый запуск в полную переустановку.
	fullRestoreCooldown = 6 * time.Hour
)

func (a *App) RunStartupChecks(log *StartupLogger) map[string]interface{} {
	log.Write("STARTUP", "=== Запуск проверок ===")

	// 1. Проверка целостности приложения
	log.Write("APPCHECK", "Проверка целостности приложения...")
	a.verifyAppIntegrity(log)

	// 1b. Серверный набор алгоритма подбора: дельта по версии ДО этапа
	// auto-select — подбор и проверка стратегии идут уже на свежем наборе
	// (решение владельца 10.11.2026). Сети нет — тихо, работает прежний.
	a.RefreshAutoSelectCfg()
	log.Writef("APPCHECK", "autoselect config version: %d", autoselectLocalVersion())

	// 1c. Смена поколения движка (zapret2 → zapret1): один раз разрешаем
	// полную переустановку (PHASE3), сняв cooldown прошлой эпохи — иначе
	// миграция машины блокируется на 6 часов.
	if a.cfg.GetZapretGeneration() != "zapret1" {
		_ = a.cfg.SetZapretGeneration("zapret1")
		if a.cfg.GetLastFullZapretRestore() > 0 {
			_ = a.cfg.SetLastFullZapretRestore(0)
			log.Write("STARTUP", "Смена поколения движка — блокировка полной переустановки сброшена")
		}
	}

	// 2. Проверка целостности файлов zapret (с учётом активной стратегии)
	log.Write("ZAPRETCHECK", "Проверка файлов zapret...")
	// Раннее создание недостающих листов
	if a.zapret != nil {
		_ = a.zapret.EnsureUserLists()
	}
	a.ensureGoogleList()
	vr := a.zapret.VerifyFiles()
	zapretOk := vr.AllPresent

	if !zapretOk {
		missing := []string{}
		for _, f := range vr.Files {
			if !f.Exists {
				missing = append(missing, f.Path)
				log.Writef("ZAPRETCHECK", "MISSING_LIST: %s", f.Path)
			}
		}
		log.Writef("ZAPRETCHECK", "Отсутствуют файлы: %s", strings.Join(missing, ", "))
		// пытаемся создать недостающие user-листы
		if a.zapret != nil {
			_ = a.zapret.EnsureUserLists()
		}
		a.ensureGoogleList()
		// перепроверяем
		vr = a.zapret.VerifyFiles()
		zapretOk = vr.AllPresent
		if !zapretOk {
			missing = []string{}
			for _, f := range vr.Files {
				if !f.Exists {
					missing = append(missing, f.Path)
				}
			}
			log.Writef("ZAPRETCHECK", "Отсутствуют файлы: %s", strings.Join(missing, ", "))
		}
	}

	if zapretOk {
		zapretOk = a.verifyZapretIntegrity(log)
	}

	// 3. Если файлы zapret повреждены — сразу на скачивание
	if !zapretOk {
		log.Write("ZAPRETCHECK", "Файлы zapret повреждены — требуется переустановка")
		if !a.isNetworkAvailable() {
			log.Write("ZAPRETCHECK", "Сеть недоступна — скачивание невозможно")
			return map[string]interface{}{
				"status":    "critical",
				"healthy":   false,
				"errorCode": "R-001",
				"message":   "Файлы zapret повреждены, но сеть недоступна для скачивания",
			}
		}
		return a.runStartupRecovery(log)
	}

	// 4. Проверка службы
	if !a.HasSystemZapretService() {
		log.Write("SERVICE", "Служба zapret не найдена — пропускаем проверку")
		res := map[string]interface{}{
			"status":  "no_service",
			"healthy": false,
		}
		if !vr.AllPresent {
			res["message"] = "Отсутствуют файлы zapret. Требуется установка."
			res["needs_install"] = true
		}
		return res
	}

	// 5. Проверка статуса и стратегии
	if a.zapret.GetServiceStatus().Running {
		log.Write("SERVICE", "Служба zapret запущена")
		strategy := a.resolveStrategy()
		if strategy != "" {
			log.Writef("STRATEGY", "Стратегия: %s", strategy)
			a.cfg.SetServiceCrashCount(0)
			return map[string]interface{}{
				"status":   "running",
				"healthy":  true,
				"strategy": strategy,
			}
		}
		log.Write("STRATEGY", "Стратегия не определена")
		a.cfg.SetServiceCrashCount(0)
		return map[string]interface{}{
			"status":  "running",
			"healthy": true,
		}
	}

	// Ручной останов: пользователь выключил движок тумблером — восстановление
	// запускаем только если он «должен работать» (флаг переживает перезапуск
	// приложения). Иначе каждый старт приложения молча поднимал остановленный
	// движок. Повреждённые файлы (шаг 3 выше) чинятся и без запуска службы —
	// сюда попадаем только при целых файлах и остановленной службе.
	if !a.cfg.GetLastZapretState() && !a.zapretExpected.Load() {
		a.zapretExpected.Store(false)
		log.Write("SERVICE", "Движок был остановлен вручную — автозапуск не выполняем")
		return map[string]interface{}{
			"status":  "stopped_by_user",
			"healthy": true,
			"message": "Движок остановлен пользователем — автозапуск не выполняем",
		}
	}

	log.Write("SERVICE", "Служба zapret остановлена — запускаем восстановление")
	return a.runStartupRecovery(log)
}

func (a *App) runStartupRecovery(log *StartupLogger) map[string]interface{} {
	if devNoZapretAutostart {
		log.Write("RECOVERY", "Автозапуск движка отключён на время разработки (devNoZapretAutostart)")
		return map[string]interface{}{
			"status":  "stopped",
			"healthy": false,
			"message": "Автозапуск движка отключён на время разработки",
		}
	}
	log.Write("RECOVERY", "=== Восстановление zapret ===")

	tryStart := func(label string) (bool, string) {
		for i := 1; i <= 3; i++ {
			log.Writef(label, "Попытка %d/3", i)
			err := a.zapret.Start()
			if err != nil {
				log.Writef(label, "Start(): %v", err)
			}
			time.Sleep(2 * time.Second)

			if !a.zapret.GetServiceStatus().Running {
				continue
			}

			strategy := a.resolveStrategy()
			if strategy != "" {
				log.Writef(label, "✓ Запущен, стратегия: %s", strategy)
				return true, strategy
			}
			log.Write(label, "Запущен, ждём подтверждения...")
			time.Sleep(3 * time.Second)
			if a.zapret.GetServiceStatus().Running {
				log.Write(label, "✓ Подтверждён")
				return true, a.resolveStrategy()
			}
		}
		log.Write(label, "✗ Все попытки исчерпаны")
		return false, ""
	}

	// Фаза 1: запуск
	log.Write("PHASE1", "Фаза 1: запуск службы")
	a.logStartup("PHASE1", "Фаза 1: запуск службы")
	recoveryStart := time.Now()
	if ok, strategy := tryStart("PHASE1"); ok {
		a.cfg.SetServiceCrashCount(0)
		log.Write("DONE", "✓ Служба zapret успешно запущена")
		a.logStartup("DONE", "✓ Служба zapret запущена")
		return map[string]interface{}{
			"status":   "running",
			"healthy":  true,
			"message":  "Service started",
			"strategy": strategy,
		}
	}

	// Причина смерти движка снимается сразу после фазы 1: позже журнал
	// пополняют собственные записи восстановления, и «последней ошибкой»
	// становится уже не гибель winws.
	engineErr := a.lastWinwsError(recoveryStart)
	if engineErr != "" {
		log.Writef("PHASE1", "Причина смерти движка: %s", engineErr)
		a.logStartup("PHASE1", "Причина смерти движка: "+engineErr)
	}

	// Фаза 2: переустановка службы
	log.Write("PHASE2", "Фаза 2: переустановка службы")
	a.logStartup("PHASE2", "Фаза 2: переустановка службы")
	if engineErr != "" {
		log.Writef("PHASE2", "Последняя ошибка движка: %s", engineErr)
	}
	a.teardownForRecovery(false, log)
	strategy := a.resolveStrategy()
	log.Writef("PHASE2", "Стратегия: %s", strategy)

	if err := a.zapret.InstallService(strategy); err != nil {
		log.Writef("PHASE2", "InstallService(): %v", err)
	} else {
		log.Write("PHASE2", "Служба переустановлена")
		if ok, s := tryStart("PHASE2"); ok {
			a.cfg.SetServiceCrashCount(0)
			log.Write("DONE", "✓ Восстановлено после переустановки")
			return map[string]interface{}{
				"status":   "running",
				"healthy":  true,
				"message":  "Service reinstalled",
				"strategy": s,
			}
		}
	}

	// Фаза 3: восстановление из архива или скачивание
	hasArchive := a.zapret.BackupArchivePath() != "" || a.zapret.StoredArchivePath() != ""
	if !hasArchive && !a.isNetworkAvailable() {
		log.Write("PHASE3", "Фаза 3 пропущена: нет архива и сеть недоступна")
		return a.failResult(log, engineErr)
	}

	// Гард от петли восстановления: полная переустановка оставляет свежие
	// файлы и драйверы — если движок не завёлся и с ними, повторный снос на
	// каждом запуске ничего не лечит, только стирает пользовательские списки.
	if last := a.cfg.GetLastFullZapretRestore(); last > 0 && time.Since(time.Unix(last, 0)) < fullRestoreCooldown {
		log.Write("PHASE3", "Полная переустановка уже выполнялась недавно — повторять её при каждом запуске нельзя")
		return a.failResult(log, engineErr)
	}
	_ = a.cfg.SetLastFullZapretRestore(time.Now().Unix())

	log.Write("PHASE3", "Фаза 3: восстановление zapret из архива или сети")
	a.logStartup("PHASE3", "Фаза 3: восстановление zapret")
	a.teardownForRecovery(true, log)

	if err := a.pauseEngineWatchdogDuring(func() error {
		return a.installZapretFromBestSource()
	}); err != nil {
		log.Writef("PHASE3", "installZapretFromBestSource(): %v", err)
		return a.failResult(log, engineErr)
	}
	a.zapret.RefreshVersion()
	log.Writef("PHASE3", "Версия: %s", a.zapret.GetVersion())

	if err := a.zapret.InstallService(strategy); err != nil {
		log.Writef("PHASE3", "InstallService(): %v", err)
		return a.failResult(log, engineErr)
	}
	log.Write("PHASE3", "Служба установлена")

	if ok, s := tryStart("PHASE3"); ok {
		a.cfg.SetServiceCrashCount(0)
		log.Write("DONE", "✓ Zapret полностью переустановлен и запущен")
		if a.cfg.GetNotificationsEnabled() {
			notify.Show("ZPUI", "Запрет успешно восстановлен")
		}
		return map[string]interface{}{
			"status":   "running",
			"healthy":  true,
			"message":  "Full reinstall successful",
			"strategy": s,
		}
	}

	log.Write("PHASE3", "✗ Все способы восстановления исчерпаны")
	return a.failResult(log, engineErr)
}

func (a *App) verifyAppIntegrity(log *StartupLogger) {
	exeDir := a.getExeDir()
	checksumsPath := filepath.Join(exeDir, "checksums.sha256")
	data, err := os.ReadFile(checksumsPath)
	if err != nil {
		log.Writef("APPCHECK", "Файл checksums.sha256 не найден: %v", err)
		return
	}

	checksums := updater.ParseChecksums(data)
	if len(checksums) == 0 {
		log.Write("APPCHECK", "Нет записей для проверки")
		return
	}

	checked := 0
	corrupted := 0
	for name, entry := range checksums {
		if entry.SHA256 == "" {
			continue
		}
		fullPath := filepath.Join(exeDir, name)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			log.Writef("APPCHECK", "Отсутствует: %s", name)
			continue
		}
		ok, verr := updater.VerifyFileChecksum(fullPath, entry.SHA256)
		if verr != nil {
			log.Writef("APPCHECK", "Ошибка проверки %s: %v", name, verr)
			continue
		}
		if !ok {
			log.Writef("APPCHECK", "✗ НЕ СОВПАДАЕТ хеш: %s — файл повреждён", name)
			corrupted++
		}
		checked++
	}
	if corrupted > 0 {
		log.Writef("APPCHECK", "Итог: %d файлов проверено, %d повреждены", checked, corrupted)
	} else {
		log.Writef("APPCHECK", "Итог: %d файлов проверено, все целы", checked)
	}
}

func (a *App) verifyZapretIntegrity(log *StartupLogger) bool {
	zapretDir := a.cfg.GetZapretPath()
	files := []string{
		"bin/winws.exe",
		"bin/WinDivert.dll",
		"bin/WinDivert64.sys",
		"service.bat",
		"general.bat",
	}

	allOk := true
	for _, relPath := range files {
		fullPath := filepath.Join(zapretDir, relPath)
		info, err := os.Stat(fullPath)
		if os.IsNotExist(err) {
			log.Writef("ZAPRETCHECK", "Отсутствует: %s", relPath)
			allOk = false
			continue
		}
		if info.Size() == 0 {
			log.Writef("ZAPRETCHECK", "✗ Пустой файл: %s", relPath)
			allOk = false
			continue
		}
	}

	if allOk {
		winwsPath := filepath.Join(zapretDir, "bin/winws.exe")
		if a.isValidPE(winwsPath, log) {
			log.Write("ZAPRETCHECK", "winws.exe: валидный PE-файл")
		} else {
			log.Write("ZAPRETCHECK", "✗ winws.exe: НЕ является валидным PE-файлом")
			allOk = false
		}
		divertPath := filepath.Join(zapretDir, "bin/WinDivert.dll")
		if a.isValidPE(divertPath, log) {
			log.Write("ZAPRETCHECK", "WinDivert.dll: валидный PE-файл")
		} else {
			log.Write("ZAPRETCHECK", "✗ WinDivert.dll: НЕ является валидным PE-файлом")
			allOk = false
		}
	}

	return allOk
}

func (a *App) isValidPE(path string, log *StartupLogger) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	header := make([]byte, 2)
	_, err = f.Read(header)
	if err != nil {
		return false
	}
	return header[0] == 0x4D && header[1] == 0x5A // MZ
}

// logStartup пишет в startup.log И эмитит событие во фронтенд (для UI recovery log).
func (a *App) logStartup(phase, msg string) {
	if a.startupLog != nil {
		a.startupLog.Write(phase, msg)
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "setup:progress", map[string]interface{}{
			"phase":   phase,
			"message": msg,
		})
	}
}

// writeStartupLog пишет только в startup.log (без событий UI).
func (a *App) writeStartupLog(phase, msg string) {
	if a.startupLog != nil {
		a.startupLog.Write(phase, msg)
	}
}

func (a *App) resolveStrategy() string {
	s := a.cfg.GetCurrentStrategy()
	if s == "" {
		return a.zapret.DefaultStrategyName()
	}
	// Заданная стратегия удалена (миграция era zapret2, обновление дистрибутива)
	// — парсинг такого имени ломает установку службы, откатываемся на дефолт.
	if _, err := os.Stat(a.cfg.StrategyPath(s)); err != nil {
		return a.zapret.DefaultStrategyName()
	}
	return s
}

// resetWinDivertDrivers — выгрузка драйверов WinDivert из ядра. После жёсткой
// остановки winws (краш, обновление поверх работающего движка) старый драйвер
// остаётся загруженным, и заново запущенный winws умирает на открытии девайса —
// служба «запускается и сразу останавливается». Удалённые драйверы winws
// ставит заново из своих dll при следующем старте.
func (a *App) resetWinDivertDrivers() {
	executil.RunWithTimeout(10*time.Second, "taskkill", "/IM", "winws.exe", "/F")
	for _, drv := range []string{"WinDivert", "WinDivert14", "WinDivert2.2"} {
		executil.RunWithTimeout(10*time.Second, "sc", "stop", drv)
		executil.RunWithTimeout(10*time.Second, "sc", "delete", drv)
	}
	time.Sleep(1 * time.Second)
}

// lastWinwsError — последняя ошибка движка не старее since: почему winws
// не удержался. Свежие ERROR читаются с запасом и фильтруются по времени —
// иначе «причиной» становится ошибка недельной давности.
func (a *App) lastWinwsError(since time.Time) string {
	entries, err := a.log.ReadRecent(logdb.TableZapret, "ERROR", 20, 0)
	if err != nil || len(entries) == 0 {
		return ""
	}
	cutoff := since.Format("2006-01-02 15:04:05")
	for _, e := range entries {
		if e.Timestamp >= cutoff {
			return e.Message
		}
	}
	return ""
}

func (a *App) teardownForRecovery(full bool, log *StartupLogger) {
	a.zapret.Stop()
	a.zapret.RemoveService()
	a.resetWinDivertDrivers()

	if full {
		zapretDir := a.cfg.GetZapretPath()
		log.Writef("TEARDOWN", "Удаление папки: %s", zapretDir)
		os.RemoveAll(zapretDir)
		os.MkdirAll(zapretDir, 0755)
	}
}

// promoteZapretBackupLoop ждёт первого успешного запуска движка и только
// затем делает свежий архив от установщика/обновлятора основным. Так плохое
// обновление не затирает прошлую рабочую копию.
func (a *App) promoteZapretBackupLoop() {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	deadline := time.Now().Add(10 * time.Minute)
	for {
		select {
		case <-a.stopCh:
			return
		case <-t.C:
		}
		if time.Now().After(deadline) {
			return
		}
		if !a.zapret.GetServiceStatus().Running {
			continue
		}
		if err := a.zapret.PromotePendingBackup(); err != nil {
			a.log.Warn("zapret", "backup promote: "+err.Error())
		}
		return
	}
}

func (a *App) failResult(log *StartupLogger, reason string) map[string]interface{} {
	log.Write("FAIL", "✗ Все способы восстановления исчерпаны")
	if reason != "" {
		log.Writef("FAIL", "Последняя ошибка движка: %s", reason)
	}
	code := "R-000"
	msg := "Служба Запрета не может быть запущена."
	if hvciEnabled() {
		// Конкретная и самая частая причина на Win11: HVCI блокирует драйвер
		// WinDivert — даём пользователю точную инструкцию вместо «обратитесь
		// в поддержку».
		code = "R-000-HVCI"
		reason := "драйвер WinDivert заблокирован «Целостностью памяти» Windows"
		log.Write("FAIL", "✗ "+reason+" (HVCI включена)")
		log.Write("FAIL", "Отключите: Безопасность Windows → Безопасность устройства → Ядро → «Целостность памяти» → Выкл → перезагрузка. Затем включите запрет.")
		msg = "Драйвер запрета (WinDivert) заблокирован «Целостностью памяти» Windows. Отключите её: Безопасность Windows → Безопасность устройства → Ядро → «Целостность памяти» → Выкл → перезагрузите ПК → включите запрет."
		log.Writef("FAIL", "Код ошибки: %s", code)
		return map[string]interface{}{
			"status":    "critical",
			"healthy":   false,
			"errorCode": code,
			"message":   msg,
		}
	}
	log.Writef("FAIL", "Код ошибки: %s. Обратитесь в поддержку: https://github.com/suzcuaru/ZPUI/issues", code)
	msg += " Код ошибки: " + code + ". Обратитесь в поддержку: https://github.com/suzcuaru/ZPUI/issues"
	if reason != "" {
		msg += " Последняя ошибка движка: " + reason
	}
	return map[string]interface{}{
		"status":    "critical",
		"healthy":   false,
		"errorCode": code,
		"message":   msg,
	}
}
