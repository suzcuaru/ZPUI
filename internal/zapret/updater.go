package zapret

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"zpui/internal/executil"
	"zpui/internal/security"
	"zpui/internal/updater"
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

	// Get latest version from raw file (GitHub)
	if resp, err := client.Get(githubVersionURL); err == nil {
		if resp.StatusCode == 200 {
			body, _ := io.ReadAll(resp.Body)
			info.LatestVersion = strings.TrimSpace(string(body))
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
				// If GitHub API returned tag_name but version.txt failed, use tag as version
				if info.LatestVersion == "" && release.TagName != "" {
					info.LatestVersion = strings.TrimPrefix(release.TagName, "v")
				}
			}
		}
		resp.Body.Close()
	}

	// Yandex Disk fallback: if GitHub failed completely (no version OR no download URL),
	// try to find zapret folder on Yandex Disk.
	if info.LatestVersion == "" || info.DownloadURL == "" {
		yaURL, yaName, yErr := updater.YandexFindZapretZip(updater.YandexPublicURL)
		if yErr == nil && yaURL != "" {
			m.log.Info("updater", "Found zapret on Yandex Disk: "+yaName)
			if info.DownloadURL == "" {
				info.DownloadURL = yaURL
			}
			// Extract version from folder/file name
			if v := extractVersionFromZapretName(yaName); v != "" && info.LatestVersion == "" {
				info.LatestVersion = v
			}
		}
	}

	// Decision: only semver comparison.
	// "unknown"/empty current version is not considered an update, to avoid
	// false notifications right after incomplete/first install.
	if info.LatestVersion != "" && info.CurrentVersion != "" && info.CurrentVersion != "unknown" {
		info.UpdateNeeded = updater.IsNewer(info.CurrentVersion, info.LatestVersion)
	}

	if info.LatestVersion == "" {
		return nil, fmt.Errorf("\xd0\xbd\xd0\xb5 \xd1\x83\xd0\xb4\xd0\xb0\xd0\xbb\xd0\xbe\xd1\x81\xd1\x8c \xd0\xbf\xd0\xbe\xd0\xbb\xd1\x83\xd1\x87\xd0\xb8\xd1\x82\xd1\x8c \xd0\xb2\xd0\xb5\xd1\x80\xd1\x81\xd0\xb8\xd1\x8e \xd0\x97\xd0\xb0\xd0\xbf\xd1\x80\xd0\xb5\xd1\x82\xd0\xb0")
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
	// Yandex Disk fallback (added at the end so GitHub is tried first)
	if yaURL, _, yErr := updater.YandexFindZapretZip(updater.YandexPublicURL); yErr == nil && yaURL != "" {
		urls = append(urls, yaURL)
	}

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

	sendProgress(progress, "Scanning for malware", 50)
	scanResult, scanErr := security.ScanZip(tempZip, []string{"bin", "lists"})
	if scanErr != nil {
		m.RestoreState(snap)
		progress <- UpdateProgress{Step: "Error", Error: fmt.Sprintf("Security scan failed: %v", scanErr)}
		return scanErr
	}
	if !scanResult.Clean {
		m.RestoreState(snap)
		err := fmt.Errorf("SECURITY: update rejected — %d threat(s) detected", len(scanResult.Threats))
		progress <- UpdateProgress{Step: "Error", Error: security.FormatScanResult(scanResult)}
		m.log.Error("updater", security.FormatScanResult(scanResult))
		return err
	}
	m.log.Info("updater", security.FormatScanResult(scanResult))

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

	prefix := detectZipRoot(r.File)

	type entry struct {
		f       *zip.File
		relPath string
	}

	var entries []entry
	for _, f := range r.File {
		name := f.Name
		if prefix != "" && len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			name = name[len(prefix):]
		}
		name = strings.TrimLeft(name, "/\\")
		if name == "" {
			continue
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(filepath.Join(zapretDir, name), 0755)
			continue
		}
		entries = append(entries, entry{f, name})
	}

	failed := make(map[string]*entry)
	for i := range entries {
		e := &entries[i]
		dest := filepath.Join(zapretDir, e.relPath)
		if err := writeZipEntry(e.f, dest); err != nil {
			m.log.Warn("updater", fmt.Sprintf("Файл занят, будет повтор: %s", e.relPath))
			failed[e.relPath] = e
		}
	}

	if len(failed) > 0 {
		m.log.Info("updater", fmt.Sprintf("%d файлов заняты, останавливаем процессы и повторяем по одному...", len(failed)))
		killWinws()
		executil.HiddenCmd("sc", "stop", "WinDivert").Run()
		executil.HiddenCmd("sc", "stop", "WinDivert14").Run()
		time.Sleep(2 * time.Second)

		for relPath, e := range failed {
			dest := filepath.Join(zapretDir, relPath)
			if err := m.forceWriteEntry(e.f, dest, relPath); err != nil {
				m.log.Error("updater", fmt.Sprintf("Не удалось записать после повтора: %s: %v", relPath, err))
				continue
			}
			delete(failed, relPath)
		}
	}

	if len(failed) > 0 {
		names := make([]string, 0, len(failed))
		for n := range failed {
			names = append(names, n)
		}
		sort.Strings(names)
		return fmt.Errorf("не удалось распаковать (заняты процессом): %s", strings.Join(names, ", "))
	}

	m.log.Info("updater", fmt.Sprintf("Распаковано файлов: %d", len(entries)))
	return nil
}

