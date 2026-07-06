package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type Release struct {
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
	Assets  []Asset `json:"assets"`
}

type Updater struct {
	owner string
	repo  string
	ver   string
}

func New(owner, repo, version string) *Updater {
	return &Updater{owner: owner, repo: repo, ver: version}
}

func (u *Updater) CheckLatest(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", u.owner, u.repo)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ZPUI/"+u.ver)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("check update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("github api: %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("parse release: %w", err)
	}
	return &rel, nil
}

func (u *Updater) NeedsUpdate(rel *Release) bool {
	v := strings.TrimPrefix(rel.TagName, "v")
	return v != "" && v != u.ver
}

func (u *Updater) FindAsset(rel *Release, suffix string) *Asset {
	for i := range rel.Assets {
		if strings.HasSuffix(rel.Assets[i].Name, suffix) {
			return &rel.Assets[i]
		}
	}
	return nil
}

func (u *Updater) Download(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "ZPUI/"+u.ver)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download: %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}

	f, err := os.Create(dest + ".tmp")
	if err != nil {
		return err
	}

	written, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(dest + ".tmp")
		return fmt.Errorf("download write: %w", err)
	}
	if written == 0 {
		os.Remove(dest + ".tmp")
		return fmt.Errorf("download: empty file")
	}

	return os.Rename(dest+".tmp", dest)
}

func (u *Updater) PrepareSelfUpdate(exePath, newExePath string) (batPath string, err error) {
	dir := filepath.Dir(exePath)
	batPath = filepath.Join(dir, "_update.bat")

	bat := fmt.Sprintf(`@echo off
chcp 65001 >nul
:wait
tasklist /fi "PID eq %d" 2>nul | find "%d" >nul
if not errorlevel 1 (
	timeout /t 1 /nobreak >nul
	goto wait
)
move /y "%s" "%s" >nul 2>&1
start "" "%s" --skip-checks
del "%%~f0"
`, os.Getpid(), os.Getpid(), newExePath, exePath, exePath)

	if err := os.WriteFile(batPath, []byte(bat), 0644); err != nil {
		return "", fmt.Errorf("create update bat: %w", err)
	}
	return batPath, nil
}

func RunBat(batPath string) error {
	return exec.Command("cmd", "/c", batPath).Start()
}
