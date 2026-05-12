//go:build windows

package toolbar

import (
	"os/exec"
	"syscall"
)

func newNoWindowCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	return cmd
}
