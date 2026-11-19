package updater

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type ProgressFunc func(percent int, downloaded, total int64)

type DownloadResult struct {
	Source   string `json:"source"`
	URL      string `json:"url"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
	DestPath string `json:"dest_path"`
}

func downloadFromYandexWithProgress(url, dest string, progress ProgressFunc) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	req, err := http.NewRequest("GET", url, nil)
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
		return fmt.Errorf("yandex download: HTTP %d", resp.StatusCode)
	}

	total := resp.ContentLength
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	pw := &progressWriter{
		total:    total,
		progress: progress,
	}

	_, err = io.Copy(out, io.TeeReader(io.LimitReader(resp.Body, 200<<20), pw))
	if err != nil {
		return err
	}

	if progress != nil {
		progress(100, total, total)
	}
	return nil
}

// DownloadHTTPWithProgress — скачивание файла по прямому URL с прогрессом
// (основной канал обновлений — сервер управления; Яндекс.Диск — фолбэк).
func DownloadHTTPWithProgress(url, dest string, progress ProgressFunc) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: HTTP %d", resp.StatusCode)
	}

	total := resp.ContentLength
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	pw := &progressWriter{total: total, progress: progress}
	if _, err := io.Copy(out, io.TeeReader(resp.Body, pw)); err != nil {
		return err
	}

	if progress != nil {
		progress(100, total, total)
	}
	return nil
}

func FetchChecksumsYandex() (map[string]ChecksumEntry, error) {
	dlURL, err := yandexDownloadURL(YandexPublicURL, "", "checksums.sha256")
	if err != nil || dlURL == "" {
		return nil, fmt.Errorf("checksums.sha256 not found on Yandex Disk")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", dlURL, nil)
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yandex checksums: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, err
	}

	return ParseChecksums(data), nil
}
