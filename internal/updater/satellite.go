package updater

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	githubReleasePage = "https://github.com/suzcuaru/ZPUI/releases/latest"
	downloadBase      = githubReleasePage + "/download/"
	versionsURL       = downloadBase + "versions.json"
	githubAPIURL      = "https://api.github.com/repos/suzcuaru/ZPUI/releases/latest"
	userAgent         = "ZPUI/updater"
)

type RemoteVersions struct {
	ZPUI         string `json:"zpui"`
	Wizard       string `json:"wizard"`
	AutoSelect   string `json:"autoselect"`
	SelfUpdate   string `json:"selfupdate"`
	ZapretUpdate string `json:"zapretupdate"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type releaseInfo struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type ComponentUpdateStatus struct {
	Name        string `json:"name"`
	Current     string `json:"current"`
	Latest      string `json:"latest"`
	NeedsUpdate bool   `json:"needs_update"`
	DownloadURL string `json:"download_url"`
	File        string `json:"file"`
}

func newGitHubRequest(url string) (*http.Request, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
	return req, nil
}

// fetchReleaseInfo получает информацию о последнем релизе с кешированием
// (in-memory TTL 5 минут + ETag/If-None-Match для экономии rate-limit GitHub).
func fetchReleaseInfo() (*releaseInfo, error) {
	cacheMu.RLock()
	if cachedRel != nil && time.Since(cachedAt) < releaseCacheTTL {
		r := *cachedRel
		cacheMu.RUnlock()
		return &r, nil
	}
	etag := cachedEtag
	body := cachedBody
	cacheMu.RUnlock()

	if etag == "" || body == nil {
		etag, body = loadPersistentCache()
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := newGitHubRequest(githubAPIURL)
	if err != nil {
		return nil, err
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotModified:
		if body != nil {
			var rel releaseInfo
			if err := json.Unmarshal(body, &rel); err == nil {
				storeCache(&rel, body, etag)
				return &rel, nil
			}
		}
		return nil, fmt.Errorf("github 304 без кеша")
	case http.StatusOK:
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return nil, fmt.Errorf("чтение ответа github: %w", err)
		}
		var rel releaseInfo
		if err := json.Unmarshal(raw, &rel); err != nil {
			return nil, fmt.Errorf("разбор ответа github: %w", err)
		}
		storeCache(&rel, raw, resp.Header.Get("ETag"))
		return &rel, nil
	default:
		return nil, fmt.Errorf("github api вернул статус %d", resp.StatusCode)
	}
}

func findAsset(assets []releaseAsset, name string) string {
	for _, a := range assets {
		if a.Name == name {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

func FetchRemoteVersions() (*RemoteVersions, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", versionsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("versions.json: статус %d", resp.StatusCode)
	}

	var rv RemoteVersions
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rv); err != nil {
		return nil, fmt.Errorf("разбор versions.json: %w", err)
	}
	return &rv, nil
}

func CheckAllComponents(localVersions map[string]string, exeDir string) ([]ComponentUpdateStatus, error) {
	remote, err := FetchRemoteVersions()
	if err != nil {
		return nil, err
	}

	remoteMap := map[string]string{
		"zpui":         remote.ZPUI,
		"wizard":       remote.Wizard,
		"autoselect":   remote.AutoSelect,
		"selfupdate":   remote.SelfUpdate,
		"zapretupdate": remote.ZapretUpdate,
	}

	fileMap := map[string]string{
		"zpui":         "zpui.exe",
		"wizard":       "wizard.exe",
		"autoselect":   "autoselect.exe",
		"selfupdate":   "selfupdate.exe",
		"zapretupdate": "zapretupdate.exe",
	}

	order := []string{"zpui", "wizard", "autoselect", "selfupdate", "zapretupdate"}
	var result []ComponentUpdateStatus

	for _, key := range order {
		current := normalizeVersion(localVersions[key])
		latest := normalizeVersion(remoteMap[key])
		needs := latest != "" && IsNewer(current, latest)

		result = append(result, ComponentUpdateStatus{
			Name:        key,
			Current:     localVersions[key],
			Latest:      latest,
			NeedsUpdate: needs,
			File:        fileMap[key],
		})
	}

	return result, nil
}

func ReplaceModule(exeDir, name string) error {
	fileMap := map[string]string{
		"wizard":       "wizard.exe",
		"autoselect":   "autoselect.exe",
		"selfupdate":   "selfupdate.exe",
		"zapretupdate": "zapretupdate.exe",
	}

	fileName, ok := fileMap[name]
	if !ok {
		return fmt.Errorf("unknown module: %s", name)
	}

	targetPath := filepath.Join(exeDir, fileName)
	bakPath := targetPath + ".bak"

	archSuffix := "win64"
	if runtime.GOARCH == "386" {
		archSuffix = "win32"
	}
	zipURL := fmt.Sprintf("%szpui-%s.zip", downloadBase, archSuffix)

	tmpZip := filepath.Join(exeDir, "zpui-modules-tmp.zip")
	if err := downloadFile(zipURL, tmpZip); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(tmpZip)

	if err := extractFromZip(tmpZip, fileName, targetPath); err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}

	bm := NewBackupManager(exeDir)
	if _, err := os.Stat(targetPath); err == nil {
		bm.BackupComponent("module_"+name, "pre-update", "module", []string{targetPath})
		os.Rename(targetPath, bakPath)
	}

	if err := os.Rename(targetPath+".tmp", targetPath); err != nil {
		os.Rename(bakPath, targetPath)
		return fmt.Errorf("replace failed: %w", err)
	}

	os.Remove(bakPath)
	return nil
}

func extractFromZip(zipPath, fileName, destPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if strings.EqualFold(filepath.Base(f.Name), fileName) {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			out, err := os.Create(destPath + ".tmp")
			if err != nil {
				return err
			}
			defer out.Close()

			_, err = io.Copy(out, rc)
			return err
		}
	}

	return fmt.Errorf("file %s not found in archive", fileName)
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("статус %d при загрузке %s", resp.StatusCode, filepath.Base(dest))
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, io.LimitReader(resp.Body, 100<<20))
	return err
}
