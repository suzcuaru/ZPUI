package window

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var browserCmd *exec.Cmd

func Open(port int) {
	url := fmt.Sprintf("http://localhost:%d", port)

	profileDir := filepath.Join(os.TempDir(), "zpui-app-profile")
	_ = os.MkdirAll(profileDir, 0755)

	programFiles := os.Getenv("ProgramFiles")
	programFilesX86 := os.Getenv("ProgramFiles(x86)")
	localAppData := os.Getenv("LOCALAPPDATA")

	browserPaths := []string{
		filepath.Join(programFiles, "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(programFilesX86, "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(localAppData, "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(programFiles, "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(programFilesX86, "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(localAppData, "Google", "Chrome", "Application", "chrome.exe"),
	}

	for _, p := range browserPaths {
		if _, err := os.Stat(p); err == nil {
			args := []string{
				fmt.Sprintf(`--app=%s`, url),
				fmt.Sprintf(`--user-data-dir=%s`, profileDir),
				`--proxy-server=direct://`,
				`--disable-extensions`,
				`--disable-translate`,
				`--disable-background-networking`,
				`--disable-sync`,
				`--no-default-browser-check`,
				`--no-first-run`,
				`--disable-default-apps`,
				`--disable-popup-blocking`,
				`--force-color-profile=srgb`,
				`--window-size=1100,850`,
			}
			cmd := exec.Command(p, args...)
			if err := cmd.Start(); err == nil {
				browserCmd = cmd
				go cmd.Wait()
			}
			return
		}
	}

	exec.Command("cmd", "/c", "start", "", url).Start()
}

func Close() {
	if browserCmd != nil && browserCmd.Process != nil {
		browserCmd.Process.Kill()
		browserCmd = nil
	}
}

func IsWindowsDarkTheme() bool {
	out, err := exec.Command("reg", "query",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`,
		"/v", "AppsUseLightTheme").CombinedOutput()
	if err != nil {
		return true
	}
	return !strings.Contains(string(out), "0x1")
}
