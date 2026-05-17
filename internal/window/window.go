package window

// Tracker 窗口跟踪器接口
type Tracker interface {
	// FindWindow 根据窗口标题查找窗口，返回窗口句柄（跨平台抽象）
	FindWindow(title string) (uintptr, error)
	
	// GetWindowRect 获取窗口位置和大小
	GetWindowRect(handle uintptr) (x, y, width, height int, err error)
	
	// SetWindowPos 设置窗口位置和大小
	SetWindowPos(handle uintptr, x, y, width, height int) error
	
	// IsWindowVisible 检查窗口是否可见
	IsWindowVisible(handle uintptr) (bool, error)
	
	// SendKeyboardEvent 发送键盘事件到窗口
	SendKeyboardEvent(handle uintptr, keyCode int, ctrl bool, shift bool, alt bool) error
	
	// SendRotateShortcut 发送旋转快捷键（Alt+Left）到窗口
	SendRotateShortcut(handle uintptr) error
	
	// SendFullscreenShortcut 发送全屏快捷键（Alt+F）到窗口
	SendFullscreenShortcut(handle uintptr) error
	
	// GetForegroundWindow 获取当前前台窗口句柄
	GetForegroundWindow() uintptr
	
	// SetForegroundWindow 设置窗口为前台
	SetForegroundWindow(handle uintptr) bool
	
	// IsWindowMinimized 检查窗口是否最小化
	IsWindowMinimized(handle uintptr) bool
	
	// BringWindowToTop 将窗口带到最前面
	BringWindowToTop(handle uintptr) bool
	
	// SetWindowPosWithZOrder 设置窗口位置和层级
	SetWindowPosWithZOrder(handle uintptr, insertAfter uintptr, x, y, width, height int, flags uint32) bool
	
	// HideFromTaskbar 隐藏窗口在任务栏中的显示
	HideFromTaskbar(handle uintptr)

	// SetTopMost 设置窗口为置顶
	SetTopMost(handle uintptr) bool

	// UnsetTopMost 取消窗口置顶
	UnsetTopMost(handle uintptr) bool
}

// KeyCode 键码常量
const (
	KeyCodeBack      = 4   // 返回键
	KeyCodeHome      = 3   // Home键
	KeyCodeAppSwitch = 187 // 最近应用键
	KeyCodeVolumeUp  = 24  // 音量增加
	KeyCodeVolumeDown = 25 // 音量减少
	KeyCodePower     = 26  // 电源键
)

