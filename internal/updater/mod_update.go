package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ModUpdateInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Current       string `json:"current"`
	Latest        string `json:"latest"`
	NeedsUpdate   bool   `json:"needs_update"`
	DownloadURL   string `json:"download_url,omitempty"`
	Repository    string `json:"repository"`
	UpdateType    string `json:"update_type"` // "mod", "core"
}

type ModManifestRemote struct {
	Version string `json:"version"`
	Entry   string `json:"entry"`
}

func CheckModUpdate(id, name, currentVersion, repository string) (*ModUpdateInfo, error) {
	if repository == "" {
		return &ModUpdateInfo{
			ID: id, Name: name, Current: currentVersion,
			Latest: currentVersion, NeedsUpdate: false,
		}, nil
	}

	// Check if this version is ignored
	bm := NewBackupManager(filepath.Dir(repository))
	if bm.IsVersionIgnored(id, currentVersion) {
		return &ModUpdateInfo{
			ID: id, Name: name, Current: currentVersion,
			Latest: currentVersion, NeedsUpdate: false,
		}, nil
	}

	// Try to fetch mod version from GitHub
	latestVer, downloadURL := fetchModLatestVersion(repository)
	if latestVer == "" {
		return &ModUpdateInfo{
			ID: id, Name: name, Current: currentVersion,
			Latest: currentVersion, NeedsUpdate: false,
		}, nil
	}

	needsUpdate := latestVer != "" && IsNewer(currentVersion, latestVer)

	return &ModUpdateInfo{
		ID:          id,
		Name:        name,
		Current:     currentVersion,
		Latest:      latestVer,
		NeedsUpdate: needsUpdate,
		DownloadURL: downloadURL,
		Repository:  repository,
		UpdateType:  "mod",
	}, nil
}

func fetchModLatestVersion(repoURL string) (string, string) {
	// Extract owner/repo from GitHub URL
	// https://github.com/owner/repo -> owner/repo
	repoPath := strings.TrimPrefix(repoURL, "https://github.com/")
	repoPath = strings.TrimSuffix(repoPath, ".git")
	repoPath = strings.TrimSuffix(repoPath, "/")

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repoPath)
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ZPUI/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		// Try tags as fallback
		return fetchModLatestTag(repoPath)
	}

	var rel struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", ""
	}

	version := strings.TrimPrefix(rel.TagName, "v")
	var downloadURL string
	for _, a := range rel.Assets {
		if strings.HasSuffix(a.Name, ".zip") || strings.HasSuffix(a.Name, ".js") {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}

	return version, downloadURL
}

func fetchModLatestTag(repoPath string) (string, string) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/tags", repoPath)
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ZPUI/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", ""
	}

	var tags []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil || len(tags) == 0 {
		return "", ""
	}

	return strings.TrimPrefix(tags[0].Name, "v"), ""
}

func DownloadModUpdate(modDir, downloadURL string) error {
	if downloadURL == "" {
		return fmt.Errorf("no download URL")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	// Determine filename from URL or content
	filename := filepath.Base(downloadURL)
	if !strings.HasSuffix(filename, ".js") && !strings.HasSuffix(filename, ".zip") {
		filename = "index.js"
	}

	destPath := filepath.Join(modDir, filename)
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	return os.WriteFile(destPath, data, 0644)
}
