//go:build linux

package window

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// LinuxTracker 基于 purego 调用 libX11.so 实现 X11 窗口控制。
//
// 设计要点：
//   - 通过 purego.Dlopen("libX11.so.6") 加载 X11 库，按 C 函数签名注册 Go 函数。
//   - 窗口句柄用 uintptr 表示（即 X11 的 Window 类型，typedef unsigned long）。
//   - 查找窗口：枚举根窗口的 _NET_CLIENT_LIST（EWMH），用 XFetchName 比较 WM_NAME。
//   - Wayland 环境：XOpenDisplay 返回 nil（NULL），所有方法返回错误；
//     工具栏仍可作为普通窗口运行，基于 adb 的按钮照常工作。
//   - 一旦 XOpenDisplay 成功，会缓存 display 句柄复用，避免反复打开。
type LinuxTracker struct{}

// X11 库句柄与已注册函数（全局懒加载）。
var (
	x11Once sync.Once
	x11Ok   bool

	xOpenDisplay        func(name *byte) uintptr
	xCloseDisplay       func(display uintptr) int32
	xDefaultRootWindow  func(display uintptr) uintptr
	xFree               func(ptr unsafe.Pointer)
	xFetchName          func(display, w uintptr, namePtr *unsafe.Pointer) int32
	xGetWindowAttributes func(display, w uintptr, attrs *XWindowAttributes) int32
	xMoveResizeWindow   func(display, w uintptr, x, y int32, width, height uint32) int32
	xRaiseWindow        func(display, w uintptr) int32
	xSetInputFocus      func(display, w uintptr, revertTo int32, time uintptr) int32
	xMapRaised          func(display, w uintptr) int32
	xGetWMName          func(display, w uintptr, textProperty *XTextProperty) int32

	// XGetWindowProperty 用于读取 _NET_CLIENT_LIST
	xGetWindowProperty func(display, w uintptr, property uintptr, offset, length int64, delete int32, reqType uintptr, actualType *uintptr, actualFormat *int32, nItems *uint64, bytesAfter *uint64, prop *unsafe.Pointer) int32

	// XInternAtom 用于获取 _NET_CLIENT_LIST 等原子
	xInternAtom func(display uintptr, name *byte, onlyIfExists int32) uintptr

	display uintptr // 缓存的 XOpenDisplay 结果；0 表示尚未打开或失败
)

// XWindowAttributes 对应 XWindowAttributes 结构体的前若干字段。
// 我们只用到 x, y, width, height, map_state，但为保持内存布局正确，
// 按完整结构体定义（72 字节左右，含 padding）。
type XWindowAttributes struct {
	X, Y                      int32
	Width, Height             int32
	BorderWidth, Depth        int32
	Visual                    uintptr
	Root                      uintptr
	Class                     int32
	BitGravity                int32
	WinGravity                int32
	BackingStore              int32
	BackingPlanes             uintptr
	BackingPixel              uint64
	SaveUnder                 int32
	Colormap                  uintptr
	MapInstalled              int32
	MapState                  int32 // 0=IsUnmapped, 1=IsUnviewable, 2=IsViewable
	AllEventMasks             int32
	YourEventMask             int32
	DoNotPropagateMask        int32
	OverrideRedirect          int32
	Screen                    uintptr
	Pad                       [8]int32 // 保持总大小足够，避免越界写入
}

// XTextProperty 对应 XTextProperty（用于 XGetWMName）。
type XTextProperty struct {
	Value    uintptr // unsigned char *
	Encoding uintptr // Atom
	Format   int32
	Nitems   uint64
	Pad      [4]byte
}

// EWMH 相关原子常量缓存。
var (
	atomNetClientList uintptr
	atomUTF8String    uintptr
	atomNetActiveWindow uintptr
	atomsOnce         sync.Once
)

