package executil

import (
	"os/exec"
	"syscall"
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