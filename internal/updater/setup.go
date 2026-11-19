package updater

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ============================================================
// Обновление ZPUI через NSIS-установщик.
//
// Раскладка на Яндекс.Диске (единственный источник установщика):
//   /ZPUI-Setup-<версия>.exe      - универсальный установщик (x86+x64 в одном,
//                                   битность выбирается по системе при установке)
//   /ZPUI-Setup-<версия>-win32.exe - легаси-установщики прежних версий
//   /checksums.sha256            (строка вида <hash>  <file> (<size> bytes))
//   /components/report/...       (обновления модуля report)
//   /Zapret <версия>/            (zapret — как раньше)
// ============================================================

// Универсальный установщик без суффикса и легаси -win32 (прежние релизы
// на Диске остаются на месте и тоже находятся при листинге).
var setupNameRe = regexp.MustCompile(`(?i)^ZPUI-Setup-(\d+(?:\.\d+)*[a-z]?)(?:-win32)?\.exe$`)

// ZPUISetupFileName — каноничное имя установщика для версии.
func ZPUISetupFileName(version string) string {
	return fmt.Sprintf("ZPUI-Setup-%s.exe", version)
}

// ZPUISetupInfo — найденный установщик (Яндекс.Диск или релиз GitHub).
type ZPUISetupInfo struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	Source  string `json:"source"`
	// ChecksumsURL — файл checksums.sha256 рядом (только источник GitHub);
	// пусто — сверять нечем.
	ChecksumsURL string `json:"-"`
}

// CheckZPUISetupUpdate ищет самый свежий установщик ZPUI в корне публичной
// папки Яндекс.Диска (версия парсится из имени файла) и сравнивает с current.
func CheckZPUISetupUpdate(current string) (*ZPUISetupInfo, error) {
	items, err := listYandexDir(YandexPublicURL, "")
	if err != nil {
		return nil, fmt.Errorf("yandex list: %w", err)
	}

	bestVer, bestURL := "", ""
	for _, it := range items {
		if it.Type != "file" {
			continue
		}
		m := setupNameRe.FindStringSubmatch(it.Name)
		if m == nil {
			continue
		}
		if bestVer == "" || CompareVersions(m[1], bestVer) > 0 {
			bestVer = m[1]
			bestURL = it.File
		}
	}

	if bestVer == "" {
		return nil, fmt.Errorf("установщики ZPUI-Setup-*.exe не найдены на Яндекс.Диске")
	}

	return &ZPUISetupInfo{
		Version: bestVer,
		URL:     bestURL,
		Source:  "yandex",
	}, nil
}

// DownloadZPUISetup скачивает установщик в dest с прогрессом и проверкой
// SHA-256 по checksums.sha256 с Яндекс.Диска (если запись для файла есть).
func DownloadZPUISetup(info *ZPUISetupInfo, dest string, progress ProgressFunc) (*DownloadResult, error) {
	if info == nil || info.URL == "" {
		return nil, fmt.Errorf("нет URL установщика ZPUI")
	}

	if err := downloadFromYandexWithProgress(info.URL, dest, progress); err != nil {
		return nil, fmt.Errorf("yandex: %v", err)
	}

	sha, _ := ComputeSHA256(dest)
	res := &DownloadResult{
		Source:   "yandex",
		URL:      info.URL,
		SHA256:   sha,
		DestPath: dest,
	}
	if st, err := os.Stat(dest); err == nil {
		res.Size = st.Size()
	}

	// Checksums недоступны (сеть/зеркало лежит) — предупреждение и установка
	// продолжается; checksums получены, но записи нет или сумма не совпала —
	// отказ без установки.
	checksums, err := FetchChecksumsYandex()
	if err != nil {
		slog.Warn("checksums.sha256 недоступен, установщик принят без проверки", "error", err)
	} else {
		entry, ok := checksums[strings.ToLower(filepath.Base(dest))]
		if !ok {
			os.Remove(dest)
			return nil, fmt.Errorf("в checksums.sha256 нет записи для %s — файл удалён", filepath.Base(dest))
		}
		ok, verr := VerifyFileChecksum(dest, entry.SHA256)
		if verr != nil || !ok {
			os.Remove(dest)
			return nil, fmt.Errorf("контрольная сумма установщика не совпала — файл удалён")
		}
	}

	return res, nil
}
