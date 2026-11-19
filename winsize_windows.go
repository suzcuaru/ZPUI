//go:build windows

package main

import "syscall"

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// defaultWindowSize adapts the base window size to the primary screen
// resolution via GetSystemMetrics(SM_CXSCREEN/SM_CYSCREEN).
func defaultWindowSize() (int, int) {
	const (
		smCXScreen = 0
		smCYScreen = 1
	)

	user32 := syscall.NewLazyDLL("user32.dll")
	procGetSystemMetrics := user32.NewProc("GetSystemMetrics")

	sw, _, _ := procGetSystemMetrics.Call(uintptr(smCXScreen))
	sh, _, _ := procGetSystemMetrics.Call(uintptr(smCYScreen))
	screenW, screenH := int(int32(sw)), int(int32(sh))
	if screenW <= 0 || screenH <= 0 {
		return 1280, 720
	}

	width := clamp(screenW*65/100, 900, 1280)
	height := clamp(screenH*70/100, 650, 720)
	return width, height
}
