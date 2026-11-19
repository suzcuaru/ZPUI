package updater

import (
	"net/http"
	"time"
)

// ============================================================
// Yandex Disk — канал обновлений.
//
// Public API: https://cloud-api.yandex.net/v1/disk/public/resources
//
// Раскладка публичной папки:
//   /                          - корень (public_key)
//     /ZPUI-Setup-<версия>-win32.exe  - установщики ZPUI (без папок версий)
//     /checksums.sha256         - хеши установщика и модулей
//     /components/<name>/       - модульные обновления (report)
//     /zapret/                  - легаси-zip zapret
//     /Zapret <версия>/         - папки zapret (скачиваются как zip)
// ============================================================

const (
	// YandexPublicURL — публичная ссылка на папку с обновлениями
	// (единственный канал установщиков ZPUI, один из двух каналов zapret).
	// Структура:
	//   /ZPUI-Setup-<версия>-win32.exe - установщики ZPUI
	//   /Zapret <версия>/    - папка с распакованным zapret (bin/, lists/, ...)
	YandexPublicURL = "https://disk.yandex.ru/d/mPqm8h8WwkQg-A"

	// yandexAPIBase — базовый URL публичного API Яндекс.Диска.
	yandexAPIBase = "https://cloud-api.yandex.net/v1/disk/public/resources"
)

// yandexClient — HTTP-клиент с таймаутом.
var yandexClient = &http.Client{Timeout: 20 * time.Second}
