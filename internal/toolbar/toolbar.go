package toolbar

import (
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"scrcpy-gui/internal/scrcpy"
	"scrcpy-gui/internal/window"
)

// Button 表示一个工具栏按钮
type Button struct {
	Icon    string
	Tooltip string
	Action  func()
	widget  widget.Clickable
}

// Toolbar 表示悬浮工具栏
type Toolbar struct {
	instance *scrcpy.Instance
	tracker  window.Tracker
	window   *app.Window
	buttons  []Button
	theme    *material.Theme
	title    string
	running  bool
	pinned   bool
	mu       sync.Mutex
	stopCh   chan struct{}
}

// New 创建新的工具栏
func New(instance *scrcpy.Instance) *Toolbar {
	theme := material.NewTheme()
	theme.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Regular()))
	t := &Toolbar{
		instance: instance,
		tracker:  window.NewTracker(),
		theme:    theme,
		stopCh:   make(chan struct{}),
	}

	// 初始化按钮
	t.initButtons()

	return t
}

// initButtons 初始化工具栏按钮
func (t *Toolbar) initButtons() {
	t.buttons = []Button{
		{
			Icon:    "Vol+",
			Tooltip: "Vol+",
			Action:  t.pressVolumeUp,
		},
		{
			Icon:    "Vol-",
			Tooltip: "Vol-",
			Action:  t.pressVolumeDown,
		},
		{
			Icon:    "Rotate",
			Tooltip: "Rotate",
			Action:  t.rotate,
		},
		{
			Icon:    "Full",
			Tooltip: "Fullscreen",
			Action:  t.toggleFullscreen,
		},
		{
			Icon:    "Pin",
			Tooltip: "TopMost",
			Action:  t.toggleTopMost,
		},
		{
			Icon:    "Power",
			Tooltip: "Power",
			Action:  t.pressPower,
		},
		{
			Icon:    "Recent",
			Tooltip: "Recent",
			Action:  t.pressRecentApps,
		},
		{
			Icon:    "Home",
			Tooltip: "Home",
			Action:  t.pressHome,
		},
		{
			Icon:    "Back",
			Tooltip: "Back",
			Action:  t.pressBack,
		},
	}
}

// Run 运行工具栏
func (t *Toolbar) Run() error {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return fmt.Errorf("工具栏已在运行")
	}
	t.running = true
	t.mu.Unlock()

	// 使用设备序列号生成唯一窗口标题，避免多个工具栏窗口混淆
	t.title = fmt.Sprintf("Scrcpy Toolbar - %s", t.instance.GetSerial())

	// 创建窗口
	t.window = new(app.Window)
	t.window.Option(app.Title(t.title))
	t.window.Option(app.Size(unit.Dp(120), unit.Dp(500)))
	t.window.Option(app.MaxSize(unit.Dp(120), unit.Dp(500)))
	t.window.Option(app.MinSize(unit.Dp(120), unit.Dp(500)))

	// 启动位置跟踪goroutine
	go t.trackWindow()

	// 运行Gio事件循环
	go func() {
		if err := t.runWindow(); err != nil {
			log.Printf("工具栏窗口错误: %v", err)
		}
	}()

	// 等待窗口创建后隐藏任务栏图标
	go t.waitForWindowAndHideTaskbar()

	return nil
}

// waitForWindowAndHideTaskbar 等待窗口创建并隐藏任务栏图标
func (t *Toolbar) waitForWindowAndHideTaskbar() {
	maxAttempts := 50
	for i := 0; i < maxAttempts; i++ {
		time.Sleep(50 * time.Millisecond)
		toolbarHandle, err := t.tracker.FindWindow(t.title)
		if err == nil {
			t.tracker.HideFromTaskbar(toolbarHandle)
			return
		}
	}
	log.Printf("警告: 未找到工具栏窗口，无法隐藏任务栏图标")
}

// Stop 停止工具栏
func (t *Toolbar) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running {
		return
	}

	close(t.stopCh)
	t.running = false

	// 关闭Gio窗口
	if t.window != nil {
		t.window.Perform(system.ActionClose)
	}
}

// trackWindow 跟踪scrcpy窗口位置
func (t *Toolbar) trackWindow() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopCh:
			return
		case <-ticker.C:
			t.updatePosition()
		}
	}
}

// updatePosition 更新工具栏位置和层级
func (t *Toolbar) updatePosition() {
	// 获取scrcpy窗口位置
	scrcpyX, scrcpyY, scrcpyWidth, _, err := t.instance.GetWindowRect()
	if err != nil {
		return
	}

	// 计算工具栏位置（scrcpy窗口右侧）
	toolbarX := scrcpyX + scrcpyWidth + 5
	toolbarY := scrcpyY

	// 获取工具栏窗口句柄（通过唯一标题查找）
	toolbarHandle, err := t.tracker.FindWindow(t.title)
	if err != nil {
		return
	}

	// 获取工具栏当前实际尺寸（保持初始Gio布局的尺寸，不跟随scrcpy窗口变化）
	_, _, toolbarWidth, toolbarHeight, err := t.tracker.GetWindowRect(toolbarHandle)
	if err != nil {
		return
	}

	// 检查scrcpy窗口是否在前台
	scrcpyHandle := uintptr(t.instance.GetWindowHandle())
	if scrcpyHandle == 0 {
		return
	}

	foregroundHandle := t.tracker.GetForegroundWindow()
	isScrcpyForeground := (foregroundHandle == scrcpyHandle)
	isToolbarForeground := (foregroundHandle == toolbarHandle)
	isScrcpyMinimized := t.tracker.IsWindowMinimized(scrcpyHandle)

	if isScrcpyMinimized {
		// 如果scrcpy最小化，工具栏也最小化
		t.tracker.SetWindowPosWithZOrder(toolbarHandle, 0, 0, 0, 0, 0, 0x0040) // SWP_HIDEWINDOW
	} else if isScrcpyForeground || isToolbarForeground {
		// 如果scrcpy或工具栏在前台，工具栏也显示在前台
		t.tracker.BringWindowToTop(toolbarHandle)
		t.tracker.SetWindowPosWithZOrder(toolbarHandle, scrcpyHandle, toolbarX, toolbarY, toolbarWidth, toolbarHeight, 0x0010) // SWP_NOACTIVATE
	} else {
		// 如果scrcpy和工具栏都不在前台，工具栏也不在前台
		t.tracker.SetWindowPosWithZOrder(toolbarHandle, 1, toolbarX, toolbarY, toolbarWidth, toolbarHeight, 0x0010) // HWND_BOTTOM
	}
}

