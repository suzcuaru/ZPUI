package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"zpui/internal/updater"
)

// knownExes — список exe-файлов, которые должны присутствовать в директории.
// Всё что НЕ в этом списке — мусор от старых версий.
var knownExes = map[string]bool{
	"ZPUI.exe":       true,
	"selfupdate.exe": true,
	"ZPUI-x86.exe":   true,
	"uninstall.exe":  true,
}

// knownComponents — модули, которые могут лежать в components/, с файлами,
// подтверждающими валидность папки (достаточно любого одного). Компоненты
// без exe (wintun — только dll) не должны удаляться чистильщиком.
var knownComponents = map[string][]string{
	"selfupdate": {"selfupdate.exe"},
	// Служба ZPUI (watchdog) и драйвер wintun (VPN): чистка 26.10 стирала их
	// как «неизвестные», оставляя службу без бинарника для восстановления.
	"service": {"ZPUI-Service.exe", "version.txt", "checksum.sha256"},
	"wintun":  {"wintun_386.dll", "wintun_amd64.dll"},
	// Мост Telegram (дочерний exe из tg-ws-proxy): чистка 31.10 стирала его
	// сразу после установки — «tgproxy component not installed».
	"tgproxy": {"ZPUI-TgProxy.exe", "version.txt"},
	// Браузерное расширение: подписанный пакет (без exe), чистильщик не должен
	// его стирать как «неизвестный» — иначе установка в браузеры ломалась.
	"extension": {"zpui-extension.crx"},
}

