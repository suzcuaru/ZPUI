//go:build windows

package tray

import (
	"syscall"
)

var (
	modKernel32         = syscall.NewLazyDLL("kernel32.dll")
	modUser32ForConsole = syscall.NewLazyDLL("user32.dll")

	procGetConsoleWindow = modKernel32.NewProc("GetConsoleWindow")
	procShowWindow       = modUser32ForConsole.NewProc("ShowWindow")
	procIsWindowVisible  = modUser32ForConsole.NewProc("IsWindowVisible")
)

func getConsoleHWND() uintptr {
	hwnd, _, _ := procGetConsoleWindow.Call()
	return hwnd
}

func HideConsole() {
	hwnd := getConsoleHWND()
	if hwnd != 0 {
		procShowWindow.Call(hwnd, 0)
	}
}

func ShowConsole() {
	hwnd := getConsoleHWND()
	if hwnd != 0 {
		procShowWindow.Call(hwnd, 1)
	}
}

func IsConsoleVisible() bool {
	hwnd := getConsoleHWND()
	if hwnd == 0 {
		return false
	}
	ret, _, _ := procIsWindowVisible.Call(hwnd)
	return ret != 0
}
