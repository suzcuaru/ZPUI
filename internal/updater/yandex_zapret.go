package updater

import (
	"fmt"
	"strings"
)

// Раздача запрета на Яндекс.Диске: папки «Zapret <версия>» с zip-архивом
// zapret-discord-youtube-*.zip внутри. Источник yandex для zapret
// (yandex_zapret в цепочке обновлений).

// ZapretYandexInfo — найденная на Диске версия запрета и прямой URL архива.
type ZapretYandexInfo struct {
	Version string
	URL     string
}

// extractVersionFromZapretFolder extracts version from folder name.
// "Zapret 1.9.9d" -> "1.9.9d", "Zapret" -> ""
func extractVersionFromZapretFolder(name string) string {
	lower := strings.ToLower(name)
	lower = strings.TrimPrefix(lower, "zapret")
	lower = strings.TrimSpace(lower)
	return lower
}

// CheckZapretYandexUpdate ищет самую свежую папку «Zapret <версия>» в корне
// публичной папки и zip-архив релиза внутри неё.
func CheckZapretYandexUpdate() (*ZapretYandexInfo, error) {
	items, err := listYandexDir(YandexPublicURL, "")
	if err != nil {
		return nil, fmt.Errorf("yandex list: %w", err)
	}
	bestVer, bestPath := "", ""
	for _, it := range items {
		if it.Type != "dir" {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(it.Name), "zapret") {
			continue
		}
		ver := extractVersionFromZapretFolder(it.Name)
		if ver == "" {
			continue
		}
		if bestVer == "" || CompareVersions(ver, bestVer) > 0 {
			bestVer, bestPath = ver, it.Path
		}
	}
	if bestVer == "" {
		return nil, fmt.Errorf("папки «Zapret <версия>» на Яндекс.Диске не найдены")
	}
	file, _, err := yandexDownloadURLByPrefix(YandexPublicURL, bestPath, "zapret-discord-youtube-")
	if err != nil {
		return nil, err
	}
	if file == "" {
		return nil, fmt.Errorf("в папке «Zapret %s» нет архива zapret-discord-youtube-*.zip", bestVer)
	}
	return &ZapretYandexInfo{Version: bestVer, URL: file}, nil
}
