package pkginst

// Чистка СВОЕГО мусора (спеки Документы/Задачи/2026-11-20-update-download-fix-212
// и дополнение 2.1.3): установщики/обновляторы и недокачанные .part в %TEMP%,
// временные CHANGELOG-файлы, архив запрета, утёкшие снимки BackupUserFiles и
// хлам старых версий в корне установки. Чужие файлы и каталоги не
// соответствующие шаблонам не трогаются, возрастного гейта нет (шаблоны —
// только наши имена). Best-effort: занятые файлы (запущенный установщик)
// удалить не выйдет — ошибка учитывается в skipped.

import (
	"os"
	"path/filepath"
)

// tempLeftoverPatterns — шаблоны мусора ZPUI в %TEMP%. zapret-*.zip — это
// zapret-update.zip из internal/zapret (остаётся после ручной установки zip);
// zpui-user-* — снимки BackupUserFiles (os.MkdirTemp("", "zpui-user-"),
// pkginst.go), теряющиеся при краше до Restore.
var tempLeftoverPatterns = []string{
	"ZPUI-Setup-*.exe",
	"ZPUI-Update-*.exe",
	"ZPUI-*.part",
	"ZPUI-CHANGELOG-*",
	"zapret-*.zip",
	"zpui-user-*",
}

// exeDirLeftovers — наш хлам в корне установки: обновляторы и легаси старых
// версий из списка RemoveInstallation (selfupdate/security/wizard — в текущий
// payload не входят). Живой payload (ZPUI.exe, ZPUI-Service*, uninstall.exe,
// данные, zapret-backup.zip) не соответствует шаблонам и не трогается.
var exeDirLeftovers = []string{
	"ZPUI-Update-*.exe",
	"selfupdate.exe",
	"security.exe",
	"wizard.exe",
}

// CleanupUpdateLeftovers удаляет остатки прошлых обновлений и свой мусор в
// %TEMP%. freed — байты удалённых обычных файлов (каталоги в размер не
// считаются), removed — записи, skipped — занятые/несоздаваемые.
func CleanupUpdateLeftovers(exeDir string) (freed int64, removed, skipped int) {
	return cleanupLeftovers(os.TempDir(), exeDir)
}

func cleanupLeftovers(tempDir, exeDir string) (freed int64, removed, skipped int) {
	freed, removed, skipped = removeGlob(tempDir, tempLeftoverPatterns...)
	if exeDir != "" {
		f, r, s := removeGlob(exeDir, exeDirLeftovers...)
		freed += f
		removed += r
		skipped += s
	}
	return freed, removed, skipped
}

func removeGlob(dir string, patterns ...string) (freed int64, removed, skipped int) {
	for _, p := range patterns {
		matches, err := filepath.Glob(filepath.Join(dir, p))
		if err != nil {
			continue
		}
		for _, m := range matches {
			info, err := os.Lstat(m)
			if err != nil {
				skipped++
				continue
			}
			if info.IsDir() {
				if err := os.RemoveAll(m); err != nil {
					skipped++
					continue
				}
				removed++
				continue
			}
			if err := os.Remove(m); err != nil {
				skipped++
				continue
			}
			removed++
			if info.Mode().IsRegular() {
				freed += info.Size()
			}
		}
	}
	return freed, removed, skipped
}
