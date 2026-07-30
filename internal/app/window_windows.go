package app

import (
	"syscall"
	"unsafe"
)

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	procFindWindowW = user32.NewProc("FindWindowW")
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

	var getLong, setLong *syscall.LazyProc
	if uintptrSize() == 8 {
		getLong = user32.NewProc("GetWindowLongPtrW")
		setLong = user32.NewProc("SetWindowLongPtrW")
	} else {
		getLong = user32.NewProc("GetWindowLongW")
		setLong = user32.NewProc("SetWindowLongW")
	}

	style, _, _ := getLong.Call(hwnd, gwlStyle)
	newStyle := style &^ wsMaximizeBox
	setLong.Call(hwnd, gwlStyle, uintptr(newStyle))
}

func uintptrSize() int {
	var p uintptr
	return int(unsafe.Sizeof(p))
}
