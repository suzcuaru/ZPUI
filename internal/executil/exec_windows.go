package executil

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
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

func RunWithTimeout(timeout time.Duration, name string, args ...string) (string, error) {
	cmd := HiddenCmd(name, args...)

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		return buf.String(), err

	case <-time.After(timeout):
		pid := cmd.Process.Pid
		// Скрытый taskkill: видимое консольное окно при каждом таймауте
		// фоновой команды спамит пользователю (особенно на Win10/слабых ПК).
		kill := HiddenCmd("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
		kill.Run()

		select {
		case err := <-done:
			return buf.String(), err
		case <-time.After(3 * time.Second):
			os.Stderr.WriteString(fmt.Sprintf("RunWithTimeout: process %d did not die after taskkill\n", pid))
			return buf.String(), fmt.Errorf("timeout after %s (process %d)", timeout, pid)
		}
	}
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