// runWindow 运行窗口事件循环
func (t *Toolbar) runWindow() error {
	var ops op.Ops

	for {
		select {
		case <-t.stopCh:
			return nil
		default:
			switch e := t.window.Event().(type) {
			case app.DestroyEvent:
				return e.Err
			case app.FrameEvent:
				gtx := app.NewContext(&ops, e)
				t.layout(gtx)
				e.Frame(gtx.Ops)
			}
		}
	}
}

// layout 布局工具栏
func (t *Toolbar) layout(gtx layout.Context) layout.Dimensions {
	// 检查按钮点击
	for i := range t.buttons {
		btn := &t.buttons[i]
		if btn.widget.Clicked(gtx) {
			btn.Action()
		}
	}

	// 使用Inset添加内边距
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// 创建垂直布局
		return layout.Flex{
			Axis: layout.Vertical,
		}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// 标题
				title := material.H6(t.theme, "Control")
				return layout.Inset{
					Bottom: unit.Dp(12),
				}.Layout(gtx, title.Layout)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// 按钮列表
				return t.layoutButtons(gtx)
			}),
		)
	})
}

// layoutButtons 布局按钮
func (t *Toolbar) layoutButtons(gtx layout.Context) layout.Dimensions {
	var children []layout.FlexChild

	theme := t.theme
	for i := range t.buttons {
		btn := &t.buttons[i]
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{
					Top:    unit.Dp(2),
					Bottom: unit.Dp(2),
					Left:   unit.Dp(4),
					Right:  unit.Dp(4),
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Button(theme, &btn.widget, btn.Icon).Layout(gtx)
				})
			}),
		)
	}

	return layout.Flex{
		Axis:    layout.Vertical,
		Spacing: layout.SpaceEvenly,
	}.Layout(gtx, children...)
}

// sendAdbKeyEvent 发送ADB按键事件
func (t *Toolbar) sendAdbKeyEvent(keyCode int) error {
	serial := t.instance.GetSerial()
	cmd := newNoWindowCmd("adb", "-s", serial, "shell", "input", "keyevent", strconv.Itoa(keyCode))
	return cmd.Run()
}

// 按钮动作实现
func (t *Toolbar) pressBack() {
	if err := t.sendAdbKeyEvent(window.KeyCodeBack); err != nil {
		log.Printf("发送返回键失败: %v", err)
	}
}

func (t *Toolbar) pressHome() {
	if err := t.sendAdbKeyEvent(window.KeyCodeHome); err != nil {
		log.Printf("发送Home键失败: %v", err)
	}
}

func (t *Toolbar) pressRecentApps() {
	if err := t.sendAdbKeyEvent(window.KeyCodeAppSwitch); err != nil {
		log.Printf("发送最近应用键失败: %v", err)
	}
}

func (t *Toolbar) pressVolumeUp() {
	if err := t.sendAdbKeyEvent(window.KeyCodeVolumeUp); err != nil {
		log.Printf("发送音量+键失败: %v", err)
	}
}

func (t *Toolbar) pressVolumeDown() {
	if err := t.sendAdbKeyEvent(window.KeyCodeVolumeDown); err != nil {
		log.Printf("发送音量-键失败: %v", err)
	}
}

func (t *Toolbar) pressPower() {
	if err := t.sendAdbKeyEvent(window.KeyCodePower); err != nil {
		log.Printf("发送电源键失败: %v", err)
	}
}

func (t *Toolbar) rotate() {
	if err := t.instance.Rotate(); err != nil {
		log.Printf("发送旋转快捷键失败: %v", err)
	}
}

func (t *Toolbar) toggleFullscreen() {
	if err := t.instance.ToggleFullscreen(); err != nil {
		log.Printf("发送全屏快捷键失败: %v", err)
	}
}

func (t *Toolbar) toggleTopMost() {
	scrcpyHandle := uintptr(t.instance.GetWindowHandle())
	if scrcpyHandle == 0 {
		log.Printf("获取scrcpy窗口句柄失败")
		return
	}

	t.mu.Lock()
	t.pinned = !t.pinned
	pinned := t.pinned
	t.mu.Unlock()

	if pinned {
		if !t.tracker.SetTopMost(scrcpyHandle) {
			log.Printf("设置窗口置顶失败")
		}
	} else {
		if !t.tracker.UnsetTopMost(scrcpyHandle) {
			log.Printf("取消窗口置顶失败")
		}
	}
}
