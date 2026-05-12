//go:build windows

package window

import (
	"fmt"
	"syscall"
	"time"

	"github.com/lxn/win"
)

// WindowsTracker Windows平台的窗口跟踪器
type WindowsTracker struct {
	user32 *syscall.LazyDLL
}

// NewTracker 创建Windows平台的窗口跟踪器
func NewTracker() Tracker {
	return &WindowsTracker{
		user32: syscall.NewLazyDLL("user32.dll"),
	}
}

// FindWindow 根据窗口标题查找窗口
func (t *WindowsTracker) FindWindow(title string) (uintptr, error) {
	titlePtr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return 0, fmt.Errorf("转换窗口标题失败: %w", err)
	}
	
	hwnd := win.FindWindow(nil, titlePtr)
	if hwnd == 0 {
		return 0, fmt.Errorf("未找到窗口: %s", title)
	}
	
	return uintptr(hwnd), nil
}

// GetWindowRect 获取窗口位置和大小
func (t *WindowsTracker) GetWindowRect(handle uintptr) (x, y, width, height int, err error) {
	hwnd := win.HWND(handle)
	var rect win.RECT
	
	if !win.GetWindowRect(hwnd, &rect) {
		return 0, 0, 0, 0, fmt.Errorf("获取窗口矩形失败")
	}
	
	x = int(rect.Left)
	y = int(rect.Top)
	width = int(rect.Right - rect.Left)
	height = int(rect.Bottom - rect.Top)
	
	return x, y, width, height, nil
}

// SetWindowPos 设置窗口位置和大小
func (t *WindowsTracker) SetWindowPos(handle uintptr, x, y, width, height int) error {
	hwnd := win.HWND(handle)
	
	// 使用SWP_NOZORDER标志，不改变窗口层级
	flags := uint32(win.SWP_NOZORDER | win.SWP_NOACTIVATE)
	
	if !win.SetWindowPos(hwnd, 0, int32(x), int32(y), int32(width), int32(height), flags) {
		return fmt.Errorf("设置窗口位置失败")
	}
	
	return nil
}

// IsWindowVisible 检查窗口是否可见
func (t *WindowsTracker) IsWindowVisible(handle uintptr) (bool, error) {
	hwnd := win.HWND(handle)
	return win.IsWindowVisible(hwnd), nil
}

// SendKeyboardEvent 发送键盘事件到窗口
func (t *WindowsTracker) SendKeyboardEvent(handle uintptr, keyCode int, ctrl bool, shift bool, alt bool) error {
	// 对于Scrcpy快捷键，我们需要使用PostMessage发送WM_KEYDOWN和WM_KEYUP
	// 这里简化处理，使用keybd_event模拟全局键盘事件
	// 注意：这会影响整个系统，更好的方法是使用Scrcpy的控制协议
	
	// 暂时使用简单的方法：通过adb shell input keyevent发送按键
	// 这将在scrcpy包中实现
	
	return nil
}

// SendRotateShortcut 发送旋转快捷键（Alt+Left）到窗口
func (t *WindowsTracker) SendRotateShortcut(handle uintptr) error {
	hwnd := win.HWND(handle)

	win.SetForegroundWindow(hwnd)
	time.Sleep(50 * time.Millisecond)

	// 按下Alt
	win.PostMessage(hwnd, win.WM_KEYDOWN, win.VK_MENU, 0)
	time.Sleep(10 * time.Millisecond)

	// 按下左箭头
	win.PostMessage(hwnd, win.WM_KEYDOWN, win.VK_LEFT, 0)
	time.Sleep(10 * time.Millisecond)

	// 释放左箭头
	win.PostMessage(hwnd, win.WM_KEYUP, win.VK_LEFT, 0)
	time.Sleep(10 * time.Millisecond)

	// 释放Alt
	win.PostMessage(hwnd, win.WM_KEYUP, win.VK_MENU, 0)

	return nil
}

// SendFullscreenShortcut 发送全屏快捷键（Alt+F）到窗口
func (t *WindowsTracker) SendFullscreenShortcut(handle uintptr) error {
	hwnd := win.HWND(handle)

	win.SetForegroundWindow(hwnd)
	time.Sleep(50 * time.Millisecond)

	// 按下Alt
	win.PostMessage(hwnd, win.WM_KEYDOWN, win.VK_MENU, 0)
	time.Sleep(10 * time.Millisecond)

	// 按下F键
	win.PostMessage(hwnd, win.WM_KEYDOWN, uintptr('F'), 0)
	time.Sleep(10 * time.Millisecond)

	// 释放F键
	win.PostMessage(hwnd, win.WM_KEYUP, uintptr('F'), 0)
	time.Sleep(10 * time.Millisecond)

	// 释放Alt
	win.PostMessage(hwnd, win.WM_KEYUP, win.VK_MENU, 0)

	return nil
}

// GetForegroundWindow 获取当前前台窗口句柄
func (t *WindowsTracker) GetForegroundWindow() uintptr {
	return uintptr(win.GetForegroundWindow())
}

// SetForegroundWindow 设置窗口为前台
func (t *WindowsTracker) SetForegroundWindow(handle uintptr) bool {
	return win.SetForegroundWindow(win.HWND(handle))
}

// IsWindowMinimized 检查窗口是否最小化
func (t *WindowsTracker) IsWindowMinimized(handle uintptr) bool {
	return win.IsIconic(win.HWND(handle))
}

// BringWindowToTop 将窗口带到最前面
func (t *WindowsTracker) BringWindowToTop(handle uintptr) bool {
	return win.BringWindowToTop(win.HWND(handle))
}

// SetWindowPosWithZOrder 设置窗口位置和层级
func (t *WindowsTracker) SetWindowPosWithZOrder(handle uintptr, insertAfter uintptr, x, y, width, height int, flags uint32) bool {
	return win.SetWindowPos(win.HWND(handle), win.HWND(insertAfter), int32(x), int32(y), int32(width), int32(height), flags)
}

// HideFromTaskbar 隐藏窗口在任务栏中的显示
func (t *WindowsTracker) HideFromTaskbar(handle uintptr) {
	hwnd := win.HWND(handle)
	
	// 获取当前扩展样式
	style := win.GetWindowLong(hwnd, win.GWL_EXSTYLE)
	
	// 添加WS_EX_TOOLWINDOW样式，使窗口不在任务栏显示
	style |= win.WS_EX_TOOLWINDOW
	
	// 设置新的扩展样式
	win.SetWindowLong(hwnd, win.GWL_EXSTYLE, style)
}