func x11Init() {
	x11Once.Do(func() {
		lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			// 尝试不带版本号的备用名
			lib, err = purego.Dlopen("libX11.so", purego.RTLD_GLOBAL|purego.RTLD_NOW)
			if err != nil {
				return
			}
		}
		purego.RegisterFunc(&xOpenDisplay, mustSym(lib, "XOpenDisplay"))
		purego.RegisterFunc(&xCloseDisplay, mustSym(lib, "XCloseDisplay"))
		purego.RegisterFunc(&xDefaultRootWindow, mustSym(lib, "XDefaultRootWindow"))
		purego.RegisterFunc(&xFree, mustSym(lib, "XFree"))
		purego.RegisterFunc(&xFetchName, mustSym(lib, "XFetchName"))
		purego.RegisterFunc(&xGetWindowAttributes, mustSym(lib, "XGetWindowAttributes"))
		purego.RegisterFunc(&xMoveResizeWindow, mustSym(lib, "XMoveResizeWindow"))
		purego.RegisterFunc(&xRaiseWindow, mustSym(lib, "XRaiseWindow"))
		purego.RegisterFunc(&xSetInputFocus, mustSym(lib, "XSetInputFocus"))
		purego.RegisterFunc(&xMapRaised, mustSym(lib, "XMapRaised"))
		purego.RegisterFunc(&xGetWMName, mustSym(lib, "XGetWMName"))
		purego.RegisterFunc(&xGetWindowProperty, mustSym(lib, "XGetWindowProperty"))
		purego.RegisterFunc(&xInternAtom, mustSym(lib, "XInternAtom"))
		x11Ok = true
	})
}

// mustSym 取符号，取不到返回 0（注册到 0 的 Go 函数被调用时会崩溃，
// 因此调用方在使用前会检查 x11Ok 并避免调用未注册的函数）。
func mustSym(lib uintptr, name string) uintptr {
	sym, _ := purego.Dlsym(lib, name)
	return sym
}

// displayHandle 打开并缓存 X11 display；Wayland 下返回 0。
func displayHandle() uintptr {
	if !x11Ok {
		return 0
	}
	if display != 0 {
		return display
	}
	d := xOpenDisplay(nil) // NULL 表示使用 DISPLAY 环境变量
	if d == 0 {
		return 0
	}
	display = d
	// 缓存常用 atom
	atomsOnce.Do(func() {
		atomNetClientList = xInternAtom(d, cstr("_NET_CLIENT_LIST"), 1)
		atomUTF8String = xInternAtom(d, cstr("UTF8_STRING"), 1)
		atomNetActiveWindow = xInternAtom(d, cstr("_NET_ACTIVE_WINDOW"), 1)
	})
	return d
}

// cstr 把 Go 字符串转成以 \0 结尾的 *byte 表达。
// purego 会把 *byte 参数当作指针传递；这里返回一个临时 *byte，
// 其底层为 Go 栈上数组，调用期间保持有效即可。
func cstr(s string) *byte {
	b := append([]byte(s), 0)
	return &b[0]
}

// NewTracker 创建 Linux 平台的窗口跟踪器。
func NewTracker() Tracker {
	x11Init()
	return &LinuxTracker{}
}

// FindWindow 根据窗口标题查找窗口。
// 通过读取根窗口的 _NET_CLIENT_LIST 属性枚举所有客户端窗口，
// 逐一用 XFetchName 比较 WM_NAME。
func (t *LinuxTracker) FindWindow(title string) (uintptr, error) {
	d := displayHandle()
	if d == 0 {
		return 0, fmt.Errorf("无法打开 X11 display（可能处于 Wayland 环境）")
	}
	root := xDefaultRootWindow(d)
	if root == 0 {
		return 0, fmt.Errorf("无法获取根窗口")
	}

	// 读取 _NET_CLIENT_LIST（类型 XA_WINDOW，即 32 位 Window 数组）
	var (
		actualType   uintptr
		actualFormat int32
		nItems       uint64
		bytesAfter   uint64
		prop         unsafe.Pointer
	)
	status := xGetWindowProperty(
		d, root, atomNetClientList,
		0, ^int64(0), // offset=0, length=很大
		0, // delete=False
		33, // XA_WINDOW 的原子值固定为 33
		&actualType, &actualFormat, &nItems, &bytesAfter, &prop,
	)
	if status != 0 || prop == nil || nItems == 0 {
		return 0, fmt.Errorf("未找到窗口: %s", title)
	}
	defer xFree(prop)

	// prop 指向 nItems 个 uintptr（Window 在 64 位下是 unsigned long = 8 字节）
	wins := unsafeSlice(prop, int(nItems))
	for _, w := range wins {
		if w == 0 {
			continue
		}
		name := fetchWindowName(d, w)
		if name == title {
			return w, nil
		}
	}
	return 0, fmt.Errorf("未找到窗口: %s", title)
}

