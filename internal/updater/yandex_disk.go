package updater

import (
        "encoding/json"
        "fmt"
        "io"
        "net/http"
        "net/url"
        "os"
        "strings"
        "time"
)

// ============================================================
// Yandex Disk fallback for update downloads.
//
// Used when GitHub is unreachable (e.g. blocked by DPI/RKN).
// Public API: https://cloud-api.yandex.net/v1/disk/public/resources
//
// Folder structure expected on Yandex Disk:
//   /                          - root (this public_key)
//     /zpui.exe                - latest ZPUI build
//     /wizard.exe
//     /autoselect.exe
//     /selfupdate.exe
//     /zapretupdate.exe
//     /versions.json           - same format as GitHub versions.json
//     /zapret/                 - latest zapret-discord-youtube zip
//       /zapret-discord-youtube-<version>.zip
//     /mods/                   - mod files
// ============================================================

const (
        // YandexPublicURL — публичная ссылка на папку с обновлениями.
        // Используется как fallback когда GitHub недоступен (DPI/RKN).
        // Структура:
        //   /<версия>/           - папка с версией ZPUI (versions.json + *.exe)
        //   /Zapret <версия>/    - папка с распакованным zapret (bin/, lists/, ...)
        //   /ZPUI-Setup-*.exe    - установщик
        YandexPublicURL = "https://disk.yandex.ru/d/mPqm8h8WwkQg-A"

        // yandexAPIBase — базовый URL публичного API Яндекс.Диска.
        yandexAPIBase = "https://cloud-api.yandex.net/v1/disk/public/resources"
)

// yandexResource — элемент листинга публичной папки Яндекс.Диска.
// Только нужные поля, остальные игнорируются.
type yandexResource struct {
        Name  string `json:"name"`
        Type  string `json:"type"`  // "file" или "dir"
        Path  string `json:"path"`  // путь внутри публичной папки
        Size  int64  `json:"size"`
        File  string `json:"file"`  // прямой URL для скачивания (только для файлов)
        Md5   string `json:"md5"`
        Modified string `json:"modified"`
}

type yandexResourcesResponse struct {
        Name    string           `json:"name"`
        Type    string           `json:"type"`
        Path    string           `json:"path"`
        Embedded *struct {
                Items []yandexResource `json:"items"`
                Total int              `json:"total"`
        } `json:"_embedded"`
}

// yandexClient — HTTP-клиент с таймаутом.
var yandexClient = &http.Client{Timeout: 20 * time.Second}

// listYandexDir возвращает содержимое папки по публичному ключу.
// path = "" для корня, или "/zapret" для подпапки.
func listYandexDir(publicKey, path string) ([]yandexResource, error) {
        q := url.Values{}
        q.Set("public_key", publicKey)
        if path != "" {
                q.Set("path", path)
        }
        q.Set("limit", "200") // достаточный лимит для одной папки

        reqURL := yandexAPIBase + "?" + q.Encode()
        req, err := http.NewRequest("GET", reqURL, nil)
        if err != nil {
                return nil, err
        }
        req.Header.Set("User-Agent", userAgent)
        req.Header.Set("Accept", "application/json")

        resp, err := yandexClient.Do(req)
        if err != nil {
                return nil, fmt.Errorf("yandex api: %w", err)
        }
        defer resp.Body.Close()

        if resp.StatusCode != http.StatusOK {
                body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
                return nil, fmt.Errorf("yandex api: статус %d, тело: %s", resp.StatusCode, string(body))
        }

        var r yandexResourcesResponse
        if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
                return nil, fmt.Errorf("yandex api parse: %w", err)
        }
        if r.Embedded == nil {
                return nil, nil
        }
        return r.Embedded.Items, nil
}

// findYandexFile ищет файл по имени в публичной папке.
// Если path = "" — ищет в корне, иначе в подпапке (например "/zapret").
// Возвращает ресурс или nil если не найден.
func findYandexFile(publicKey, path, filename string) (*yandexResource, error) {
        items, err := listYandexDir(publicKey, path)
        if err != nil {
                return nil, err
        }
        for i, it := range items {
                if it.Type == "file" && strings.EqualFold(it.Name, filename) {
                        return &items[i], nil
                }
        }
        return nil, nil
}

// findYandexFileByPrefix ищет файл по префиксу имени (например "zapret-discord-youtube-"
// для поиска zip-архива с версией). Возвращает первый совпавший.
func findYandexFileByPrefix(publicKey, path, prefix string) (*yandexResource, error) {
        items, err := listYandexDir(publicKey, path)
        if err != nil {
                return nil, err
        }
        for i, it := range items {
                if it.Type == "file" && strings.HasPrefix(strings.ToLower(it.Name), strings.ToLower(prefix)) {
                        return &items[i], nil
                }
        }
        return nil, nil
}

