package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"zpui/internal/updater"
)

// knownExes — список exe-файлов, которые должны присутствовать в директории.
// Всё что НЕ в этом списке — мусор от старых версий.
var knownExes = map[string]bool{
	"ZPUI.exe":       true,
	"selfupdate.exe": true,
	"report.exe":     true,
	"security.exe":   true,
}

// knownComponents — список модулей, которые могут лежать в components/.
var knownComponents = map[string]bool{
	"selfupdate": true,
	"report":     true,
	"security":   true,
}

// tempFiles — файлы/папки, которые создаются при обновлении и должны
// быть удалены при следующем запуске (остатки незавершённого обновления).
var tempFiles = []string{
	".zpui-update.zip",
	".update-extract",
	".backup",
}

// runStartupCleanup сканирует exe-файлы в директории приложения,
// проверяет целостность через checksums.sha256 и удаляет мусор.
func (a *App) runStartupCleanup() {
	exeDir := a.getExeDir()

	a.cleanTempFiles(exeDir)
	a.cleanTempFiles(filepath.Join(exeDir, "components"))
	a.verifyAndCleanExes(exeDir)
	a.cleanComponents(exeDir)
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
		if knownComponents[name] {
			// Проверяем что exe на месте
			exePath := filepath.Join(compDir, name, name+".exe")
			if _, err := os.Stat(exePath); err != nil {
				a.log.Warn("cleanup", "Component dir "+name+" exists but exe missing, removing")
				os.RemoveAll(filepath.Join(compDir, name))
			}
			continue
		}
		// Неизвестный компонент — удаляем
		os.RemoveAll(filepath.Join(compDir, name))
		a.log.Info("cleanup", "Removed unknown component: "+name)
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
