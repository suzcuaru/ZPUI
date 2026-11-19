package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)


// yandexResource — элемент листинга публичной папки Яндекс.Диска.
// Только нужные поля, остальные игнорируются.
type yandexResource struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // "file" или "dir"
	Path     string `json:"path"` // путь внутри публичной папки
	Size     int64  `json:"size"`
	File     string `json:"file"` // прямой URL для скачивания (только для файлов)
	Md5      string `json:"md5"`
	Modified string `json:"modified"`
}

type yandexResourcesResponse struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Path     string `json:"path"`
	Embedded *struct {
		Items []yandexResource `json:"items"`
		Total int              `json:"total"`
	} `json:"_embedded"`
}

// versionFolders возвращает папки версий ZPUI из листинга корня Яндекс.Диска,
// отсортированные от самой свежей к самой старой. Папки «Zapret <version>» и
// некорректные имена игнорируются.
func versionFolders(items []yandexResource) []yandexResource {
	var folders []yandexResource
	for _, it := range items {
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
		folders = append(folders, it)
	}
	sort.Slice(folders, func(i, j int) bool {
		return CompareVersions(folders[i].Name, folders[j].Name) > 0
	})
	return folders
}

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

// yandexDownloadURL возвращает прямую ссылку на скачивание файла с Яндекс.Диска.
// Для новой структуры: ищет в папке с самой свежей версией.
// Для старой: ищет в корне или указанной подпапке.
// Если файл не найден — возвращает пустую строку.
func yandexDownloadURL(publicKey, path, filename string) (string, error) {
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

	// New format: search file in version folders, newest first
	if items, err := listYandexDir(publicKey, ""); err == nil {
		for _, folder := range versionFolders(items) {
			res, ferr := findYandexFile(publicKey, folder.Path, filename)
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
