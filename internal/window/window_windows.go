//go:build windows

package window

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"github.com/lxn/win"
)

// WindowsTracker Windows平台的窗口跟踪器
type WindowsTracker struct {
	user32        *syscall.LazyDLL
	mapVirtualKey *syscall.LazyProc
}

// NewTracker 创建Windows平台的窗口跟踪器
func NewTracker() Tracker {
	dll := syscall.NewLazyDLL("user32.dll")
	return &WindowsTracker{
		user32:        dll,
		mapVirtualKey: dll.NewProc("MapVirtualKeyW"),
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

// getScanCode 通过MapVirtualKey获取虚拟键码对应的扫描码
func (t *WindowsTracker) getScanCode(vk uint16) uint16 {
	ret, _, _ := t.mapVirtualKey.Call(uintptr(vk), 0)
	return uint16(ret)
}

// sendInputKeys 使用SendInput发送一组按键（按下+松开），模拟硬件级输入
// SendInput会更新系统物理按键状态表，比PostMessage更可靠，SDL窗口能正确接收
func (t *WindowsTracker) sendInputKeys(hwnd win.HWND, keys ...uint16) error {
	// 先将目标窗口设为前台
	win.SetForegroundWindow(hwnd)
	time.Sleep(50 * time.Millisecond)

	for _, vk := range keys {
		scan := t.getScanCode(vk)
		flags := uint32(0)
		// 左箭头是扩展键，需要设置KEYEVENTF_EXTENDEDKEY标志
		if vk == win.VK_LEFT || vk == win.VK_RIGHT || vk == win.VK_UP || vk == win.VK_DOWN {
			flags |= win.KEYEVENTF_EXTENDEDKEY
		}

		down := win.KEYBD_INPUT{
			Type: win.INPUT_KEYBOARD,
			Ki: win.KEYBDINPUT{
				WVk:     vk,
				WScan:   scan,
				DwFlags: flags,
			},
		}
		win.SendInput(1, unsafe.Pointer(&down), int32(unsafe.Sizeof(down)))
		time.Sleep(10 * time.Millisecond)
	}

	// 按逆序松开所有按键
	for i := len(keys) - 1; i >= 0; i-- {
		vk := keys[i]
		scan := t.getScanCode(vk)
		flags := uint32(win.KEYEVENTF_KEYUP)
		if vk == win.VK_LEFT || vk == win.VK_RIGHT || vk == win.VK_UP || vk == win.VK_DOWN {
			flags |= win.KEYEVENTF_EXTENDEDKEY
		}

		up := win.KEYBD_INPUT{
			Type: win.INPUT_KEYBOARD,
			Ki: win.KEYBDINPUT{
				WVk:     vk,
				WScan:   scan,
				DwFlags: flags,
			},
		}
		win.SendInput(1, unsafe.Pointer(&up), int32(unsafe.Sizeof(up)))
		time.Sleep(10 * time.Millisecond)
	}

	return nil
}

// SendKeyboardEvent 发送键盘事件到窗口
func (t *WindowsTracker) SendKeyboardEvent(handle uintptr, keyCode int, ctrl bool, shift bool, alt bool) error {
	keys := []uint16{}
	if ctrl {
		keys = append(keys, win.VK_CONTROL)
	}
	if shift {
		keys = append(keys, win.VK_SHIFT)
	}
	if alt {
		keys = append(keys, win.VK_MENU)
	}
	keys = append(keys, uint16(keyCode))
	return t.sendInputKeys(win.HWND(handle), keys...)
}

// SendRotateShortcut 发送旋转快捷键（Alt+Left）到窗口
func (t *WindowsTracker) SendRotateShortcut(handle uintptr) error {
	return t.sendInputKeys(win.HWND(handle), win.VK_MENU, win.VK_LEFT)
}

// SendFullscreenShortcut 发送全屏快捷键（Alt+F）到窗口
func (t *WindowsTracker) SendFullscreenShortcut(handle uintptr) error {
	return t.sendInputKeys(win.HWND(handle), win.VK_MENU, 'F')
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

// SetTopMost 设置窗口为置顶
func (t *WindowsTracker) SetTopMost(handle uintptr) bool {
	hwnd := win.HWND(handle)
	return win.SetWindowPos(hwnd, win.HWND_TOPMOST, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOACTIVATE)
}

// UnsetTopMost 取消窗口置顶
func (t *WindowsTracker) UnsetTopMost(handle uintptr) bool {
	hwnd := win.HWND(handle)
	return win.SetWindowPos(hwnd, win.HWND_NOTOPMOST, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOACTIVATE)
}