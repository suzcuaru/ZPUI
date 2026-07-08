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
        YandexPublicURL = "https://disk.yandex.ru/d/1WD-A1MHklliaw"

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
// Если publicKey пустой — возвращает ошибку (fallback отключен).
func fetchYandexVersions(publicKey string) (*RemoteVersions, error) {
        if publicKey == "" {
                return nil, fmt.Errorf("yandex public key is empty")
        }
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

// YandexFindZapretZip ищет zapret zip-архив в подпапке /zapret/ на Яндекс.Диске.
// Возвращает прямую ссылку для скачивания, имя файла и ошибку.
// Используется как fallback в zapret/updater.go когда GitHub недоступен.
func YandexFindZapretZip(publicKey string) (downloadURL, name string, err error) {
        if publicKey == "" {
                return "", "", fmt.Errorf("yandex public key is empty")
        }
        res, err := findYandexFileByPrefix(publicKey, "/zapret", "zapret-discord-youtube-")
        if err != nil {
                return "", "", err
        }
        if res == nil {
                return "", "", nil
        }
        return res.File, res.Name, nil
}

// yandexDownloadURL возвращает прямую ссылку на скачивание файла с Яндекс.Диска.
// Если файл не найден — возвращает пустую строку.
func yandexDownloadURL(publicKey, path, filename string) (string, error) {
        if publicKey == "" {
                return "", nil
        }
        res, err := findYandexFile(publicKey, path, filename)
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
