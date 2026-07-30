package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zpui/internal/executil"
	"zpui/internal/updater"
)

// applyZPUI применяет обновление ZPUI: использует уже скачанный
// .zpui-update.zip (если есть, иначе скачивает), проверяет целостность,
// делает резервную копию, останавливает ZPUI.exe, устанавливает файлы,
// перезапускает ZPUI.
func applyZPUI(a *App) error {
	exeDir := a.exeDir
	zpuiExe := filepath.Join(exeDir, "ZPUI.exe")

	zipPath := filepath.Join(exeDir, ".zpui-update.zip")
	if _, err := os.Stat(zipPath); err != nil {
		a.applyProgress("downloading", 0)
		_, err := updater.DownloadZPUIUpdate(zipPath, func(pct int, downloaded, total int64) {
			a.applyProgress(fmt.Sprintf("Загрузка... %d%%", pct), pct)
		})
		if err != nil {
			return fmt.Errorf("загрузка не удалась: %w", err)
		}
	}

	remote, err := updater.FetchRemoteVersions()
	if err == nil {
		a.log.Info("selfupdate", fmt.Sprintf("Remote ZPUI version: %s", remote.ZPUI))
	}

	checksums := loadLocalChecksums(exeDir, a)

	if len(checksums) > 0 {
		a.applyProgress("Проверка целостности", 5)
		zipEntry, hasZip := checksums["zpui-win32.zip"]
		if !hasZip {
			zipEntry, hasZip = checksums["zpui-win64.zip"]
		}
		if hasZip && zipEntry.SHA256 != "" {
			ok, verr := updater.VerifyFileChecksum(zipPath, zipEntry.SHA256)
			if verr != nil {
				return fmt.Errorf("ошибка проверки: %w", verr)
			}
			if !ok {
				return fmt.Errorf("нарушение целостности: SHA-256 не совпадает")
			}
		}
	}

	a.applyProgress("Резервная копия", 15)
	backupDir := filepath.Join(exeDir, ".backup", "selfupdate_"+time.Now().Format("20060102_150405"))
	os.MkdirAll(backupDir, 0755)
	if err := backupCriticalFiles(exeDir, backupDir); err != nil {
		return fmt.Errorf("резервное копирование не удалось: %w", err)
	}

	a.applyProgress("Распаковка", 30)
	extractDir := filepath.Join(exeDir, ".update-extract")
	os.RemoveAll(extractDir)
	os.MkdirAll(extractDir, 0755)
	if err := unzipTo(zipPath, extractDir, ""); err != nil {
		return fmt.Errorf("распаковка не удалась: %w", err)
	}

	newZpuiExe := findFile(extractDir, "ZPUI.exe")
	if newZpuiExe == "" {
		return fmt.Errorf("ZPUI.exe не найден в архиве обновления")
	}

	if len(checksums) > 0 {
		if entry, ok := checksums["ZPUI.exe"]; ok && entry.SHA256 != "" {
			ok, verr := updater.VerifyFileChecksum(newZpuiExe, entry.SHA256)
			if verr != nil {
				return fmt.Errorf("ошибка проверки ZPUI.exe: %w", verr)
			}
			if !ok {
				return fmt.Errorf("ZPUI.exe: нарушение целостности")
			}
		}
	}

	a.applyProgress("Остановка ZPUI", 55)
	executil.HiddenCmd("taskkill", "/IM", "ZPUI.exe", "/F").Run()
	if !waitForProcessExit("ZPUI.exe", 10*time.Second) {
		return fmt.Errorf("не удалось остановить ZPUI (процесс не завершается)")
	}
	time.Sleep(1 * time.Second)

	a.applyProgress("Установка", 70)
	if err := installFiles(extractDir, exeDir); err != nil {
		restoreFromBackup(backupDir, exeDir)
		return fmt.Errorf("установка не удалась, восстановлено из резервной копии: %w", err)
	}

	os.RemoveAll(zipPath)
	os.RemoveAll(extractDir)

	if _, err := os.Stat(zpuiExe); err != nil {
		restoreFromBackup(backupDir, exeDir)
		return fmt.Errorf("ZPUI.exe отсутствует после установки, восстановлено из резервной копии")
	}

	if remote != nil && remote.ZPUI != "" {
		writeVersionMarker(exeDir, remote.ZPUI)
	}

	a.applyProgress("Запуск ZPUI", 90)
	cmd := executil.GuiCmd(zpuiExe)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("не удалось запустить ZPUI: %w", err)
	}

	a.applyProgress("Завершено", 100)
	return nil
}

func loadLocalChecksums(exeDir string, a *App) map[string]updater.ChecksumEntry {
	localPath := filepath.Join(exeDir, "checksums.sha256")
	data, err := os.ReadFile(localPath)
	if err != nil {
		a.log.Info("selfupdate", "Local checksums not found: "+err.Error())
		return nil
	}
	checksums := updater.ParseChecksums(data)
	a.log.Info("selfupdate", fmt.Sprintf("Loaded %d local checksums", len(checksums)))
	return checksums
}

func backupCriticalFiles(exeDir, backupDir string) error {
	criticalFiles := []string{"ZPUI.exe", "zpui.db", "config.json", "versions.json"}
	for _, f := range criticalFiles {
		src := filepath.Join(exeDir, f)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := filepath.Join(backupDir, f)
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("backup %s: %w", f, err)
		}
	}
	return nil
}

func restoreFromBackup(backupDir, exeDir string) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		src := filepath.Join(backupDir, e.Name())
		dst := filepath.Join(exeDir, e.Name())
		copyFile(src, dst)
	}
}

func installFiles(extractDir, exeDir string) error {
	selfName := ""
	if exe, err := os.Executable(); err == nil {
		selfName = strings.ToLower(filepath.Base(exe))
	}

	return filepath.Walk(extractDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(extractDir, path)
		baseName := strings.ToLower(filepath.Base(path))

		if selfName != "" && baseName == selfName {
			oldPath := filepath.Join(exeDir, baseName+".old")
			os.Remove(oldPath)
			os.Rename(filepath.Join(exeDir, baseName), oldPath)
		}

		dstPath := filepath.Join(exeDir, relPath)
		os.MkdirAll(filepath.Dir(dstPath), 0755)

		return copyFile(path, dstPath)
	})
}

func unzipTo(zipPath, dest, skipName string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	// Примечание: selfupdate.exe НЕ пропускаем намеренно. Архив ZPUI
	// (zpui-win32.zip) содержит Центр обновлений, поэтому его нужно
	// распаковать, чтобы installFiles() заменил запущенный exe через
	// rename-трюк (на Windows работающий .exe можно переименовать).
	for _, f := range r.File {
		baseName := strings.ToLower(filepath.Base(f.Name))
		if skipName != "" && baseName == strings.ToLower(skipName) {
			continue
		}

		path := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(path, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(path), 0755)

		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(path)
		if err != nil {
			rc.Close()
			return err
		}
		io.Copy(out, rc)
		out.Close()
		rc.Close()
	}
	return nil
}

func findFile(dir, name string) string {
	var found string
	lowerName := strings.ToLower(name)
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.ToLower(filepath.Base(path)) == lowerName {
			found = path
		}
		return nil
	})
	return found
}

func waitForProcessExit(processName string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cmd := executil.HiddenCmd("tasklist", "/FI", "IMAGENAME eq "+processName, "/NH")
		output, _ := cmd.Output()
		if !strings.Contains(string(output), processName) {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func writeVersionMarker(exeDir, version string) {
	markerPath := filepath.Join(exeDir, ".last_update")
	os.WriteFile(markerPath, []byte(version+"\n"), 0644)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
