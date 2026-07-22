package updater

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// DownloadProgress callback for progress reporting.
type DownloadProgress func(downloaded, total int64)

type progressWriter struct {
	fn      DownloadProgress
	total   int64
	written *int64
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	*pw.written += int64(n)
	if pw.fn != nil {
		pw.fn(*pw.written, pw.total)
	}
	return n, nil
}

// DownloadOption configures a download.
type DownloadOption struct {
	URL      string
	Dest     string
	Progress DownloadProgress
	Timeout  time.Duration
	MaxSize  int64
}

// DownloadFile downloads a file with progress reporting and retries.
func DownloadFile(opt DownloadOption) error {
	if opt.Timeout == 0 {
		opt.Timeout = 10 * time.Minute
	}
	if opt.MaxSize == 0 {
		opt.MaxSize = 500 << 20 // 500 MB
	}

	transport := &http.Transport{
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	client := &http.Client{
		Timeout:   opt.Timeout,
		Transport: transport,
	}

	var lastErr error
	for i := 0; i < 3; i++ {
		req, err := http.NewRequest("GET", opt.URL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", userAgent)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(i+1) * 2 * time.Second)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("server returned status %d", resp.StatusCode)
			time.Sleep(time.Duration(i+1) * 2 * time.Second)
			continue
		}

		out, err := os.Create(opt.Dest)
		if err != nil {
			resp.Body.Close()
			return err
		}

		total := resp.ContentLength
		var written int64
		var readErr error

		if opt.Progress != nil {
			_, readErr = io.Copy(out, io.TeeReader(resp.Body, &progressWriter{
				fn:      opt.Progress,
				total:   total,
				written: &written,
			}))
		} else {
			_, readErr = io.Copy(out, io.LimitReader(resp.Body, opt.MaxSize))
		}

		resp.Body.Close()
		out.Close()

		if readErr != nil {
			lastErr = readErr
			os.Remove(opt.Dest)
			time.Sleep(time.Duration(i+1) * 2 * time.Second)
			continue
		}

		return nil
	}

	return fmt.Errorf("download failed after 3 attempts: %w", lastErr)
}

// DownloadFromBestSource tries multiple URLs and returns the first successful download.
// URLs are tried in order. Progress callback is called for each attempt.
func DownloadFromBestSource(urls []string, dest string, progress DownloadProgress) error {
	var lastErr error
	for _, u := range urls {
		if u == "" {
			continue
		}
		err := DownloadFile(DownloadOption{
			URL:      u,
			Dest:     dest,
			Progress: progress,
		})
		if err == nil {
			return nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no available URLs to download")
	}
	return lastErr
}

// ChooseBestSource selects the source with the newest version.
// Returns the download URLs sorted by preference (newest version first).
func ChooseBestSource(githubURL, yandexURL, githubVersion, yandexVersion string) []string {
	// If only one is available, use it
	if githubURL == "" && yandexURL == "" {
		return nil
	}
	if githubURL == "" {
		return []string{yandexURL}
	}
	if yandexURL == "" {
		return []string{githubURL}
	}

	// Both available — compare versions, newest first
	ghVer := normalizeVersion(githubVersion)
	yaVer := normalizeVersion(yandexVersion)

	if CompareVersions(ghVer, yaVer) >= 0 {
		return []string{githubURL, yandexURL}
	}
	return []string{yandexURL, githubURL}
}