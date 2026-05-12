//go:build !windows

package toolbar

import "os/exec"

func newNoWindowCmd(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