// versionFolderRe — имя папки-версии вида «1.6.3» (остатки старого бага
// обновления, когда архив распаковывался в подпапку).
var versionFolderRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+)+$`)

// tempFiles — файлы/папки, которые создаются при обновлении и должны
// быть удалены при следующем запуске (остатки незавершённого обновления).
// ZPUI-Service.exe в корне — легаси-копия (служба живёт только в
// components\service), убирается у существующих установок 31.10.
var tempFiles = []string{
	".zpui-update.zip",
	".update-extract",
	".backup",
	"ZPUI-Service.exe",
}

// runStartupCleanup сканирует exe-файлы в директории приложения,
// проверяет целостность через checksums.sha256 и удаляет мусор.
func (a *App) runStartupCleanup() {
	exeDir := a.getExeDir()

	if a.startupLog != nil {
		a.startupLog.Write("CLEANUP", "Очистка временных файлов...")
	}
	a.cleanTempFiles(exeDir)
	a.cleanTempFiles(filepath.Join(exeDir, "components"))

	if a.startupLog != nil {
		a.startupLog.Write("CLEANUP", "Проверка целостности exe-файлов...")
	}
	a.verifyAndCleanExes(exeDir)
	a.cleanComponents(exeDir)
	a.cleanVersionFolders(exeDir)
	a.cleanOrphanListFiles(exeDir)

	// Проверка и удаление стороннего zapret (только при первом запуске)
	if !a.cfg.GetZapretSkipped() && !a.HasLocalZapret() {
		if a.startupLog != nil {
			a.startupLog.Write("THIRDPARTY", "Проверка стороннего zapret...")
		}
		res := a.DetectThirdPartyZapret()
		if has, _ := res["has_third_party"].(bool); has {
			if a.startupLog != nil {
				a.startupLog.Write("THIRDPARTY", "Обнаружен сторонний zapret — удаление...")
			}
			a.RemoveThirdPartyZapret()
			if a.startupLog != nil {
				a.startupLog.Write("THIRDPARTY", "Сторонний zapret удалён")
			}
		} else if a.startupLog != nil {
			a.startupLog.Write("THIRDPARTY", "Сторонний zapret не обнаружен")
		}
	}

	if a.startupLog != nil {
		a.startupLog.Write("BOOT", "Инициализация завершена")
	}
}

// cleanOrphanListFiles — осиротевшие файлы листов от прежних сборок:
// *.backup в lists оставляла старая сборка (xcopy забирал папку целиком).
// list-google.txt не удаляем: на него ссылается general (ALT).bat.
func (a *App) cleanOrphanListFiles(exeDir string) {
	p := filepath.Join(exeDir, "zapret", "lists", "ipset-all.txt.backup")
	if err := os.Remove(p); err == nil && a.startupLog != nil {
		a.startupLog.Write("CLEANUP", "Удалён осиротевший файл списка: "+filepath.Base(p))
	}
}

// cleanTempFiles удаляет временные файлы от предыдущих обновлений.
func (a *App) cleanTempFiles(dir string) {
	for _, name := range tempFiles {
		p := filepath.Join(dir, name)
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if info.IsDir() {
			os.RemoveAll(p)
			a.log.Info("cleanup", "Removed temp dir: "+name)
		} else {
			os.Remove(p)
			a.log.Info("cleanup", "Removed temp file: "+name)
		}
	}
}

// cleanComponents проверяет components/<name>/ — удаляет мусор,
// проверяет целостность модулей.
func (a *App) cleanComponents(exeDir string) {
	compDir := filepath.Join(exeDir, "components")
	entries, err := os.ReadDir(compDir)
	if err != nil {
		return // нет папки components — не ошибка
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		files, known := knownComponents[name]
		if !known {
			// Неизвестный компонент — удаляем
			os.RemoveAll(filepath.Join(compDir, name))
			a.log.Info("cleanup", "Removed unknown component: "+name)
			continue
		}
		// Известный компонент валиден, если на месте хоть один его файл
		found := false
		for _, f := range files {
			if _, err := os.Stat(filepath.Join(compDir, name, f)); err == nil {
				found = true
				break
			}
		}
		if !found {
			a.log.Warn("cleanup", "Component dir "+name+" is empty or incomplete, removing")
			os.RemoveAll(filepath.Join(compDir, name))
		}
	}
}

// verifyAndCleanExes проверяет SHA-256 всех exe-файлов и удаляет
// те, которых нет в списке knownExes (мусор от старых версий).
func (a *App) verifyAndCleanExes(exeDir string) {
	entries, err := os.ReadDir(exeDir)
	if err != nil {
		a.log.Warn("cleanup", "Cannot scan exeDir: "+err.Error())
		return
	}

	// Загружаем checksums если есть
	var checksums map[string]updater.ChecksumEntry
	if data, err := os.ReadFile(filepath.Join(exeDir, "checksums.sha256")); err == nil {
		checksums = updater.ParseChecksums(data)
	}

	deleted := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".exe") {
			continue
		}

		// Известный exe — проверяем checksum
		if knownExes[name] {
			if checksums != nil {
				if entry, ok := checksums[name]; ok && entry.SHA256 != "" {
					fullPath := filepath.Join(exeDir, name)
					ok, verr := updater.VerifyFileChecksum(fullPath, entry.SHA256)
					if verr != nil {
						a.log.Warn("cleanup", fmt.Sprintf("Checksum error %s: %v", name, verr))
					} else if !ok {
						a.log.Warn("cleanup", fmt.Sprintf("CHECKSUM MISMATCH: %s — file may be corrupted or tampered", name))
					}
				}
			}
			continue
		}

		// Неизвестный exe — удаляем (мусор от старых версий)
		fullPath := filepath.Join(exeDir, name)
		if err := os.Remove(fullPath); err != nil {
			a.log.Warn("cleanup", fmt.Sprintf("Failed to remove stale exe %s: %v", name, err))
		} else {
			a.log.Info("cleanup", "Removed stale exe: "+name)
			deleted++
		}
	}

	// Удаляем неактуальные модули (wizard, autoselect, zapretupdate)
	staleModules := []string{"wizard.exe", "autoselect.exe", "zapretupdate.exe"}
	for _, name := range staleModules {
		p := filepath.Join(exeDir, name)
		if _, err := os.Stat(p); err == nil {
			os.Remove(p)
			a.log.Info("cleanup", "Removed deprecated module: "+name)
			deleted++
		}
	}

	// Удаляем устаревшие .lnk файлы автозагрузки
	lnkPath := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "ZPUI.lnk")
	if _, err := os.Stat(lnkPath); err == nil {
		os.Remove(lnkPath)
		a.log.Info("cleanup", "Removed legacy startup shortcut: ZPUI.lnk")
		deleted++
	}

	if deleted > 0 {
		a.log.Info("cleanup", fmt.Sprintf("Startup cleanup complete: removed %d stale files", deleted))
	}
}

// cleanVersionFolders удаляет папки версий («1.6.2», «1.6.3», …) в папке
// приложения — остатки старого бага обновления, когда архив распаковывался
// в подпапку вместо корня.
func (a *App) cleanVersionFolders(exeDir string) {
	entries, err := os.ReadDir(exeDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !versionFolderRe.MatchString(e.Name()) {
			continue
		}
		p := filepath.Join(exeDir, e.Name())
		if err := os.RemoveAll(p); err == nil {
			a.log.Info("cleanup", "Removed leftover version folder: "+e.Name())
			if a.startupLog != nil {
				a.startupLog.Write("CLEANUP", "Удалена папка-остаток обновления: "+e.Name())
			}
		}
	}
}
