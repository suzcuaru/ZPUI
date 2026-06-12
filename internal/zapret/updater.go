package zapret

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"zpui/internal/executil"
	"path/filepath"
	"strings"
	"time"
)

const (
	githubVersionURL  = "https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/main/.service/version.txt"
	githubReleaseURL  = "https://github.com/Flowseal/zapret-discord-youtube/releases/latest/download/zapret-discord-youtube.zip"
	githubReleasePage = "https://github.com/Flowseal/zapret-discord-youtube/releases/latest"
)

type UpdateInfo struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	UpdateNeeded   bool   `json:"update_needed"`
	DownloadURL    string `json:"download_url"`
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
		DownloadURL:    githubReleaseURL,
		ReleasePage:    githubReleasePage,
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(githubVersionURL)
	if err != nil {
		return nil, fmt.Errorf("fetch version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("version check returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read version: %w", err)
	}

	info.LatestVersion = strings.TrimSpace(string(body))
	info.UpdateNeeded = info.CurrentVersion != info.LatestVersion

	return info, nil
}

func (m *Manager) PerformUpdate(progress chan<- UpdateProgress) error {
	defer close(progress)

	currentStrategy := m.cfg.GetCurrentStrategy()

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

	sendProgress(progress, "Backing up user lists", 10)
	backupDir, err := m.backupUserLists()
	if err != nil {
		progress <- UpdateProgress{Step: "Error", Error: fmt.Sprintf("Backup failed: %v", err)}
		return err
	}
	m.log.Info("updater", fmt.Sprintf("User lists backed up to: %s", backupDir))

	sendProgress(progress, "Stopping zapret", 20)
	m.RemoveService()
	m.Stop()
	time.Sleep(3 * time.Second)

	sendProgress(progress, "Downloading update", 30)
	tempZip := filepath.Join(os.TempDir(), "zapret-update.zip")
	if err := downloadFile(info.DownloadURL, tempZip); err != nil {
		m.restoreFromBackup(backupDir)
		progress <- UpdateProgress{Step: "Error", Error: fmt.Sprintf("Download failed: %v", err)}
		return err
	}
	m.log.Info("updater", "Download complete")

	sendProgress(progress, "Extracting update", 60)
	if err := m.extractUpdate(tempZip); err != nil {
		m.restoreFromBackup(backupDir)
		progress <- UpdateProgress{Step: "Error", Error: fmt.Sprintf("Extract failed: %v", err)}
		return err
	}
	os.Remove(tempZip)

	sendProgress(progress, "Restoring user lists", 80)
	if err := m.restoreFromBackup(backupDir); err != nil {
		m.log.Error("updater", fmt.Sprintf("Restore warning: %v", err))
	}
	os.RemoveAll(backupDir)

	m.version = detectZapretVersion(m.cfg)
	m.log.Info("updater", fmt.Sprintf("Updated to version: %s", m.version))

	sendProgress(progress, "Restarting zapret", 90)
	if currentStrategy != "" {
		if err := m.InstallService(currentStrategy); err != nil {
			m.log.Warn("updater", fmt.Sprintf("Service restart failed: %v", err))
			m.StartWithStrategy(currentStrategy)
		}
	}

	sendProgress(progress, "Update complete", 100)
	return nil
}

func (m *Manager) backupUserLists() (string, error) {
	backupDir := filepath.Join(os.TempDir(), fmt.Sprintf("zapret-backup-%d", time.Now().Unix()))
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", err
	}

	listsDir := m.cfg.ListsDir()
	userFiles := []string{
		"list-general-user.txt",
		"list-exclude-user.txt",
		"ipset-exclude-user.txt",
	}

	for _, f := range userFiles {
		src := filepath.Join(listsDir, f)
		dst := filepath.Join(backupDir, f)
		if _, err := os.Stat(src); err == nil {
			if err := copyFile(src, dst); err != nil {
				return backupDir, fmt.Errorf("backup %s: %w", f, err)
			}
			m.log.Info("updater", fmt.Sprintf("Backed up: %s", f))
		}
	}

	strategyFile := filepath.Join(backupDir, ".strategy")
	os.WriteFile(strategyFile, []byte(m.cfg.GetCurrentStrategy()), 0644)

	return backupDir, nil
}

func (m *Manager) restoreFromBackup(backupDir string) error {
	listsDir := m.cfg.ListsDir()
	userFiles := []string{
		"list-general-user.txt",
		"list-exclude-user.txt",
		"ipset-exclude-user.txt",
	}

	for _, f := range userFiles {
		src := filepath.Join(backupDir, f)
		dst := filepath.Join(listsDir, f)
		if _, err := os.Stat(src); err == nil {
			if err := copyFile(src, dst); err != nil {
				m.log.Error("updater", fmt.Sprintf("Restore %s failed: %v", f, err))
			} else {
				m.log.Info("updater", fmt.Sprintf("Restored: %s", f))
			}
		}
	}

	strategyFile := filepath.Join(backupDir, ".strategy")
	if data, err := os.ReadFile(strategyFile); err == nil {
		m.cfg.SetCurrentStrategy(strings.TrimSpace(string(data)))
	}

	return nil
}

func (m *Manager) extractUpdate(zipPath string) error {
	zapretDir := m.cfg.GetZapretPath()

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(zapretDir, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
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

func downloadFile(url, filepath string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func sendProgress(ch chan<- UpdateProgress, step string, pct int) {
	ch <- UpdateProgress{Step: step, Percent: pct}
}

func (m *Manager) InstallZapret(sourceDir string, progress chan<- UpdateProgress) error {
	defer close(progress)

	sendProgress(progress, "Checking source", 5)
	winws := filepath.Join(sourceDir, "bin", "winws.exe")
	if _, err := os.Stat(winws); os.IsNotExist(err) {
		return fmt.Errorf("zapret not found at: %s", sourceDir)
	}

	sendProgress(progress, "Backing up user lists", 10)
	backupDir, err := m.backupUserLists()
	if err == nil {
		defer m.restoreFromBackup(backupDir)
		defer os.RemoveAll(backupDir)
	}

	svc := m.GetServiceStatus()
	wasRunning := svc.Running
	prevStrategy := m.cfg.GetCurrentStrategy()

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
		return fmt.Errorf("copy files: %v: %s", err, string(output))
	}

	m.version = detectZapretVersion(m.cfg)
	m.log.Info("updater", fmt.Sprintf("Zapret installed, version: %s", m.version))

	if prevStrategy != "" {
		sendProgress(progress, "Restoring strategy", 70)
		m.cfg.SetCurrentStrategy(prevStrategy)
	}

	sendProgress(progress, "Starting zapret", 80)
	if wasRunning && prevStrategy != "" {
		m.InstallService(prevStrategy)
	}

	sendProgress(progress, "Installation complete", 100)
	return nil
}
