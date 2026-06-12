package executil

import (
	"os/exec"
	"syscall"
)

const CREATE_NO_WINDOW = 0x08000000

// HiddenCmd creates an exec.Cmd with CREATE_NO_WINDOW flag set,
// so no console window appears when spawning subprocesses.
func HiddenCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: CREATE_NO_WINDOW,
	}
	return cmd
}