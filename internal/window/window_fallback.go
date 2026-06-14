//go:build !windows && !darwin && !linux

package window

import "fmt"

// FallbackTracker 用于未被显式支持的平台（如 FreeBSD 等）。
// 所有窗口控制操作都返回错误；工具栏会作为一个普通窗口运行，
// 基于 adb 的按钮（Back/Home/Recent/音量/电源/通知）仍可正常工作。
type FallbackTracker struct{}

// NewTracker 创建不支持平台的窗口跟踪器（存根）。
func NewTracker() Tracker {
	return &FallbackTracker{}
}

func (t *FallbackTracker) FindWindow(title string) (uintptr, error) {
	return 0, fmt.Errorf("当前平台不支持窗口查找")
}

func (t *FallbackTracker) GetWindowRect(handle uintptr) (x, y, width, height int, err error) {
	return 0, 0, 0, 0, fmt.Errorf("当前平台不支持获取窗口矩形")
}

func (t *FallbackTracker) SetWindowPos(handle uintptr, x, y, width, height int) error {
	return fmt.Errorf("当前平台不支持设置窗口位置")
}

func (t *FallbackTracker) IsWindowVisible(handle uintptr) (bool, error) {
	return false, fmt.Errorf("当前平台不支持检查窗口可见性")
}

func (t *FallbackTracker) SendKeyboardEvent(handle uintptr, keyCode int, ctrl bool, shift bool, alt bool) error {
	return fmt.Errorf("当前平台不支持发送键盘事件")
}

func (t *FallbackTracker) SendRotateShortcut(handle uintptr) error {
	return fmt.Errorf("当前平台不支持发送旋转快捷键")
}

func (t *FallbackTracker) SendFullscreenShortcut(handle uintptr) error {
	return fmt.Errorf("当前平台不支持发送全屏快捷键")
}

func (t *FallbackTracker) GetForegroundWindow() uintptr               { return 0 }
func (t *FallbackTracker) SetForegroundWindow(handle uintptr) bool    { return false }
func (t *FallbackTracker) IsWindowMinimized(handle uintptr) bool      { return false }
func (t *FallbackTracker) BringWindowToTop(handle uintptr) bool       { return false }
func (t *FallbackTracker) SetWindowPosWithZOrder(handle uintptr, insertAfter uintptr, x, y, width, height int, flags uint32) bool {
	return false
}
func (t *FallbackTracker) HideFromTaskbar(handle uintptr)    {}
func (t *FallbackTracker) SetTopMost(handle uintptr) bool    { return false }
func (t *FallbackTracker) UnsetTopMost(handle uintptr) bool  { return false }
