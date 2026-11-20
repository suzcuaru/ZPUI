package pkginst

// Чистка мусора обновлений (спека Документы/Задачи/2026-11-20-update-download-fix-212):
// установщики/обновляторы и недокачанные .part в %TEMP%, временные
// CHANGELOG-файлы, архив запрета и ZPUI-Update-*.exe рядом с ZPUI.exe.
// Best-effort: занятые файлы (запущенный установщик) удалить не выйдет —
// ошибка игнорируется.

import (
	"os"
	"path/filepath"
)

// tempLeftoverPatterns — шаблоны мусора в %TEMP%. zapret-*.zip — это
// zapret-update.zip из internal/zapret (остаётся после ручной установки zip).
var tempLeftoverPatterns = []string{
	"ZPUI-Setup-*.exe",
	"ZPUI-Update-*.exe",
	"ZPUI-*.part",
	"ZPUI-CHANGELOG-*",
	"zapret-*.zip",
}

// CleanupUpdateLeftovers удаляет остатки прошлых обновлений.
func CleanupUpdateLeftovers(exeDir string) {
	cleanupLeftovers(os.TempDir(), exeDir)
}

func cleanupLeftovers(tempDir, exeDir string) {
	for _, p := range tempLeftoverPatterns {
		matches, err := filepath.Glob(filepath.Join(tempDir, p))
		if err != nil {
			continue
		}
		for _, m := range matches {
			_ = os.Remove(m)
		}
	}
	if exeDir == "" {
		return
	}
	matches, err := filepath.Glob(filepath.Join(exeDir, "ZPUI-Update-*.exe"))
	if err != nil {
		return
	}
	for _, m := range matches {
		_ = os.Remove(m)
	}
}
