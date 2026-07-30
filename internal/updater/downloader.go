package updater

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

type ProgressFunc func(percent int, downloaded, total int64)

type DownloadResult struct {
	Source      string `json:"source"`
	URL         string `json:"url"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	DestPath    string `json:"dest_path"`
}

func ZPUIZipURLYandex() (string, error) {
	archSuffix := "win64"
	if runtime.GOARCH == "386" {
		archSuffix = "win32"
	}
	return yandexDownloadURL(YandexPublicURL, "", "zpui-"+archSuffix+".zip")
}

// ZPUIUpdateInfo описывает выбранную для скачивания версию ZPUI и источник.
type ZPUIUpdateInfo struct {
	Version string
	Source  string
	Yandex  string
}

// FetchZPUIUpdateInfo получает информацию об обновлении ZPUI с Яндекс.Диска.
func FetchZPUIUpdateInfo() *ZPUIUpdateInfo {
	yaV := ""
	if rv, err := FetchRemoteVersionsYandex(YandexPublicURL); err == nil && rv != nil {
		yaV = rv.ZPUI
	}
	return &ZPUIUpdateInfo{Version: yaV, Source: "yandex", Yandex: yaV}
}

func DownloadZPUIUpdate(dest string, progress ProgressFunc) (*DownloadResult, error) {
	yaURL, _ := ZPUIZipURLYandex()
	if yaURL == "" {
		return nil, fmt.Errorf("Yandex Disk: нет доступного URL для скачивания ZPUI")
	}
	if err := downloadFromYandexWithProgress(yaURL, dest, progress); err != nil {
		return nil, fmt.Errorf("yandex: %v", err)
	}
	sha, _ := ComputeSHA256(dest)
	info, _ := os.Stat(dest)
	size := int64(0)
	if info != nil {
		size = info.Size()
	}
	return &DownloadResult{
		Source:   "yandex",
		URL:      yaURL,
		Size:     size,
		SHA256:   sha,
		DestPath: dest,
	}, nil
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

func FetchChecksums() (map[string]ChecksumEntry, error) {
	return FetchChecksumsYandex()
}

func CompareDownloadWithChecksum(result *DownloadResult, checksums map[string]ChecksumEntry, expectedFile string) error {
	if len(checksums) == 0 {
		return nil
	}
	entry, ok := checksums[strings.ToLower(expectedFile)]
	if !ok {
		return nil
	}
	ok, err := VerifyFileChecksum(result.DestPath, entry.SHA256)
	if err != nil {
		return fmt.Errorf("checksum verification error: %w", err)
	}
	if !ok {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", expectedFile, entry.SHA256, result.SHA256)
	}
	return nil
}
