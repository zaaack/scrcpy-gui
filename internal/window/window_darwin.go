//go:build darwin

package window

import (
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// DarwinTracker 基于 purego/objc 调用 Cocoa/AppKit 与 CoreGraphics 实现 macOS 窗口控制。
//
// 设计要点：
//   - Objective-C 消息发送通过 purego/objc 的高层 API（ID.Send / objc.Send[T]）完成，
//     它内部正确处理了结构体返回值（arm64 用 objc_msgSend，amd64 大结构体用 objc_msgSend_stret）。
//   - 按值传递的结构体参数（NSRect/NSPoint/NSSize）可直接作为 Send 的参数传入。
//   - 键盘事件通过 CoreGraphics 的 CGEvent 注入。
//   - 窗口句柄用 uintptr 表示（即 objc.ID），在层间传递时与 Windows 保持一致。
type DarwinTracker struct{}

// NSPoint / NSSize / NSRect 对应 macOS 的同名结构体（CGFloat 在 64 位下为 double）。
type NSPoint struct {
	X, Y float64
}

type NSSize struct {
	Width, Height float64
}

type NSRect struct {
	Origin NSPoint
	Size   NSSize
}

// AppKit 窗口层级常量。
const (
	nsNormalWindowLevel   = 0 // NSNormalWindowLevel
	nsFloatingWindowLevel = 3 // NSFloatingWindowLevel（相当于置顶）
)

// CGEvent 类型与字段常量。
const (
	cgKeyDown   = 10 // kCGEventKeyDown
	cgKeyUp     = 11 // kCGEventKeyUp
	tapLocation = 0  // kCGHIDEventTap
)

// CGEvent 修饰键掩码。
const (
	maskCmd   = uint64(1 << 20) // kCGEventFlagMaskCommand
	maskAlt   = uint64(1 << 19) // kCGEventFlagMaskAlternate
	maskShift = uint64(1 << 17) // kCGEventFlagMaskShift
	maskCtrl  = uint64(1 << 18) // kCGEventFlagMaskControl
)

// macOS 虚拟键码（USB HID 键码）。
const (
	macVK_ANSI_F    = 0x03
	macVK_LeftArrow = 0x7B
)

// darwinOnce 保证 Cocoa 框架只加载一次。
var darwinOnce sync.Once

// msgSendString 注册为返回 string 的 objc_msgSend 变体。
// purego 会自动把返回的 char* 复制成 Go string。仅在第一次用到时注册。
var msgSendString func(receiver objc.ID, sel objc.SEL) string

// darwinInit 加载 Cocoa 框架（purego/objc 包的 init() 已加载 libobjc）。
func darwinInit() {
	darwinOnce.Do(func() {
		// 加载 Cocoa 框架，使 AppKit 类（NSApplication/NSWindow/NSScreen/NSString）可用
		_, _ = purego.Dlopen("/System/Library/Frameworks/Cocoa.framework/Cocoa", purego.RTLD_GLOBAL|purego.RTLD_LAZY)
		// 注册返回 string 的 objc_msgSend 变体；purego 会自动把返回的 char* 复制成 Go string。
		objcLib, _ := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if sym, err := purego.Dlsym(objcLib, "objc_msgSend"); err == nil {
			purego.RegisterFunc(&msgSendString, sym)
		}
	})
}

// 缓存常用的 selector（RegisterName 抓全局锁，缓存可避免重复调用）。
var (
	selSharedApplication = objc.RegisterName("sharedApplication")
	selOrderedWindows    = objc.RegisterName("orderedWindows")
	selCount             = objc.RegisterName("count")
	selObjectAtIndex     = objc.RegisterName("objectAtIndex:")
	selTitle             = objc.RegisterName("title")
	selUTF8String        = objc.RegisterName("UTF8String")
	selFrame             = objc.RegisterName("frame")
	selSetFrameOrigin    = objc.RegisterName("setFrameOrigin:")
	selSetFrameSize      = objc.RegisterName("setFrameSize:")
	selIsVisible         = objc.RegisterName("isVisible")
	selKeyWindow         = objc.RegisterName("keyWindow")
	selMakeKeyOrderFront = objc.RegisterName("makeKeyAndOrderFront:")
	selIsMiniaturized    = objc.RegisterName("isMiniaturized")
	selOrderFront        = objc.RegisterName("orderFront:")
	selSetLevel          = objc.RegisterName("setLevel:")
	selSetCanHide        = objc.RegisterName("setCanHide:")
	selMainScreen        = objc.RegisterName("mainScreen")
)

// NewTracker 创建 macOS 平台的窗口跟踪器。
func NewTracker() Tracker {
	darwinInit()
	return &DarwinTracker{}
}

// sharedApplication 返回共享的 NSApplication 实例。
func sharedApplication() objc.ID {
	nsApp := objc.GetClass("NSApplication")
	if nsApp == 0 {
		return 0
	}
	return objc.ID(nsApp).Send(selSharedApplication)
}

// FindWindow 根据窗口标题查找窗口。
// 遍历 [NSApp orderedWindows]，比较每个窗口的 title 与目标标题。
func (t *DarwinTracker) FindWindow(title string) (uintptr, error) {
	darwinInit()
	nsApp := sharedApplication()
	if nsApp == 0 {
		return 0, fmt.Errorf("无法获取 NSApplication")
	}
	windows := nsApp.Send(selOrderedWindows) // NSArray*
	if windows == 0 {
		return 0, fmt.Errorf("未找到窗口: %s", title)
	}
	count := windows.Send(selCount)
	for i := uint64(0); i < uint64(count); i++ {
		obj := windows.Send(selObjectAtIndex, uintptr(i))
		if obj == 0 {
			continue
		}
		titleID := obj.Send(selTitle) // NSString*
		if titleID == 0 {
			continue
		}
		// NSString 的 UTF8String 返回 C 字符串。直接用注册好的 msgSendString
		// 一步取出 title 对应的 UTF8 字符串（purego 会把 char* 复制成 Go string）。
		if msgSendString == nil {
			continue
		}
		got := msgSendString(titleID, selUTF8String)
		if got == title {
			return uintptr(obj), nil
		}
	}
	return 0, fmt.Errorf("未找到窗口: %s", title)
}

// primaryScreenHeight 返回主屏幕的高度（用于坐标原点转换）。
func primaryScreenHeight() float64 {
	screenCls := objc.GetClass("NSScreen")
	if screenCls == 0 {
		return 0
	}
	main := objc.ID(screenCls).Send(selMainScreen)
	if main == 0 {
		return 0
	}
	frame := objc.Send[NSRect](main, selFrame)
	return frame.Size.Height
}

// GetWindowRect 获取窗口位置和大小。
// macOS 坐标系原点在左下角，这里换算成左上角原点（与 Windows 一致）。
func (t *DarwinTracker) GetWindowRect(handle uintptr) (x, y, width, height int, err error) {
	if handle == 0 {
		return 0, 0, 0, 0, fmt.Errorf("窗口句柄无效")
	}
	rect := objc.Send[NSRect](objc.ID(handle), selFrame)
	screenH := primaryScreenHeight()
	x = int(rect.Origin.X)
	width = int(rect.Size.Width)
	height = int(rect.Size.Height)
	y = int(screenH) - int(rect.Origin.Y) - height
	return x, y, width, height, nil
}

// SetWindowPos 设置窗口位置和大小（输入坐标以左上角为原点）。
func (t *DarwinTracker) SetWindowPos(handle uintptr, x, y, width, height int) error {
	if handle == 0 {
		return fmt.Errorf("窗口句柄无效")
	}
	screenH := primaryScreenHeight()
	origin := NSPoint{X: float64(x), Y: screenH - float64(y) - float64(height)}
	size := NSSize{Width: float64(width), Height: float64(height)}
	objc.ID(handle).Send(selSetFrameOrigin, origin)
	objc.ID(handle).Send(selSetFrameSize, size)
	return nil
}

// IsWindowVisible 检查窗口是否可见。
func (t *DarwinTracker) IsWindowVisible(handle uintptr) (bool, error) {
	if handle == 0 {
		return false, fmt.Errorf("窗口句柄无效")
	}
	// isVisible 返回 BOOL，通过 objc.Send[bool] 获取
	visible := objc.Send[bool](objc.ID(handle), selIsVisible)
	return visible, nil
}

// SendKeyboardEvent 发送键盘事件到前台窗口。
// macOS 使用 CoreGraphics 的 CGEvent 注入按键，作用于当前键盘焦点。
func (t *DarwinTracker) SendKeyboardEvent(handle uintptr, keyCode int, ctrl, shift, alt bool) error {
	if handle == 0 {
		return fmt.Errorf("窗口句柄无效")
	}
	macVK, ok := mapVK(keyCode)
	if !ok {
		return fmt.Errorf("不支持的键码: %d（建议改用 adb 发送）", keyCode)
	}
	objc.ID(handle).Send(selMakeKeyOrderFront, objc.ID(0))
	var flags uint64
	if ctrl {
		flags |= maskCtrl
	}
	if shift {
		flags |= maskShift
	}
	if alt {
		flags |= maskAlt
	}
	postKeyEvent(macVK, flags)
	return nil
}

// SendRotateShortcut 发送旋转快捷键。
// scrcpy 在 macOS 上的默认修饰键是 Cmd，对应 mod+left 旋转。
func (t *DarwinTracker) SendRotateShortcut(handle uintptr) error {
	if handle == 0 {
		return fmt.Errorf("窗口句柄无效")
	}
	objc.ID(handle).Send(selMakeKeyOrderFront, objc.ID(0))
	postKeyEvent(macVK_LeftArrow, maskCmd)
	return nil
}

// SendFullscreenShortcut 发送全屏快捷键（Cmd+F）。
func (t *DarwinTracker) SendFullscreenShortcut(handle uintptr) error {
	if handle == 0 {
		return fmt.Errorf("窗口句柄无效")
	}
	objc.ID(handle).Send(selMakeKeyOrderFront, objc.ID(0))
	postKeyEvent(macVK_ANSI_F, maskCmd)
	return nil
}

// GetForegroundWindow 获取当前前台窗口句柄。
func (t *DarwinTracker) GetForegroundWindow() uintptr {
	nsApp := sharedApplication()
	if nsApp == 0 {
		return 0
	}
	return uintptr(nsApp.Send(selKeyWindow))
}

// SetForegroundWindow 设置窗口为前台。
func (t *DarwinTracker) SetForegroundWindow(handle uintptr) bool {
	if handle == 0 {
		return false
	}
	objc.ID(handle).Send(selMakeKeyOrderFront, objc.ID(0))
	return true
}

// IsWindowMinimized 检查窗口是否最小化。
func (t *DarwinTracker) IsWindowMinimized(handle uintptr) bool {
	if handle == 0 {
		return false
	}
	return objc.Send[bool](objc.ID(handle), selIsMiniaturized)
}

// BringWindowToTop 将窗口带到最前面。
func (t *DarwinTracker) BringWindowToTop(handle uintptr) bool {
	if handle == 0 {
		return false
	}
	objc.ID(handle).Send(selOrderFront, objc.ID(0))
	return true
}

// SetWindowPosWithZOrder 设置窗口位置和层级。
// macOS 上忽略 insertAfter/flags（为与 Windows 调用方签名兼容而保留）。
// 当 height 为 0 时（工具栏调用 SWP_HIDEWINDOW 场景）跳过设置。
func (t *DarwinTracker) SetWindowPosWithZOrder(handle uintptr, insertAfter uintptr, x, y, width, height int, flags uint32) bool {
	if handle == 0 {
		return false
	}
	// flags == 0x0040 对应 Windows 的 SWP_HIDEWINDOW，macOS 无对应操作，跳过
	if flags == 0x0040 {
		return true
	}
	screenH := primaryScreenHeight()
	origin := NSPoint{X: float64(x), Y: screenH - float64(y) - float64(height)}
	objc.ID(handle).Send(selSetFrameOrigin, origin)
	if width > 0 && height > 0 {
		size := NSSize{Width: float64(width), Height: float64(height)}
		objc.ID(handle).Send(selSetFrameSize, size)
	}
	return true
}

// HideFromTaskbar 隐藏窗口在 Dock 中的显示（轻量处理：取消 Cmd+H 隐藏行为）。
// macOS 上完全从 Dock 隐藏需要修改 NSApplication 的激活策略，会影响整个 App，
// 因此这里仅做窗口级别的处理，避免副作用。
func (t *DarwinTracker) HideFromTaskbar(handle uintptr) {
	if handle == 0 {
		return
	}
	objc.ID(handle).Send(selSetCanHide, false)
}

// SetTopMost 设置窗口为置顶（NSFloatingWindowLevel）。
func (t *DarwinTracker) SetTopMost(handle uintptr) bool {
	if handle == 0 {
		return false
	}
	objc.ID(handle).Send(selSetLevel, nsFloatingWindowLevel)
	return true
}

// UnsetTopMost 取消窗口置顶（NSNormalWindowLevel）。
func (t *DarwinTracker) UnsetTopMost(handle uintptr) bool {
	if handle == 0 {
		return false
	}
	objc.ID(handle).Send(selSetLevel, nsNormalWindowLevel)
	return true
}

// —— CoreGraphics 键盘事件注入 ——
//
// CGEventCreateKeyboardEvent 和相关函数来自 CoreGraphics 框架，
// 参数都是基本类型，便于通过 purego 调用。

var (
	cgOnce                   sync.Once
	cgOk                     bool
	cgEventCreateKeyboardEvent func(source uintptr, keyCode uint16, keyDown bool) uintptr
	cgEventSetFlags          func(event uintptr, flags uint64)
	cgEventPost              func(tap uintptr, event uintptr)
	cgEventRelease           func(event uintptr)
)

func cgInit() {
	cgOnce.Do(func() {
		cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/Versions/Current/CoreGraphics", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			return
		}
		if sym, e := purego.Dlsym(cg, "CGEventCreateKeyboardEvent"); e == nil {
			purego.RegisterFunc(&cgEventCreateKeyboardEvent, sym)
		}
		if sym, e := purego.Dlsym(cg, "CGEventSetFlags"); e == nil {
			purego.RegisterFunc(&cgEventSetFlags, sym)
		}
		if sym, e := purego.Dlsym(cg, "CGEventPost"); e == nil {
			purego.RegisterFunc(&cgEventPost, sym)
		}
		if sym, e := purego.Dlsym(cg, "CGEventRelease"); e == nil {
			purego.RegisterFunc(&cgEventRelease, sym)
		}
		cgOk = true
	})
}

// postKeyEvent 发送一次 keydown + keyup。
func postKeyEvent(macVK int, flags uint64) {
	cgInit()
	if !cgOk {
		return
	}
	if down := cgEventCreateKeyboardEvent(0, uint16(macVK), true); down != 0 {
		cgEventSetFlags(down, flags)
		cgEventPost(tapLocation, down)
		cgEventRelease(down)
	}
	if up := cgEventCreateKeyboardEvent(0, uint16(macVK), false); up != 0 {
		cgEventSetFlags(up, flags)
		cgEventPost(tapLocation, up)
		cgEventRelease(up)
	}
}

// mapVK 把 Windows/Android 风格的 keyCode 映射到 macOS 虚拟键码。
// 工具栏的实际按键动作都走 adb，这里只覆盖 scrcpy 快捷键用到的字母与方向键。
func mapVK(code int) (int, bool) {
	switch code {
	case 'F', 'f':
		return macVK_ANSI_F, true
	}
	// 直接传入 macOS 键码也兼容
	if code >= 0 && code <= 0x7F {
		return code, true
	}
	return 0, false
}