// fetchYandexVersions скачивает versions.json с Яндекс.Диска и парсит его.
// Поддерживает 2 структуры:
//   - Новая: versions.json в папке /<version>/ (ищет самую свежую версию)
//   - Старая: versions.json в корне
func fetchYandexVersions(publicKey string) (*RemoteVersions, error) {
        if publicKey == "" {
                return nil, fmt.Errorf("yandex public key is empty")
        }

        // 1. Try new format: find newest version folder in root
        items, err := listYandexDir(publicKey, "")
        if err == nil {
                var bestVersionFolder *yandexResource
                for i, it := range items {
                        if it.Type != "dir" {
                                continue
                        }
                        // Skip "Zapret <version>" folders - we want version folders like "1.4.36"
                        lowerName := strings.ToLower(it.Name)
                        if strings.HasPrefix(lowerName, "zapret ") || lowerName == "zapret" {
                                continue
                        }
                        // Check if name looks like a version (starts with digit)
                        if len(it.Name) == 0 || it.Name[0] < '0' || it.Name[0] > '9' {
                                continue
                        }
                        // Compare versions
                        if bestVersionFolder == nil {
                                bestVersionFolder = &items[i]
                        } else {
                                if CompareVersions(it.Name, bestVersionFolder.Name) > 0 {
                                        bestVersionFolder = &items[i]
                                }
                        }
                }

                if bestVersionFolder != nil {
                        // Find versions.json inside this folder
                        res, ferr := findYandexFile(publicKey, bestVersionFolder.Path, "versions.json")
                        if ferr == nil && res != nil && res.File != "" {
                                resp, err := yandexClient.Get(res.File)
                                if err == nil && resp.StatusCode == http.StatusOK {
                                        var rv RemoteVersions
                                        if jsonErr := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rv); jsonErr == nil {
                                                resp.Body.Close()
                                                return &rv, nil
                                        }
                                }
                                if resp != nil {
                                        resp.Body.Close()
                                }
                        }
                }
        }

        // 2. Fallback: old format - versions.json in root
        res, err := findYandexFile(publicKey, "", "versions.json")
        if err != nil {
                return nil, fmt.Errorf("yandex list: %w", err)
        }
        if res == nil || res.File == "" {
                return nil, fmt.Errorf("versions.json not found on Yandex Disk")
        }

        resp, err := yandexClient.Get(res.File)
        if err != nil {
                return nil, fmt.Errorf("yandex download versions.json: %w", err)
        }
        defer resp.Body.Close()
        if resp.StatusCode != http.StatusOK {
                return nil, fmt.Errorf("yandex versions.json: status %d", resp.StatusCode)
        }

        var rv RemoteVersions
        if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rv); err != nil {
                return nil, fmt.Errorf("yandex versions.json parse: %w", err)
        }
        return &rv, nil
}

// YandexFindZapretZip ищет zapret в папке на Яндекс.Диске.
//
// Поддерживает 2 формата:
//  1. Старый: zip-файл zapret-discord-youtube-*.zip в подпапке /zapret/
//  2. Новый: папка "Zapret <version>" в корне (например "Zapret 1.9.9d").
//     Папка скачивается как zip через Yandex API.
//
// Возвращает прямую ссылку для скачивания, имя (файла или папки) и ошибку.
// Используется как fallback в zapret/updater.go когда GitHub недоступен.
func YandexFindZapretZip(publicKey string) (downloadURL, name string, err error) {
        if publicKey == "" {
                return "", "", fmt.Errorf("yandex public key is empty")
        }

        // 1. Try new format: folders "Zapret <version>" in root
        items, listErr := listYandexDir(publicKey, "")
        if listErr == nil {
                // Find newest "Zapret <version>" folder
                var bestFolder *yandexResource
                for i, it := range items {
                        if it.Type != "dir" {
                                continue
                        }
                        lowerName := strings.ToLower(it.Name)
                        if !strings.HasPrefix(lowerName, "zapret ") && lowerName != "zapret" {
                                continue
                        }
                        // Compare versions, keep the newest
                        if bestFolder == nil {
                                bestFolder = &items[i]
                        } else {
                                v1 := extractVersionFromZapretFolder(it.Name)
                                v2 := extractVersionFromZapretFolder(bestFolder.Name)
                                if v1 != "" && v2 != "" && CompareVersions(v1, v2) > 0 {
                                        bestFolder = &items[i]
                                }
                        }
                }

                if bestFolder != nil {
                        // Download folder as zip via Yandex API
                        zipURL, dlErr := yandexDownloadFolderAsZip(publicKey, bestFolder.Path)
                        if dlErr == nil && zipURL != "" {
                                return zipURL, bestFolder.Name, nil
                        }
                }
        }

        // 2. Fallback: old format - zip file in /zapret/ subfolder
        res, err := findYandexFileByPrefix(publicKey, "/zapret", "zapret-discord-youtube-")
        if err != nil {
                return "", "", err
        }
        if res == nil {
                return "", "", nil
        }
        return res.File, res.Name, nil
}

