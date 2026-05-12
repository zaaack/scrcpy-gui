//go:build windows

package scrcpy

import (
	"os/exec"
	"syscall"
)

func setNoWindowAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
}
