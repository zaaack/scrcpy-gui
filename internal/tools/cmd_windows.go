//go:build windows

package tools

import "syscall"

// noWindowSysProcAttr 返回 Windows 下隐藏子进程控制台窗口的 SysProcAttr。
func noWindowSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
