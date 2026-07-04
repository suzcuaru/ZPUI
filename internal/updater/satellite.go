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
	versionsURL = "https://api.github.com/repos/suzcuaru/ZPUI/releases/latest"
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
	TagName string          `json:"tag_name"`
	Assets  []releaseAsset  `json:"assets"`
}

type ComponentUpdateStatus struct {
	Name         string `json:"name"`
	Current      string `json:"current"`
	Latest       string `json:"latest"`
	NeedsUpdate  bool   `json:"needs_update"`
	DownloadURL  string `json:"download_url"`
	File         string `json:"file"`
}

func FetchRemoteVersions() (*RemoteVersions, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", versionsURL, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rel releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}

	versionsURL := ""
	for _, a := range rel.Assets {
		if a.Name == "versions.json" {
			versionsURL = a.BrowserDownloadURL
			break
		}
	}
	if versionsURL == "" {
		return nil, fmt.Errorf("versions.json not found in release assets")
	}

	resp2, err := http.Get(versionsURL)
	if err != nil {
		return nil, err
	}
	defer resp2.Body.Close()

	var rv RemoteVersions
	if err := json.NewDecoder(resp2.Body).Decode(&rv); err != nil {
		return nil, err
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
		current := localVersions[key]
		latest := remoteMap[key]
		if current == "" {
			current = "0.0.0"
		}
		if latest == "" {
			latest = current
		}
		needs := latest != current && latest != "0.0.0" && latest != ""

		result = append(result, ComponentUpdateStatus{
			Name:        key,
			Current:     current,
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

	downloadURL := findAssetURL(name, fileName)
	if downloadURL == "" {
		return fmt.Errorf("no download URL for %s", fileName)
	}

	tmpPath := targetPath + ".tmp"
	if err := downloadFile(downloadURL, tmpPath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Backup current version before replacing
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

func findAssetURL(name, fileName string) string {
	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", versionsURL, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var rel releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return ""
	}

	for _, a := range rel.Assets {
		if a.Name == fileName {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}
