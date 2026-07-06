package singleinstance

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
)

const mutexName = "Local\\ZPUI-Shell-Instance-Mutex"

func Check() (cleanup func(), err error) {
	name, _ := windows.UTF16PtrFromString(mutexName)
	h, e := windows.CreateMutex(nil, false, name)
	if e == nil {
		return func() { windows.CloseHandle(h) }, nil
	}
	if h != 0 {
		windows.CloseHandle(h)
	}
	if e != windows.ERROR_ALREADY_EXISTS {
		return nil, fmt.Errorf("CreateMutex: %w", e)
	}

	otherPID := findOtherInstance()
	if otherPID != 0 {
		title, _ := windows.UTF16PtrFromString("ZPUI — уже запущен")
		msg, _ := windows.UTF16PtrFromString(
			fmt.Sprintf("Приложение уже запущено (PID: %d).\n\nЗакрыть другой экземпляр и открыть этот?", otherPID),
		)
		btn, _ := windows.MessageBox(windows.HWND(0), msg, title, windows.MB_YESNO|windows.MB_ICONWARNING|windows.MB_TOPMOST)
		if btn == 6 {
			exec.Command("taskkill", "/F", "/PID", strconv.Itoa(otherPID)).Run()
			name2, _ := windows.UTF16PtrFromString(mutexName)
			h2, e2 := windows.CreateMutex(nil, false, name2)
			if e2 == nil {
				return func() { windows.CloseHandle(h2) }, nil
			}
			if h2 != 0 {
				windows.CloseHandle(h2)
			}
		}
	}
	return nil, fmt.Errorf("другой экземпляр ZPUI уже запущен")
}

func findOtherInstance() int {
	myPID := os.Getpid()
	exeName := filepath.Base(os.Args[0])
	out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("IMAGENAME eq %s", exeName), "/FO", "CSV", "/NH").Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\",\"")
		if len(parts) < 2 {
			continue
		}
		pid, err := strconv.Atoi(strings.Trim(parts[1], "\""))
		if err != nil || pid == myPID {
			continue
		}
		return pid
	}
	return 0
}