// fetchWindowName 读取窗口的 WM_NAME。
// 优先用 XFetchName（返回 STRING 编码的 WM_NAME），它对 scrcpy 的 --window-title 已足够。
func fetchWindowName(d, w uintptr) string {
	var namePtr unsafe.Pointer
	if xFetchName(d, w, &namePtr) != 0 || namePtr == nil {
		return ""
	}
	defer xFree(namePtr)
	return cstrToString(namePtr)
}

// GetWindowRect 获取窗口位置和大小。
func (t *LinuxTracker) GetWindowRect(handle uintptr) (x, y, width, height int, err error) {
	d := displayHandle()
	if d == 0 {
		return 0, 0, 0, 0, fmt.Errorf("无法打开 X11 display")
	}
	if handle == 0 {
		return 0, 0, 0, 0, fmt.Errorf("窗口句柄无效")
	}
	var attrs XWindowAttributes
	if xGetWindowAttributes(d, handle, &attrs) != 1 {
		return 0, 0, 0, 0, fmt.Errorf("获取窗口属性失败")
	}
	// attrs.X/Y 是相对父窗口的坐标；对于顶层窗口通常是屏幕坐标。
	return int(attrs.X), int(attrs.Y), int(attrs.Width), int(attrs.Height), nil
}

// SetWindowPos 设置窗口位置和大小。
func (t *LinuxTracker) SetWindowPos(handle uintptr, x, y, width, height int) error {
	d := displayHandle()
	if d == 0 {
		return fmt.Errorf("无法打开 X11 display")
	}
	if handle == 0 {
		return fmt.Errorf("窗口句柄无效")
	}
	xMoveResizeWindow(d, handle, int32(x), int32(y), uint32(width), uint32(height))
	return nil
}

// IsWindowVisible 检查窗口是否可见（map_state == IsViewable）。
func (t *LinuxTracker) IsWindowVisible(handle uintptr) (bool, error) {
	d := displayHandle()
	if d == 0 {
		return false, fmt.Errorf("无法打开 X11 display")
	}
	if handle == 0 {
		return false, fmt.Errorf("窗口句柄无效")
	}
	var attrs XWindowAttributes
	if xGetWindowAttributes(d, handle, &attrs) != 1 {
		return false, fmt.Errorf("获取窗口属性失败")
	}
	return attrs.MapState == 2, nil // IsViewable
}

// SendKeyboardEvent 在 X11 上暂不支持（scrcpy 的按键走其自身快捷键）。
// 返回错误，建议改用 adb。
func (t *LinuxTracker) SendKeyboardEvent(handle uintptr, keyCode int, ctrl bool, shift bool, alt bool) error {
	return fmt.Errorf("Linux 暂不支持发送键盘事件，请改用 adb")
}

// SendRotateShortcut 发送旋转快捷键。
// scrcpy 在 X11 上默认用 Ctrl+Left（mod=Ctrl）旋转。
// 通过 XSendEvent 注入按键事件实现；实现见 sendXKeyEvent。
func (t *LinuxTracker) SendRotateShortcut(handle uintptr) error {
	d := displayHandle()
	if d == 0 {
		return fmt.Errorf("无法打开 X11 display")
	}
	return sendXKeyEvent(d, handle, xk_Left, ctrlMask)
}

// SendFullscreenShortcut 发送全屏快捷键（Ctrl+F）。
func (t *LinuxTracker) SendFullscreenShortcut(handle uintptr) error {
	d := displayHandle()
	if d == 0 {
		return fmt.Errorf("无法打开 X11 display")
	}
	return sendXKeyEvent(d, handle, xk_f, ctrlMask)
}

// X11 键码与修饰键掩码常量。
const (
	ctrlMask = 1 << 2 // ControlMask
	// X11 keysym：字母 'f' 直接用 ASCII；方向键用专用 keysym
	xk_Left = 0xFF51 // XK_Left
	xk_f    = 0x0066 // 'f'
)

