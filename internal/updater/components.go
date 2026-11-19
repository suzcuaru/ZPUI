package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)


// ============================================================
// Independent component updates (/components/<name>/)
//
// Структура на Яндекс.Диске:
//   /components/<name>/version.txt  — версия модуля (одна строка)
//   /components/<name>/<name>.exe   — исполняемый файл
//   /components/<name>/checksum.sha256 — SHA-256 хеш файла
// ============================================================

// componentDownloadURL ищет файл модуля на Яндекс.Диске. Поддерживает новую
// структуру /<версия>/components/<name>/ и старую /components/<name>/ в корне.
// Возвращает прямую ссылку или пустую строку.
func componentDownloadURL(name, filename string) string {
	if items, err := listYandexDir(YandexPublicURL, ""); err == nil {
		for _, folder := range versionFolders(items) {
			if res, ferr := findYandexFile(YandexPublicURL, folder.Path+"/components/"+name, filename); ferr == nil && res != nil && res.File != "" {
				return res.File
			}
		}
	}
	if res, err := findYandexFile(YandexPublicURL, "/components/"+name, filename); err == nil && res != nil {
		return res.File
	}
	return ""
}

// FetchComponentVersion получает версию модуля из components/<name>/version.txt.
func FetchComponentVersion(name string) (string, error) {
	url := componentDownloadURL(name, "version.txt")
	if url == "" {
		return "", nil
	}
	tmpFile := os.TempDir() + "\\zpui_comp_ver.txt"
	if err := downloadFromYandex(url, tmpFile); err != nil {
		return "", err
	}
	data, err := os.ReadFile(tmpFile)
	os.Remove(tmpFile)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// DownloadComponentFile скачивает файл модуля из components/<name>/ на Яндекс.Диске.
func DownloadComponentFile(name, filename, dest string) error {
	url := componentDownloadURL(name, filename)
	if url == "" {
		return fmt.Errorf("component %s: файл %s не найден на Яндекс.Диске", name, filename)
	}
	return downloadFromYandex(url, dest)
}

// refreshModuleChecksum — best-effort синхронизация строки файла в
// checksums.sha256 с фактическим содержимым заменённого модуля.
func refreshModuleChecksum(exeDir, fileName string) {
	path := filepath.Join(exeDir, fileName)
	sha, err := ComputeSHA256(path)
	if err != nil {
		return
	}
	var size int64
	if st, serr := os.Stat(path); serr == nil {
		size = st.Size()
	}
	_ = UpdateChecksumEntry(exeDir, fileName, sha, size)
}
