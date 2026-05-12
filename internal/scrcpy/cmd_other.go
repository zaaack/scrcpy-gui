//go:build !windows

package scrcpy

import "os/exec"

func setNoWindowAttr(cmd *exec.Cmd) {
	// 非Windows平台无需设置CREATE_NO_WINDOW
}
