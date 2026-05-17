//go:build !windows

package window

import "fmt"

// OtherTracker 其他平台的窗口跟踪器（存根实现）
type OtherTracker struct{}

// NewTracker 创建其他平台的窗口跟踪器
func NewTracker() Tracker {
	return &OtherTracker{}
}

// FindWindow 根据窗口标题查找窗口
func (t *OtherTracker) FindWindow(title string) (uintptr, error) {
	return 0, fmt.Errorf("当前平台不支持窗口查找")
}

// GetWindowRect 获取窗口位置和大小
func (t *OtherTracker) GetWindowRect(handle uintptr) (x, y, width, height int, err error) {
	return 0, 0, 0, 0, fmt.Errorf("当前平台不支持获取窗口矩形")
}

// SetWindowPos 设置窗口位置和大小
func (t *OtherTracker) SetWindowPos(handle uintptr, x, y, width, height int) error {
	return fmt.Errorf("当前平台不支持设置窗口位置")
}

// IsWindowVisible 检查窗口是否可见
func (t *OtherTracker) IsWindowVisible(handle uintptr) (bool, error) {
	return false, fmt.Errorf("当前平台不支持检查窗口可见性")
}

// SendKeyboardEvent 发送键盘事件到窗口
func (t *OtherTracker) SendKeyboardEvent(handle uintptr, keyCode int, ctrl bool, shift bool, alt bool) error {
	return fmt.Errorf("当前平台不支持发送键盘事件")
}

// SendRotateShortcut 发送旋转快捷键到窗口
func (t *OtherTracker) SendRotateShortcut(handle uintptr) error {
	return fmt.Errorf("当前平台不支持发送旋转快捷键")
}

// SendFullscreenShortcut 发送全屏快捷键到窗口
func (t *OtherTracker) SendFullscreenShortcut(handle uintptr) error {
	return fmt.Errorf("当前平台不支持发送全屏快捷键")
}

// GetForegroundWindow 获取当前前台窗口句柄
func (t *OtherTracker) GetForegroundWindow() uintptr {
	return 0
}

// SetForegroundWindow 设置窗口为前台
func (t *OtherTracker) SetForegroundWindow(handle uintptr) bool {
	return false
}

// IsWindowMinimized 检查窗口是否最小化
func (t *OtherTracker) IsWindowMinimized(handle uintptr) bool {
	return false
}

// BringWindowToTop 将窗口带到最前面
func (t *OtherTracker) BringWindowToTop(handle uintptr) bool {
	return false
}

// SetWindowPosWithZOrder 设置窗口位置和层级
func (t *OtherTracker) SetWindowPosWithZOrder(handle uintptr, insertAfter uintptr, x, y, width, height int, flags uint32) bool {
	return false
}

// HideFromTaskbar 隐藏窗口在任务栏中的显示
func (t *OtherTracker) HideFromTaskbar(handle uintptr) {
	// 其他平台不支持
}

// SetTopMost 设置窗口为置顶
func (t *OtherTracker) SetTopMost(handle uintptr) bool {
	return false
}

// UnsetTopMost 取消窗口置顶
func (t *OtherTracker) UnsetTopMost(handle uintptr) bool {
	return false
}