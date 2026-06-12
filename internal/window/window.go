package window

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var browserCmd *exec.Cmd

const (
	wsCaption    = 0x00C00000
	wsThickFrame = 0x00040000
	wsSysMenu    = 0x00080000
	wsMinimize   = 0x20000000
	wsMaximize   = 0x01000000
	wsPopup      = 0x80000000
	wsVisible    = 0x10000000
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procEnumWindows  = user32.NewProc("EnumWindows")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procSetWindowLong = user32.NewProc("SetWindowLongW")
	procSetWindowPos  = user32.NewProc("SetWindowPos")
	procShowWindow    = user32.NewProc("ShowWindow")
)

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
				`--window-size=1000,800`,
				`--window-position=200,100`,
			}
			cmd := exec.Command(p, args...)
			cmd.SysProcAttr = &syscall.SysProcAttr{
				HideWindow: true,
			}
			if err := cmd.Start(); err == nil {
				browserCmd = cmd
				go func() {
					time.Sleep(800 * time.Millisecond)
					removeTitleBar(cmd.Process.Pid)
					cmd.Wait()
				}()
			}
			return
		}
	}

	exec.Command("cmd", "/c", "start", "", url).Start()
}

func removeTitleBar(pid int) {
	enumCallback := syscall.NewCallback(func(hwnd syscall.Handle, _ uintptr) uintptr {
		var windowPid uint32
		procGetWindowThreadProcessId.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&windowPid)))
		if windowPid != uint32(pid) {
			return 1
		}

		gwlStyle := ^uintptr(15) // GWL_STYLE = -16

		style, _, _ := procSetWindowLong.Call(
			uintptr(hwnd),
			gwlStyle,
			0,
		)
		if style == 0 {
			return 1
		}

		newStyle := style &^ wsCaption &^ wsThickFrame &^ wsSysMenu &^ wsMinimize &^ wsMaximize
		newStyle |= wsPopup

		procSetWindowLong.Call(
			uintptr(hwnd),
			gwlStyle,
			newStyle,
		)

		procSetWindowPos.Call(
			uintptr(hwnd),
			0,
			200, 100,
			1000, 800,
			0x0040, // SWP_FRAMECHANGED
		)

		procShowWindow.Call(uintptr(hwnd), 5) // SW_SHOW

		return 1
	})

	procEnumWindows.Call(enumCallback, 0)
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