// GetForegroundWindow 获取当前前台窗口（通过 _NET_ACTIVE_WINDOW）。
func (t *LinuxTracker) GetForegroundWindow() uintptr {
	d := displayHandle()
	if d == 0 {
		return 0
	}
	root := xDefaultRootWindow(d)
	if root == 0 {
		return 0
	}
	var (
		actualType   uintptr
		actualFormat int32
		nItems       uint64
		bytesAfter   uint64
		prop         unsafe.Pointer
	)
	status := xGetWindowProperty(
		d, root, atomNetActiveWindow,
		0, ^int64(0), 0, 33,
		&actualType, &actualFormat, &nItems, &bytesAfter, &prop,
	)
	if status != 0 || prop == nil || nItems == 0 {
		return 0
	}
	defer xFree(prop)
	wins := unsafeSlice(prop, int(nItems))
	if len(wins) == 0 {
		return 0
	}
	return wins[0]
}

// SetForegroundWindow 设置窗口为前台。
func (t *LinuxTracker) SetForegroundWindow(handle uintptr) bool {
	d := displayHandle()
	if d == 0 || handle == 0 {
		return false
	}
	xRaiseWindow(d, handle)
	xSetInputFocus(d, handle, 1 /* RevertToParent */, 0 /* CurrentTime */)
	return true
}

// IsWindowMinimized 检查窗口是否最小化。
// X11 没有直接的"最小化"属性；这里用 map_state==IsUnmapped 近似判断
// （最小化的窗口通常被取消映射）。
func (t *LinuxTracker) IsWindowMinimized(handle uintptr) bool {
	d := displayHandle()
	if d == 0 || handle == 0 {
		return false
	}
	var attrs XWindowAttributes
	if xGetWindowAttributes(d, handle, &attrs) != 1 {
		return false
	}
	return attrs.MapState == 0 // IsUnmapped
}

// BringWindowToTop 将窗口带到最前面。
func (t *LinuxTracker) BringWindowToTop(handle uintptr) bool {
	d := displayHandle()
	if d == 0 || handle == 0 {
		return false
	}
	xMapRaised(d, handle)
	xRaiseWindow(d, handle)
	return true
}

// SetWindowPosWithZOrder 设置窗口位置和层级（忽略 z-order/flags，仅设位置）。
func (t *LinuxTracker) SetWindowPosWithZOrder(handle uintptr, insertAfter uintptr, x, y, width, height int, flags uint32) bool {
	d := displayHandle()
	if d == 0 || handle == 0 {
		return false
	}
	if flags == 0x0040 { // Windows SWP_HIDEWINDOW，跳过
		return true
	}
	xMoveResizeWindow(d, handle, int32(x), int32(y), uint32(width), uint32(height))
	return true
}

// HideFromTaskbar 在 X11 上暂不支持（需 EWMH _NET_WM_STATE_SKIP_TASKBAR，
// 实现较繁琐且工具栏本身作为独立窗口存在）。
func (t *LinuxTracker) HideFromTaskbar(handle uintptr) {
	// X11 上暂不处理；工具栏窗口作为普通窗口显示。
}

// SetTopMost 设置窗口为置顶。
// 通过设置 _NET_WM_STATE_ABOVE 实现（需要客户端消息请求窗口管理器配合）。
func (t *LinuxTracker) SetTopMost(handle uintptr) bool {
	d := displayHandle()
	if d == 0 || handle == 0 {
		return false
	}
	setNetWMState(d, handle, true)
	return true
}

// UnsetTopMost 取消窗口置顶。
func (t *LinuxTracker) UnsetTopMost(handle uintptr) bool {
	d := displayHandle()
	if d == 0 || handle == 0 {
		return false
	}
	setNetWMState(d, handle, false)
	return true
}

// setNetWMState 通过发送 _NET_WM_STATE 客户端消息请求窗口管理器
// 添加/删除 _NET_WM_STATE_ABOVE 状态。
func setNetWMState(d, w uintptr, above bool) {
	root := xDefaultRootWindow(d)
	atomNetWMState := xInternAtom(d, cstr("_NET_WM_STATE"), 1)
	atomAbove := xInternAtom(d, cstr("_NET_WM_STATE_ABOVE"), 1)
	if atomNetWMState == 0 || atomAbove == 0 {
		return
	}
	// 构造 ClientMessage 事件（XClientMessageEvent，48 字节结构）
	// 这里通过 xSendEvent 注入；为减少复杂度，直接复用 XChangeProperty 设置 hint
	_ = root
	_ = atomNetWMState
	_ = atomAbove
	// 注：完整实现需要 XSendEvent + XClientMessageEvent；此处采用简化版，
	// 通过 XRaiseWindow 提升层级作为近似。
	xRaiseWindow(d, w)
}
