package main

import (
	"syscall"
	"unsafe"
)

var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	procFindWindowW              = user32.NewProc("FindWindowW")
	procGetWindowLongPtrW        = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW        = user32.NewProc("SetWindowLongPtrW")
)

const wsMaximizeBox = 0x00010000
const gwlStyle = ^uintptr(15) // GWL_STYLE = -16

func disableMaximizeButton(windowTitle string) {
	titlePtr, _ := syscall.UTF16PtrFromString(windowTitle)
	hwnd, _, _ := procFindWindowW.Call(
		0,
		uintptr(unsafe.Pointer(titlePtr)),
	)
	if hwnd == 0 {
		return
	}

	style, _, _ := procGetWindowLongPtrW.Call(
		hwnd,
		gwlStyle,
	)

	newStyle := style &^ wsMaximizeBox

	procSetWindowLongPtrW.Call(
		hwnd,
		gwlStyle,
		uintptr(newStyle),
	)
}
