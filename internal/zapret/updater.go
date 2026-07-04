package zapret

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zpui/internal/executil"
)

type ProgressFn func(downloaded, total int64)

const (
	githubVersionURL  = "https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/main/.service/version.txt"
	githubReleasePage = "https://github.com/Flowseal/zapret-discord-youtube/releases/latest"
	githubAPIReleases = "https://api.github.com/repos/Flowseal/zapret-discord-youtube/releases/latest"
)

type UpdateInfo struct {
	CurrentVersion string   `json:"current_version"`
	LatestVersion  string   `json:"latest_version"`
	UpdateNeeded   bool     `json:"update_needed"`
	DownloadURL    string   `json:"download_url"`
	FallbackURLs   []string `json:"-"`
	ReleasePage    string   `json:"release_page"`
}

type UpdateProgress struct {
	Step    string `json:"step"`
	Percent int    `json:"percent"`
	Error   string `json:"error,omitempty"`
}

func (m *Manager) CheckForUpdates() (*UpdateInfo, error) {
	info := &UpdateInfo{
		CurrentVersion: m.version,
		ReleasePage:    githubReleasePage,
	}

	client := &http.Client{Timeout: 10 * time.Second}

	// Get latest version from raw file
	if resp, err := client.Get(githubVersionURL); err == nil {
		if resp.StatusCode == 200 {
			body, _ := io.ReadAll(resp.Body)
			info.LatestVersion = strings.TrimSpace(string(body))
			info.UpdateNeeded = info.CurrentVersion != info.LatestVersion
		}
		resp.Body.Close()
	}

	// Get actual download URL + fallbacks from GitHub API
	req, _ := http.NewRequest("GET", githubAPIReleases, nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ZPUI/1.0")
	if resp, err := client.Do(req); err == nil {
		if resp.StatusCode == 200 {
			body, _ := io.ReadAll(resp.Body)
			var release struct {
				TagName string `json:"tag_name"`
				Assets  []struct {
					Name               string `json:"name"`
					BrowserDownloadURL string `json:"browser_download_url"`
				} `json:"assets"`
			}
			if json.Unmarshal(body, &release) == nil {
				zipName := "zapret-discord-youtube-" + release.TagName + ".zip"
				directURL := ""
				for _, a := range release.Assets {
					if a.Name == zipName {
						directURL = a.BrowserDownloadURL
						info.DownloadURL = directURL
						break
					}
				}
				// Fallback: try ghproxy for blocked regions
				if directURL != "" {
					proxyURL := "https://ghproxy.net/" + directURL
					proxyURL2 := "https://ghproxy.com/" + directURL
					info.FallbackURLs = []string{proxyURL, proxyURL2}
				}
			}
		}
		resp.Body.Close()
	}

	if info.LatestVersion == "" {
		return nil, fmt.Errorf("не удалось получить версию Запрета")
	}

	return info, nil
}

func (m *Manager) PerformUpdate(progress chan<- UpdateProgress) error {
	defer close(progress)

	sendProgress(progress, "Checking for updates", 5)

	info, err := m.CheckForUpdates()
	if err != nil {
		progress <- UpdateProgress{Step: "Error", Error: err.Error()}
		return err
	}

	if !info.UpdateNeeded {
		sendProgress(progress, "Already up to date", 100)
		return nil
	}

	m.log.Info("updater", fmt.Sprintf("Updating from %s to %s", info.CurrentVersion, info.LatestVersion))

	sendProgress(progress, "Backing up state", 10)
	snap := m.CaptureState()

	sendProgress(progress, "Stopping zapret", 20)
	m.RemoveService()
	m.Stop()
	killWinws()
	executil.HiddenCmd("sc", "stop", "WinDivert").Run()
	executil.HiddenCmd("sc", "stop", "WinDivert14").Run()
	time.Sleep(3 * time.Second)

	sendProgress(progress, "Downloading update", 30)
	tempZip := filepath.Join(os.TempDir(), "zapret-update.zip")
	urls := []string{info.DownloadURL}
	urls = append(urls, info.FallbackURLs...)

	var dlErr error
	downloaded := false
	for _, u := range urls {
		if u == "" {
			continue
		}
		if err := downloadFile(u, tempZip); err != nil {
			dlErr = err
			m.log.Warn("updater", "Download failed ("+u[:min(len(u), 60)]+"): "+err.Error())
			continue
		}
		downloaded = true
		break
	}
	if !downloaded {
		if dlErr == nil {
			dlErr = fmt.Errorf("нет доступных URL для скачивания")
		}
		m.RestoreState(snap)
		progress <- UpdateProgress{Step: "Error", Error: fmt.Sprintf("Download failed: %v", dlErr)}
		return dlErr
	}
	m.log.Info("updater", "Download complete")

	sendProgress(progress, "Extracting update", 60)
	if err := m.extractUpdate(tempZip); err != nil {
		m.RestoreState(snap)
		progress <- UpdateProgress{Step: "Error", Error: fmt.Sprintf("Extract failed: %v", err)}
		return err
	}
	os.Remove(tempZip)

	sendProgress(progress, "Restoring state", 80)
	m.RestoreState(snap)

	m.version = detectZapretVersion(m.cfg)
	m.log.Info("updater", fmt.Sprintf("Updated to version: %s", m.version))

	sendProgress(progress, "Update complete", 100)
	return nil
}

func (m *Manager) extractUpdate(zipPath string) error {
	zapretDir := m.cfg.GetZapretPath()

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	// Определяем общий корневой префикс (если zip содержит одну папку)
	prefix := detectZipRoot(r.File)

	for _, f := range r.File {
		name := f.Name
		if prefix != "" && len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			name = name[len(prefix):]
		}
		name = strings.TrimLeft(name, "/\\")

		if name == "" {
			continue
		}

		fpath := filepath.Join(zapretDir, name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			m.log.Warn("updater", fmt.Sprintf("Cannot open %s for writing: %v — trying rename workaround", name, err))
			oldPath := fpath + ".old"
			os.Rename(fpath, oldPath)
			os.Remove(oldPath)
			outFile, err = os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				m.log.Warn("updater", fmt.Sprintf("Skipping locked file: %s", name))
				continue
			}
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

// detectZipRoot находит общий корневой каталог в zip-архиве.
// Например, для ["dir/a", "dir/b"] вернёт "dir/".
// Если файлы лежат в корне, вернёт "".
func detectZipRoot(files []*zip.File) string {
	var prefix string
	for _, f := range files {
		name := f.Name
		idx := strings.IndexAny(name, "/\\")
		if idx < 0 {
			return "" // файл в корне — без префикса
		}
		dir := name[:idx+1]
		if prefix == "" {
			prefix = dir
		} else if prefix != dir {
			return "" // разные корневые каталоги — без префикса
		}
	}
	return prefix
}

func downloadFile(url, filepath string) error {
	return downloadFileWithProgress(url, filepath, nil)
}

func downloadFileWithProgress(url, filepath string, progressFn ProgressFn) error {
	transport := &http.Transport{
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	client := &http.Client{
		Timeout:   10 * time.Minute,
		Transport: transport,
	}

	var lastErr error
	for i := 0; i < 3; i++ {
		resp, err := client.Get(url)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(i+1) * 2 * time.Second)
			continue
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			lastErr = fmt.Errorf("сервер вернул код %d", resp.StatusCode)
			time.Sleep(time.Duration(i+1) * 2 * time.Second)
			continue
		}

		out, err := os.Create(filepath)
		if err != nil {
			resp.Body.Close()
			return err
		}

		total := resp.ContentLength
		var written int64
		if progressFn != nil {
			_, err = io.Copy(out, io.TeeReader(resp.Body, &progressWriter{fn: progressFn, total: total, written: &written}))
		} else {
			_, err = io.Copy(out, resp.Body)
		}
		resp.Body.Close()
		out.Close()
		if err != nil {
			lastErr = err
			os.Remove(filepath)
			time.Sleep(time.Duration(i+1) * 2 * time.Second)
			continue
		}

		return nil
	}

	return fmt.Errorf("скачивание не удалось после 3 попыток: %w", lastErr)
}

type progressWriter struct {
	fn      ProgressFn
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

func verifyZip(path string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	r.Close()
	return nil
}

func (m *Manager) cloneFromGitHub(destDir string) error {
	if err := executil.HiddenCmd("git", "--version").Run(); err != nil {
		return fmt.Errorf("git не найден — установите Git для альтернативного способа скачивания")
	}

	repos := []string{
		"github.com/Flowseal/zapret-discord-youtube",
	}

	// Build full URLs with protocol
	fullURLs := make([]string, 0, len(repos)*3)
	for _, r := range repos {
		fullURLs = append(fullURLs,
			"https://"+r,
			"https://ghproxy.net/https://"+r,
			"https://ghproxy.com/https://"+r,
		)
	}

	for _, repo := range fullURLs {
		os.RemoveAll(destDir)

		m.log.Info("updater", "Trying git clone: "+repo[:min(len(repo), 80)])
		cmd := executil.HiddenCmd("git", "clone", "--depth", "1", repo, destDir)
		output, err := cmd.CombinedOutput()
		if err == nil {
			m.log.Info("updater", "Git clone successful")
			return nil
		}
		m.log.Warn("updater", fmt.Sprintf("Git clone failed: %s", string(output)))
	}

	return fmt.Errorf("не удалось склонировать репозиторий ни через один из зеркал")
}

func sendProgress(ch chan<- UpdateProgress, step string, pct int) {
	ch <- UpdateProgress{Step: step, Percent: pct}
}

func (m *Manager) DownloadAndInstall(progressFn ProgressFn) error {
	m.log.Info("updater", "Checking latest version...")
	info, err := m.CheckForUpdates()
	if err != nil {
		return fmt.Errorf("проверка обновлений: %w", err)
	}

	m.log.Info("updater", "Backing up state...")
	snap := m.CaptureState()

	m.log.Info("updater", "Downloading zapret...")
	tempZip := filepath.Join(os.TempDir(), "zapret-download.zip")

	urls := []string{info.DownloadURL}
	urls = append(urls, info.FallbackURLs...)

	var lastErr error
	for _, u := range urls {
		m.log.Info("updater", "Trying: "+u[:min(len(u), 80)])
		if err := downloadFileWithProgress(u, tempZip, progressFn); err != nil {
			lastErr = err
			m.log.Warn("updater", "Download failed: "+err.Error())
			continue
		}

		if err := verifyZip(tempZip); err != nil {
			m.log.Warn("updater", "Downloaded file is not a valid zip")
			os.Remove(tempZip)
			lastErr = fmt.Errorf("скачанный файл повреждён (невалидный zip)")
			continue
		}

		lastErr = nil
		break
	}

	if lastErr != nil {
		m.log.Info("updater", "Downloads failed, trying git clone as fallback...")
		if progressFn != nil {
			progressFn(-1, -1) // signal: cloning from git
		}
		zapretDir := m.cfg.GetZapretPath()
		if err := m.cloneFromGitHub(zapretDir); err != nil {
			return fmt.Errorf("все способы скачивания не удались: %w", lastErr)
		}
		killWinws()
		m.version = detectZapretVersion(m.cfg)
		m.RestoreState(snap)
		m.log.Info("updater", fmt.Sprintf("Zapret installed via git clone, version: %s", m.version))
		return nil
	}

	defer os.Remove(tempZip)

	m.log.Info("updater", "Stopping zapret before extraction...")
	m.RemoveService()
	killWinws()
	executil.HiddenCmd("sc", "stop", "WinDivert").Run()
	executil.HiddenCmd("sc", "stop", "WinDivert14").Run()
	time.Sleep(3 * time.Second)

	m.log.Info("updater", "Extracting zapret...")
	if err := m.extractUpdate(tempZip); err != nil {
		return fmt.Errorf("распаковка архива не удалась: %w", err)
	}

	killWinws()
	m.version = detectZapretVersion(m.cfg)
	m.RestoreState(snap)
	m.log.Info("updater", fmt.Sprintf("Zapret installed, version: %s", m.version))
	return nil
}

func (m *Manager) InstallZapret(sourceDir string, progress chan<- UpdateProgress) error {
	defer close(progress)

	sendProgress(progress, "Checking source", 5)
	winws := filepath.Join(sourceDir, "bin", "winws.exe")
	if _, err := os.Stat(winws); os.IsNotExist(err) {
		return fmt.Errorf("zapret not found at: %s", sourceDir)
	}

	sendProgress(progress, "Backing up state", 10)
	snap := m.CaptureState()

	svc := m.GetServiceStatus()
	wasRunning := svc.Running

	if wasRunning {
		sendProgress(progress, "Stopping current service", 15)
		m.RemoveService()
	}

	sendProgress(progress, "Removing old zapret service", 20)
	executil.HiddenCmd("net", "stop", "zapret").Run()
	executil.HiddenCmd("sc", "delete", "zapret").Run()
	killWinws()

	sendProgress(progress, "Copying zapret files", 30)
	zapretDir := m.cfg.GetZapretPath()

	copyDir := executil.HiddenCmd("xcopy", sourceDir, zapretDir, "/E", "/Y", "/I")
	if output, err := copyDir.CombinedOutput(); err != nil {
		m.RestoreState(snap)
		return fmt.Errorf("copy files: %v: %s", err, string(output))
	}

	m.version = detectZapretVersion(m.cfg)
	m.log.Info("updater", fmt.Sprintf("Zapret installed, version: %s", m.version))

	sendProgress(progress, "Restoring state", 80)
	m.RestoreState(snap)

	sendProgress(progress, "Installation complete", 100)
	return nil
}
