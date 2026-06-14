//go:build !windows

package tools

import "syscall"

// noWindowSysProcAttr 非 Windows 平台无操作占位。
func noWindowSysProcAttr() *syscall.SysProcAttr {
	return nil
}