func writeZipEntry(f *zip.File, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = io.Copy(out, rc)
	return err
}

// forceWriteEntry пробует rename-трюк: занятый файл переименовывается
// в .zpui-old, новый пишется на его место. На Windows работающий .exe
// можно переименовать (но не удалить), поэтому трюк помогает для winws.exe.
func (m *Manager) forceWriteEntry(f *zip.File, dest, relPath string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}

	if err := writeZipEntry(f, dest); err == nil {
		return nil
	}

	oldPath := dest + ".zpui-old"
	os.Remove(oldPath)
	if err := os.Rename(dest, oldPath); err != nil {
		return fmt.Errorf("невозможно ни записать, ни переименовать %s: %w", relPath, err)
	}

	m.log.Info("updater", fmt.Sprintf("Переименован занятый файл: %s", relPath))
	if err := writeZipEntry(f, dest); err != nil {
		return fmt.Errorf("запись после rename не удалась %s: %w", relPath, err)
	}
	os.Remove(oldPath)
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

func verifyZipContents(path string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()

	prefix := detectZipRoot(r.File)
	have := make(map[string]bool)
	for _, f := range r.File {
		name := f.Name
		if prefix != "" && len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			name = name[len(prefix):]
		}
		name = strings.TrimLeft(name, "/\\")
		have[name] = true
	}
	var missing []string
	for _, rel := range essentialFiles {
		if !have[rel] {
			missing = append(missing, rel)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("архив неполный, отсутствует: %s", strings.Join(missing, ", "))
	}
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
	m.log.Info("updater", "Backing up state...")
	snap := m.CaptureState()

	zapretDir := m.cfg.GetZapretPath()
	tempZip := filepath.Join(os.TempDir(), "zapret-download.zip")

	zipOK := false
	info, checkErr := m.CheckForUpdates()
	if checkErr != nil {
		m.log.Warn("updater", "GitHub API недоступен, пропуск zip: "+checkErr.Error())
	} else {
		urls := []string{info.DownloadURL}
		urls = append(urls, info.FallbackURLs...)
		// Yandex Disk fallback
		if yaURL, _, yErr := updater.YandexFindZapretZip(updater.YandexPublicURL); yErr == nil && yaURL != "" {
			urls = append(urls, yaURL)
		}

		for _, u := range urls {
			if u == "" {
				continue
			}
			m.log.Info("updater", "Скачивание: "+u[:min(len(u), 80)])
			if err := downloadFileWithProgress(u, tempZip, progressFn); err != nil {
				m.log.Warn("updater", "Download failed: "+err.Error())
				continue
			}
			if err := verifyZip(tempZip); err != nil {
				m.log.Warn("updater", "Downloaded file is not a valid zip")
				os.Remove(tempZip)
				continue
			}
			if err := verifyZipContents(tempZip); err != nil {
				m.log.Warn("updater", "Incomplete zip: "+err.Error())
				os.Remove(tempZip)
				continue
			}
			m.log.Info("updater", "Scanning for malware...")
			scanResult, scanErr := security.ScanZip(tempZip, []string{"bin", "lists"})
			if scanErr != nil {
				m.log.Warn("updater", "Security scan error: "+scanErr.Error())
				os.Remove(tempZip)
				continue
			}
			if !scanResult.Clean {
				m.log.Error("updater", security.FormatScanResult(scanResult))
				os.Remove(tempZip)
				return fmt.Errorf("SECURITY: update rejected — %d threat(s) detected. Update NOT applied", len(scanResult.Threats))
			}
			m.log.Info("updater", security.FormatScanResult(scanResult))
			zipOK = true
			break
		}
	}

	if zipOK {
		defer os.Remove(tempZip)

		m.log.Info("updater", "Stopping zapret before extraction...")
		m.RemoveService()
		killWinws()
		executil.HiddenCmd("sc", "stop", "WinDivert").Run()
		executil.HiddenCmd("sc", "stop", "WinDivert14").Run()
		time.Sleep(3 * time.Second)

		m.log.Info("updater", "Extracting zapret...")
		if err := m.extractUpdate(tempZip); err != nil {
			m.log.Warn("updater", "Extract failed, fallback to git: "+err.Error())
		} else if err := m.verifyEssential(); err != nil {
			m.log.Warn("updater", "Verify after extract failed: "+err.Error())
		} else {
			killWinws()
			m.version = detectZapretVersion(m.cfg)
			m.RestoreState(snap)
			m.log.Info("updater", fmt.Sprintf("Zapret installed via zip, version: %s", m.version))
			return nil
		}
	}

	m.log.Info("updater", "Git clone fallback...")
	if progressFn != nil {
		progressFn(-1, -1)
	}
	killWinws()
	m.RemoveService()
	executil.HiddenCmd("sc", "stop", "WinDivert").Run()
	executil.HiddenCmd("sc", "stop", "WinDivert14").Run()
	time.Sleep(2 * time.Second)

	if err := m.cloneFromGitHub(zapretDir); err != nil {
		m.log.Error("updater", "Git clone failed: "+err.Error())
		return fmt.Errorf("не удалось скачать zapret ни через zip, ни через git clone: %w", err)
	}
	if err := m.verifyEssential(); err != nil {
		return fmt.Errorf("неполная установка (git clone): %w", err)
	}
	killWinws()
	m.version = detectZapretVersion(m.cfg)
	m.RestoreState(snap)
	m.log.Info("updater", fmt.Sprintf("Zapret installed via git clone, version: %s", m.version))
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

	if err := m.verifyEssential(); err != nil {
		m.RestoreState(snap)
		return fmt.Errorf("источник неполон: %w", err)
	}

	m.version = detectZapretVersion(m.cfg)
	m.log.Info("updater", fmt.Sprintf("Zapret installed, version: %s", m.version))

	sendProgress(progress, "Restoring state", 80)
	m.RestoreState(snap)

	sendProgress(progress, "Installation complete", 100)
	return nil
}
// extractVersionFromZipName extracts version from filename like
// "zapret-discord-youtube-1.2.3.zip" -> "1.2.3".
func extractVersionFromZipName(name string) string {
	name = strings.TrimSuffix(name, ".zip")
	idx := strings.LastIndex(name, "-")
	if idx < 0 {
		return ""
	}
	return name[idx+1:]
}
// extractVersionFromZapretName extracts version from Yandex Disk folder/file name.
// Handles: "Zapret 1.9.9d" -> "1.9.9d", "zapret-discord-youtube-1.9.9d.zip" -> "1.9.9d"
func extractVersionFromZapretName(name string) string {
	name = strings.TrimSuffix(name, ".zip")
	name = strings.TrimPrefix(strings.ToLower(name), "zapret")
	name = strings.TrimPrefix(name, "-discord-youtube")
	name = strings.TrimSpace(name)
	// Remove leading dash/space
	name = strings.TrimLeft(name, "- ")
	return name
}