// extractVersionFromZapretFolder extracts version from folder name.
// "Zapret 1.9.9d" -> "1.9.9d", "Zapret" -> ""
func extractVersionFromZapretFolder(name string) string {
        lower := strings.ToLower(name)
        lower = strings.TrimPrefix(lower, "zapret")
        lower = strings.TrimSpace(lower)
        return lower
}

// yandexDownloadFolderAsZip returns a direct download URL for a Yandex Disk
// folder as a zip archive. Uses the public/resources/download?path=... endpoint.
func yandexDownloadFolderAsZip(publicKey, folderPath string) (string, error) {
        // Yandex Disk API: download a folder as zip
        // GET /v1/disk/public/resources/download?public_key=...&path=...&is_direct_zip_experiment=true
        q := url.Values{}
        q.Set("public_key", publicKey)
        q.Set("path", folderPath)
        // This triggers zip download for folders
        reqURL := yandexAPIBase + "/download?" + q.Encode()

        req, err := http.NewRequest("GET", reqURL, nil)
        if err != nil {
                return "", err
        }
        req.Header.Set("User-Agent", userAgent)

        resp, err := yandexClient.Do(req)
        if err != nil {
                return "", fmt.Errorf("yandex download folder: %w", err)
        }
        defer resp.Body.Close()

        if resp.StatusCode != http.StatusOK {
                body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
                return "", fmt.Errorf("yandex download folder: status %d, body: %s", resp.StatusCode, string(body))
        }

        var r struct {
                Href string `json:"href"`
        }
        if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
                return "", fmt.Errorf("parse yandex download response: %w", err)
        }
        return r.Href, nil
}

// YandexDownloadURL возвращает прямую ссылку на скачивание файла с Яндекс.Диска.
// Для новой структуры: ищет в папке с самой свежей версией.
// Для старой: ищет в корне или указанной подпапке.
// Если файл не найден — возвращает пустую строку.
func YandexDownloadURL(publicKey, path, filename string) (string, error) {
        if publicKey == "" {
                return "", nil
        }

        // If path is specified, use it directly
        if path != "" {
                res, err := findYandexFile(publicKey, path, filename)
                if err != nil {
                        return "", err
                }
                if res == nil {
                        return "", nil
                }
                return res.File, nil
        }

        // New format: find newest version folder in root, search file there
        items, err := listYandexDir(publicKey, "")
        if err == nil {
                var bestVersionFolder *yandexResource
                for i, it := range items {
                        if it.Type != "dir" {
                                continue
                        }
                        lowerName := strings.ToLower(it.Name)
                        if strings.HasPrefix(lowerName, "zapret ") || lowerName == "zapret" {
                                continue
                        }
                        if len(it.Name) == 0 || it.Name[0] < '0' || it.Name[0] > '9' {
                                continue
                        }
                        if bestVersionFolder == nil {
                                bestVersionFolder = &items[i]
                        } else {
                                if CompareVersions(it.Name, bestVersionFolder.Name) > 0 {
                                        bestVersionFolder = &items[i]
                                }
                        }
                }

                if bestVersionFolder != nil {
                        res, ferr := findYandexFile(publicKey, bestVersionFolder.Path, filename)
                        if ferr == nil && res != nil && res.File != "" {
                                return res.File, nil
                        }
                }
        }

        // Old format: file in root
        res, err := findYandexFile(publicKey, "", filename)
        if err != nil {
                return "", err
        }
        if res == nil {
                return "", nil
        }
        return res.File, nil
}

// yandexDownloadURLByPrefix — то же, но поиск по префиксу имени.
func yandexDownloadURLByPrefix(publicKey, path, prefix string) (string, string, error) {
        if publicKey == "" {
                return "", "", nil
        }
        res, err := findYandexFileByPrefix(publicKey, path, prefix)
        if err != nil {
                return "", "", err
        }
        if res == nil {
                return "", "", nil
        }
        return res.File, res.Name, nil
}

// downloadFromYandex скачивает файл с Яндекс.Диска в dest.
// downloadURL — прямая ссылка из yandexDownloadURL.
func downloadFromYandex(downloadURL, dest string) error {
        client := &http.Client{Timeout: 10 * time.Minute}
        req, err := http.NewRequest("GET", downloadURL, nil)
        if err != nil {
                return err
        }
        req.Header.Set("User-Agent", userAgent)
        resp, err := client.Do(req)
        if err != nil {
                return fmt.Errorf("yandex download: %w", err)
        }
        defer resp.Body.Close()
        if resp.StatusCode != http.StatusOK {
                return fmt.Errorf("yandex download: статус %d", resp.StatusCode)
        }

        out, err := os.Create(dest)
        if err != nil {
                return err
        }
        defer out.Close()
        _, err = io.Copy(out, io.LimitReader(resp.Body, 200<<20)) // 200 MB max
        return err
}
