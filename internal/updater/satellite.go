package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	githubAPIURL = "https://api.github.com/repos/suzcuaru/ZPUI/releases/latest"
	userAgent    = "ZPUI/updater"
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

// fetchReleaseInfo получает информацию о последнем релизе с проверкой статуса.
func fetchReleaseInfo() (*releaseInfo, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := newGitHubRequest(githubAPIURL)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api вернул статус %d", resp.StatusCode)
	}

	var rel releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("разбор ответа github: %w", err)
	}
	return &rel, nil
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
	rel, err := fetchReleaseInfo()
	if err != nil {
		return nil, err
	}

	vURL := findAsset(rel.Assets, "versions.json")
	if vURL == "" {
		return nil, fmt.Errorf("versions.json не найден в assets релиза %s", rel.TagName)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", vURL, nil)
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

func ReplaceSatellite(exeDir, name string) error {
	fileMap := map[string]string{
		"wizard":       "wizard.exe",
		"autoselect":   "autoselect.exe",
		"selfupdate":   "selfupdate.exe",
		"zapretupdate": "zapretupdate.exe",
	}

	fileName, ok := fileMap[name]
	if !ok {
		return fmt.Errorf("unknown satellite: %s", name)
	}

	targetPath := filepath.Join(exeDir, fileName)
	bakPath := targetPath + ".bak"

	remote, err := FetchRemoteVersions()
	if err != nil {
		return fmt.Errorf("failed to fetch remote versions: %w", err)
	}

	remoteMap := map[string]string{
		"wizard":       remote.Wizard,
		"autoselect":   remote.AutoSelect,
		"selfupdate":   remote.SelfUpdate,
		"zapretupdate": remote.ZapretUpdate,
	}

	latestVer := remoteMap[name]
	if latestVer == "" {
		return fmt.Errorf("no remote version for %s", name)
	}

	rel, err := fetchReleaseInfo()
	if err != nil {
		return fmt.Errorf("failed to fetch release info: %w", err)
	}
	downloadURL := findAsset(rel.Assets, fileName)
	if downloadURL == "" {
		return fmt.Errorf("no download URL for %s", fileName)
	}

	tmpPath := targetPath + ".tmp"
	if err := downloadFile(downloadURL, tmpPath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	bm := NewBackupManager(exeDir)
	if _, err := os.Stat(targetPath); err == nil {
		bm.BackupComponent("satellite_"+name, "pre-update", "satellite", []string{targetPath})
		os.Rename(targetPath, bakPath)
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		os.Rename(bakPath, targetPath)
		return fmt.Errorf("replace failed: %w", err)
	}

	os.Remove(bakPath)
	return nil
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
