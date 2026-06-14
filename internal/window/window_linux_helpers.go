//go:build linux

package window

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// unsafeSlice 把 X11 返回的属性指针当成 []uintptr（Window 数组）。
// XGetWindowProperty 返回的 prop 是一段连续内存，元素大小由 actualFormat 决定；
// 对 XA_WINDOW（format=32）在 64 位系统上每元素 8 字节。
func unsafeSlice(prop unsafe.Pointer, n int) []uintptr {
	if prop == nil || n <= 0 {
		return nil
	}
	// Window 是 unsigned long，64 位下 8 字节；直接按 uintptr 数组取 n 个元素。
	return unsafe.Slice((*uintptr)(prop), n)
}

// cstrToString 把 C 风格 char* 读成 Go string，遇到 \0 截断。
func cstrToString(ptr unsafe.Pointer) string {
	if ptr == nil {
		return ""
	}
	// 用 unsafe.Add + 类型转换避免 vet 报"possible misuse of unsafe.Pointer"。
	var buf []byte
	for i := uintptr(0); ; i++ {
		b := *(*byte)(unsafe.Add(ptr, i))
		if b == 0 {
			break
		}
		buf = append(buf, b)
	}
	return string(buf)
}

// —— XSendEvent 用于注入键盘事件 ——
//
// XSendEvent(Display*, Window w, Bool propagate, long event_mask, XEvent* event)
// XEvent 是 union，最大成员 192 字节；我们构造一个 XKeyEvent 并填充前若干字段。
// XKeyEvent 结构体布局（sizeof(long) == 8 的 64 位 Linux）：
//
//	typedef union _XEvent {
//	    int type;                  // offset 0
//	    XAnyEvent xany;
//	    XKeyEvent xkey;            // 我们用这个
//	    ...
//	} XEvent;
//
// XKeyEvent 的字段顺序：
//   int type;            // KeyPress=2 / KeyRelease=3
//   unsigned long serial;
//   Bool send_event;
//   Display *display;
//   Window window;
//   Window root;
//   Window subwindow;
//   Time time;
//   int x, y;
//   int x_root, y_root;
//   unsigned int state;     // 修饰键掩码
//   unsigned int keycode;   // 物理键码（不是 keysym！）
//   Bool same_screen;
//
// 注意：XKeyEvent 用的是 keycode（物理键码），不是 keysym。
// 需要先用 XKeysymToKeycode 把 keysym 转成 keycode。

var (
	xSendEvent        func(display, w uintptr, propagate int32, eventMask int64, event *XEvent) int32
	xKeysymToKeycode  func(display uintptr, keysym uintptr) uint8
)

// XEvent 是 XEvent union 的最小占位（192 字节，足够容纳 XKeyEvent）。
type XEvent struct {
	data [24]uintptr // 24*8 = 192 字节
}

func init() {
	// 在 x11Init 之后注册；这里用 init 只是声明依赖，实际注册在 ensureXSendEvent 里。
}

// ensureXSendEvent 延迟注册 XSendEvent/XKeysymToKeycode（避免在 X11 不可用时注册到 0 符号）。
var sendEventOnce sync.Once

func ensureXSendEvent() {
	sendEventOnce.Do(func() {
		if !x11Ok {
			return
		}
		lib, _ := purego.Dlopen("libX11.so.6", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if lib == 0 {
			lib, _ = purego.Dlopen("libX11.so", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		}
		if lib == 0 {
			return
		}
		if sym, err := purego.Dlsym(lib, "XSendEvent"); err == nil {
			purego.RegisterFunc(&xSendEvent, sym)
		}
		if sym, err := purego.Dlsym(lib, "XKeysymToKeycode"); err == nil {
			purego.RegisterFunc(&xKeysymToKeycode, sym)
		}
	})
}

// sendXKeyEvent 向目标窗口注入一次按键（KeyPress + KeyRelease）。
// w 是目标窗口句柄；keysym 是 X11 keysym；state 是修饰键掩码。
func sendXKeyEvent(d, w uintptr, keysym uintptr, state uint) error {
	ensureXSendEvent()
	if xSendEvent == nil || xKeysymToKeycode == nil {
		return errNoXSend
	}
	keycode := xKeysymToKeycode(d, keysym)
	if keycode == 0 {
		return errNoKeycode
	}
	// XKeyEvent 字段（按 union 偏移填入 XEvent.data）：
	//   data[0] = type          (KeyPress=2 / KeyRelease=3)
	//   data[1] = serial        (0)
	//   data[2] = send_event    (true)
	//   data[3] = display       (d)
	//   data[4] = window        (w)
	//   data[5] = root          (root window)
	//   data[6] = subwindow     (0)
	//   data[7] = time          (0 = CurrentTime)
	//   data[8] = x             (0)
	//   data[9] = y             (0)
	//   data[10]= x_root        (0)
	//   data[11]= y_root        (0)
	//   data[12]= state         (修饰键)
	//   data[13]= keycode
	//   data[14]= same_screen   (true)
	root := xDefaultRootWindow(d)

	// KeyPress
	var ev XEvent
	ev.data[0] = 2 // KeyPress
	ev.data[2] = 1
	ev.data[3] = d
	ev.data[4] = w
	ev.data[5] = root
	ev.data[8] = 0
	ev.data[9] = 0
	ev.data[12] = uintptr(state)
	ev.data[13] = uintptr(keycode)
	ev.data[14] = 1
	xSendEvent(d, w, 1, 0x01 /* KeyPressMask */, &ev)

	// KeyRelease（type=3）
	ev.data[0] = 3 // KeyRelease
	xSendEvent(d, w, 1, 0x02 /* KeyReleaseMask */, &ev)
	return nil
}

// 错误占位（避免在热路径上分配字符串）。
var (
	errNoXSend   = errStr("XSendEvent 不可用")
	errNoKeycode = errStr("无法解析 keycode")
)

type errStr string

func (e errStr) Error() string { return string(e) }
