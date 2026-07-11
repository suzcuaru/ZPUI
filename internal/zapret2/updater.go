package zapret2

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

	"zpui/internal/security"
)

const (
	githubAPIReleases = "https://api.github.com/repos/bol-van/zapret2/releases/latest"
	githubReleasePage = "https://github.com/bol-van/zapret2/releases/latest"
)

type UpdateInfo struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	UpdateNeeded   bool   `json:"update_needed"`
	DownloadURL    string `json:"download_url"`
	FallbackURLs   []string `json:"-"`
	ReleasePage    string `json:"release_page"`
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
				info.LatestVersion = strings.TrimPrefix(release.TagName, "v")
				for _, a := range release.Assets {
					if strings.HasSuffix(a.Name, ".zip") && !strings.Contains(a.Name, "openwrt") {
						info.DownloadURL = a.BrowserDownloadURL
						break
					}
				}
				if info.DownloadURL != "" {
					info.FallbackURLs = []string{
						"https://ghproxy.net/" + info.DownloadURL,
						"https://ghproxy.com/" + info.DownloadURL,
					}
				}
			}
		}
		resp.Body.Close()
	}

	if info.LatestVersion != "" && info.CurrentVersion != "" && info.CurrentVersion != "not installed" {
		info.UpdateNeeded = isVersionNewer(info.CurrentVersion, info.LatestVersion)
	}

	if info.LatestVersion == "" {
		return nil, fmt.Errorf("failed to get zapret2 version")
	}

	return info, nil
}

func isVersionNewer(current, latest string) bool {
	if current == "not installed" || current == "" {
		return true
	}
	if current == latest {
		return false
	}
	return latest > current
}

type ProgressFn func(downloaded, total int64)

func (m *Manager) DownloadAndInstall(progressFn ProgressFn) error {
	zapret2Dir := m.cfg.GetZapret2Path()
	if zapret2Dir == "" {
		return fmt.Errorf("zapret2 path not configured")
	}

	m.log.Info("updater", "Downloading zapret2...")

	info, err := m.CheckForUpdates()
	if err != nil {
		return fmt.Errorf("check updates: %w", err)
	}

	tempZip := filepath.Join(os.TempDir(), "zapret2-download.zip")
	urls := []string{info.DownloadURL}
	urls = append(urls, info.FallbackURLs...)

	var dlErr error
	downloaded := false
	for _, u := range urls {
		if u == "" {
			continue
		}
		m.log.Info("updater", "Downloading from: "+u)
		if err := downloadFile(u, tempZip); err != nil {
			dlErr = err
			m.log.Warn("updater", "Download failed: "+err.Error())
			continue
		}
		downloaded = true
		break
	}
	if !downloaded {
		if dlErr == nil {
			dlErr = fmt.Errorf("no download URLs available")
		}
		return dlErr
	}
	defer os.Remove(tempZip)

	m.log.Info("updater", "Scanning for malware...")
	scanResult, err := security.ScanZip(tempZip, []string{"binaries", "lua", "files"})
	if err != nil {
		return fmt.Errorf("security scan failed: %w", err)
	}
	if !scanResult.Clean {
		m.log.Error("updater", security.FormatScanResult(scanResult))
		return fmt.Errorf("SECURITY: update rejected — %d threat(s) detected. Update NOT applied. Check ZPUI log for details", len(scanResult.Threats))
	}
	m.log.Info("updater", security.FormatScanResult(scanResult))

	m.log.Info("updater", "Extracting zapret2...")
	if err := m.extractUpdate(tempZip, zapret2Dir); err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	m.log.Info("updater", "Flattening binaries...")
	if err := m.flattenBinaries(zapret2Dir); err != nil {
		m.log.Warn("updater", "Flatten binaries: "+err.Error())
	}

	m.log.Info("updater", "Writing preset strategies...")
	if err := m.writePresets(zapret2Dir); err != nil {
		m.log.Warn("updater", "Presets: "+err.Error())
	}

	m.log.Info("updater", "Creating default files...")
	m.ensureDefaultFiles(zapret2Dir)

	m.version = detectVersion(m.cfg)
	m.log.Info("updater", fmt.Sprintf("Zapret2 installed, version: %s", m.version))
	return nil
}

