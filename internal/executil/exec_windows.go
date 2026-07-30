package executil

import (
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const CREATE_NO_WINDOW = 0x08000000
const CREATE_BREAKAWAY_FROM_JOB = 0x01000000

func HiddenCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: CREATE_NO_WINDOW,
	}
	return cmd
}

func DetachedCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: CREATE_NO_WINDOW | CREATE_BREAKAWAY_FROM_JOB,
	}
	return cmd
}

// GuiCmd запускает процесс как оторванный от родителя (CREATE_BREAKAWAY_FROM_JOB),
// но БЕЗ скрытия окна. Используется для GUI-модулей (selfupdate.exe),
// окно которых должно быть видно пользователю.
func GuiCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: CREATE_BREAKAWAY_FROM_JOB,
	}
	return cmd
}

var (
	shell32       = windows.NewLazySystemDLL("shell32.dll")
	procSetAppID  = shell32.NewProc("SetCurrentProcessExplicitAppUserModelID")
)

// SetProcessAppID устанавливает AppUserModelID для текущего процесса.
// Одинаковый AppID у ZPUI и selfupdate группирует их окна в панели задач Windows.
func SetProcessAppID(appID string) {
	ptr, _ := windows.UTF16PtrFromString(appID)
	procSetAppID.Call(uintptr(unsafe.Pointer(ptr)))
}