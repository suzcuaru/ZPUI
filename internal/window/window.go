package window

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"zpui/internal/executil"

	"github.com/webview/webview_go"
)

var (
	mu sync.Mutex
	w  webview.WebView
)

func Open(url string) {
	mu.Lock()
	if w != nil {
		mu.Unlock()
		return
	}
	mu.Unlock()

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		wv := webview.New(false)
		if wv == nil {
			openInBrowser(url)
			return
		}

		mu.Lock()
		w = wv
		mu.Unlock()

		wv.SetTitle("ZPUI")
		wv.SetSize(1200, 800, webview.HintNone)
		wv.SetSize(900, 700, webview.HintMin)
		wv.Navigate(url)
		wv.Run()

		mu.Lock()
		w = nil
		mu.Unlock()

		wv.Destroy()
	}()
}

func Close() {
	mu.Lock()
	wv := w
	mu.Unlock()

	if wv != nil {
		wv.Terminate()
	}
}

// IsOpen returns true if the webview window is currently open
func IsOpen() bool {
	mu.Lock()
	defer mu.Unlock()
	return w != nil
}

// Toggle shows or hides the window. If open → hide (terminate), if closed → open.
func Toggle(url string) {
	mu.Lock()
	wv := w
	mu.Unlock()

	if wv != nil {
		wv.Terminate()
	} else {
		Open(url)
	}
}

func openInBrowser(url string) {
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
				`--window-size=1100,1050`,
			}
			cmd := executil.HiddenCmd(p, args...)
			cmd.Start()
			return
		}
	}

	executil.HiddenCmd("cmd", "/c", "start", "", url).Start()
}

func IsWindowsDarkTheme() bool {
	out, err := executil.HiddenCmd("reg", "query",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`,
		"/v", "AppsUseLightTheme").CombinedOutput()
	if err != nil {
		return true
	}
	return !strings.Contains(string(out), "0x1")
}