func (m *Manager) extractUpdate(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

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

		nameNorm := filepath.ToSlash(name)

		var destName string
		skip := false

		if strings.HasPrefix(nameNorm, "binaries/") {
			rest := strings.TrimPrefix(nameNorm, "binaries/")
			if strings.HasPrefix(rest, "windows-x86_64/") {
				destName = "binaries/" + strings.TrimPrefix(rest, "windows-x86_64/")
			} else {
				skip = true
			}
		} else if strings.HasPrefix(nameNorm, "lua/") {
			destName = nameNorm
		} else if strings.HasPrefix(nameNorm, "files/") {
			destName = nameNorm
		} else {
			skip = true
		}

		if skip {
			continue
		}

		if destName == "" {
			continue
		}

		dest := filepath.Join(destDir, filepath.FromSlash(destName))
		if f.FileInfo().IsDir() {
			os.MkdirAll(dest, 0755)
			continue
		}

		if err := writeZipEntry(f, dest); err != nil {
			m.log.Warn("updater", fmt.Sprintf("Failed to write: %s: %v", destName, err))
			continue
		}
	}

	return nil
}

func (m *Manager) flattenBinaries(dir string) error {
	binDir := filepath.Join(dir, "binaries")
	win64Dir := filepath.Join(binDir, "windows-x86_64")

	entries, err := os.ReadDir(win64Dir)
	if err != nil {
		return fmt.Errorf("read windows-x86_64: %w", err)
	}

	for _, e := range entries {
		src := filepath.Join(win64Dir, e.Name())
		dst := filepath.Join(binDir, e.Name())
		data, err := os.ReadFile(src)
		if err != nil {
			m.log.Warn("updater", "Cannot read "+e.Name()+": "+err.Error())
			continue
		}
		if err := os.WriteFile(dst, data, 0755); err != nil {
			m.log.Warn("updater", "Cannot write "+e.Name()+": "+err.Error())
			continue
		}
	}

	os.RemoveAll(win64Dir)
	os.RemoveAll(filepath.Join(binDir, "windows-x86"))

	otherDirs := []string{
		"linux-x86_64", "linux-x86", "linux-arm64", "linux-arm",
		"linux-mips", "linux-mips64", "linux-mipsel", "linux-ppc",
		"linux-riscv64", "linux-lexra",
		"android-arm", "android-arm64", "android-x86", "android-x86_64",
		"freebsd-x86_64",
	}
	for _, d := range otherDirs {
		os.RemoveAll(filepath.Join(binDir, d))
	}

	m.log.Info("updater", "Binaries flattened to binaries/")
	return nil
}


func (m *Manager) ensureDefaultFiles(dir string) {
	filesDir := filepath.Join(dir, "files")
	os.MkdirAll(filesDir, 0755)

	ytList := filepath.Join(filesDir, "list-youtube.txt")
	if _, err := os.Stat(ytList); os.IsNotExist(err) {
		os.WriteFile(ytList, []byte("googlevideo.com\nyoutube.com\nytimg.com\n"), 0644)
	}
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

func detectZipRoot(files []*zip.File) string {
	var prefix string
	for _, f := range files {
		name := f.Name
		idx := strings.IndexAny(name, "/\\")
		if idx < 0 {
			return ""
		}
		dir := name[:idx+1]
		if prefix == "" {
			prefix = dir
		} else if prefix != dir {
			return ""
		}
	}
	return prefix
}

func downloadFile(url, dest string) error {
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
			lastErr = fmt.Errorf("server returned %d", resp.StatusCode)
			time.Sleep(time.Duration(i+1) * 2 * time.Second)
			continue
		}

		out, err := os.Create(dest)
		if err != nil {
			resp.Body.Close()
			return err
		}

		_, err = io.Copy(out, resp.Body)
		resp.Body.Close()
		out.Close()
		if err != nil {
			lastErr = err
			os.Remove(dest)
			time.Sleep(time.Duration(i+1) * 2 * time.Second)
			continue
		}

		return nil
	}

	return fmt.Errorf("download failed after 3 attempts: %w", lastErr)
}

func (m *Manager) PerformUpdate(progress chan<- UpdateProgress) error {
	defer close(progress)

	progress <- UpdateProgress{Step: "Checking for updates", Percent: 5}

	info, err := m.CheckForUpdates()
	if err != nil {
		progress <- UpdateProgress{Step: "Error", Error: err.Error()}
		return err
	}

	if !info.UpdateNeeded && m.version != "not installed" {
		progress <- UpdateProgress{Step: "Already up to date", Percent: 100}
		return nil
	}

	m.log.Info("updater", fmt.Sprintf("Updating from %s to %s", info.CurrentVersion, info.LatestVersion))

	progress <- UpdateProgress{Step: "Stopping zapret2", Percent: 20}
	m.Stop()
	m.RemoveService()
	killWinws2()
	time.Sleep(2 * time.Second)

	progress <- UpdateProgress{Step: "Downloading", Percent: 30}
	if err := m.DownloadAndInstall(nil); err != nil {
		progress <- UpdateProgress{Step: "Error", Error: err.Error()}
		return err
	}

	progress <- UpdateProgress{Step: "Update complete", Percent: 100}
	return nil
}