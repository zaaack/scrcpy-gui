package main_window

import (
	"image/color"
	"log"
	"strings"
	"sync"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"scrcpy-gui/internal/adb"
	"scrcpy-gui/internal/config"
	"scrcpy-gui/internal/scrcpy"
	"scrcpy-gui/internal/toolbar"
)

// DeviceItem 表示设备列表中的一项
type DeviceItem struct {
	Device    adb.Device
	Running   bool
	startBtn  widget.Clickable
	stopBtn   widget.Clickable
}

// Window 主窗口
type Window struct {
	configManager *config.ConfigManager
	devices       []DeviceItem
	instances     map[string]*scrcpy.Instance
	toolbars      map[string]*toolbar.Toolbar
	window        *app.Window
	refreshBtn    widget.Clickable
	ipEditor      widget.Editor
	connectBtn    widget.Clickable
	theme         *material.Theme
	mu            sync.Mutex
}

// New 创建主窗口
func New(configManager *config.ConfigManager) *Window {
	theme := material.NewTheme()
	theme.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Regular()))
	return &Window{
		configManager: configManager,
		instances:     make(map[string]*scrcpy.Instance),
		toolbars:      make(map[string]*toolbar.Toolbar),
		theme:         theme,
	}
}

// Run 运行主窗口
func (w *Window) Run() {
	w.window = new(app.Window)
	w.window.Option(app.Title("Scrcpy GUI - Devices"))
	w.window.Option(app.Size(unit.Dp(400), unit.Dp(400)))

	w.ipEditor.SingleLine = true
	w.ipEditor.Submit = true

	w.refreshDevices()

	go func() {
		if err := w.runWindow(); err != nil {
			log.Printf("主窗口错误: %v", err)
		}
	}()
}

// refreshDevices 刷新设备列表，合并adb设备和保存的设备
func (w *Window) refreshDevices() {
	w.mu.Lock()
	defer w.mu.Unlock()

	// 获取adb设备
	adbDevices, err := adb.ListDevices()
	if err != nil {
		log.Printf("刷新设备列表失败: %v", err)
	}

	// 构建已知设备集合
	known := make(map[string]bool)
	var newDevices []DeviceItem

	// 先添加adb发现的设备
	for _, d := range adbDevices {
		running := false
		if _, exists := w.instances[d.Serial]; exists {
			running = true
		}
		newDevices = append(newDevices, DeviceItem{Device: d, Running: running})
		known[d.Serial] = true
	}

	// 再添加保存但当前adb未发现的设备
	if w.configManager != nil {
		cfg, err := w.configManager.Load()
		if err == nil {
			for _, addr := range cfg.SavedDevices {
				if known[addr] {
					continue
				}
				running := false
				if _, exists := w.instances[addr]; exists {
					running = true
				}
				newDevices = append(newDevices, DeviceItem{
					Device:  adb.Device{Serial: addr, Model: "Saved", Status: "offline"},
					Running: running,
				})
			}
		}
	}

	w.devices = newDevices
	log.Printf("发现 %d 个设备", len(newDevices))
}

// runWindow 运行窗口事件循环
func (w *Window) runWindow() error {
	var ops op.Ops

	for {
		switch e := w.window.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			w.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// layout 布局主窗口
func (w *Window) layout(gtx layout.Context) layout.Dimensions {
	theme := w.theme

	if w.refreshBtn.Clicked(gtx) {
		w.refreshDevices()
	}

	if w.connectBtn.Clicked(gtx) {
		w.handleConnect()
	}

	// 检查编辑器回车提交
	for {
		ev, ok := w.ipEditor.Update(gtx)
		if !ok {
			break
		}
		if _, isSubmit := ev.(widget.SubmitEvent); isSubmit {
			w.handleConnect()
		}
	}

	// 检查每个设备的启动/停止按钮
	w.mu.Lock()
	for i := range w.devices {
		item := &w.devices[i]
		if item.startBtn.Clicked(gtx) {
			serial := item.Device.Serial
			go w.startScrcpy(serial)
		}
		if item.stopBtn.Clicked(gtx) {
			serial := item.Device.Serial
			go w.stopScrcpy(serial)
		}
	}
	w.mu.Unlock()

	return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{
			Axis: layout.Vertical,
		}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				title := material.H5(theme, "Scrcpy GUI")
				return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, title.Layout)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Button(theme, &w.refreshBtn, "Refresh").Layout(gtx)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return w.layoutIPInput(gtx, theme)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return w.layoutDeviceList(gtx, theme)
			}),
		)
	})
}

// layoutIPInput 布局IP输入行
func (w *Window) layoutIPInput(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	return layout.Flex{
		Axis:    layout.Horizontal,
		Spacing: layout.SpaceEnd,
	}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return widget.Border{
					Color:        color.NRGBA{R: 128, G: 128, B: 128, A: 255},
					Width:        unit.Dp(1),
					CornerRadius: unit.Dp(4),
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					editor := material.Editor(theme, &w.ipEditor, "IP address (e.g. 192.168.1.100:5555)")
					return layout.UniformInset(unit.Dp(8)).Layout(gtx, editor.Layout)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Button(theme, &w.connectBtn, "Connect").Layout(gtx)
		}),
	)
}

// layoutDeviceList 布局设备列表
func (w *Window) layoutDeviceList(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	w.mu.Lock()
	devices := w.devices
	w.mu.Unlock()

	if len(devices) == 0 {
		return material.Body1(theme, "No devices found").Layout(gtx)
	}

	var children []layout.FlexChild
	for i := range devices {
		item := &devices[i]
		serial := item.Device.Serial
		model := item.Device.Model
		running := item.Running

		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{
					Top:    unit.Dp(2),
					Bottom: unit.Dp(2),
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{
						Axis:    layout.Horizontal,
						Spacing: layout.SpaceEnd,
					}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							label := serial + " (" + model + ")"
							return material.Body1(theme, label).Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if running {
								return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return material.Button(theme, &item.stopBtn, "Stop").Layout(gtx)
								})
							}
							return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return material.Button(theme, &item.startBtn, "Start").Layout(gtx)
							})
						}),
					)
				})
			}),
		)
	}

	return layout.Flex{
		Axis:    layout.Vertical,
		Spacing: layout.SpaceEnd,
	}.Layout(gtx, children...)
}

// handleConnect 处理手动连接
func (w *Window) handleConnect() {
	addr := strings.TrimSpace(w.ipEditor.Text())
	if addr == "" {
		return
	}

	// 如果没有端口，加上默认端口
	if !strings.Contains(addr, ":") {
		addr = addr + ":5555"
	}

	go func() {
		if err := adb.ConnectDevice(addr); err != nil {
			log.Printf("连接设备失败: %v", err)
			return
		}
		log.Printf("已连接设备: %s", addr)

		// 保存到配置
		if w.configManager != nil {
			if err := w.configManager.AddSavedDevice(addr); err != nil {
				log.Printf("保存设备地址失败: %v", err)
			}
		}

		// 清空输入框
		w.ipEditor.SetText("")
		w.refreshDevices()
	}()
}

// startScrcpy 启动scrcpy实例
func (w *Window) startScrcpy(serial string) {
	w.mu.Lock()
	if _, exists := w.instances[serial]; exists {
		w.mu.Unlock()
		log.Printf("设备 %s 已有scrcpy实例运行", serial)
		return
	}
	w.mu.Unlock()

	cfg := config.DefaultConfig()
	if w.configManager != nil {
		var err error
		cfg, err = w.configManager.Load()
		if err != nil {
			log.Printf("加载配置失败: %v", err)
		}
	}

	cfg.WindowTitle = serial

	instance := scrcpy.NewInstance(serial, cfg)
	
	// 设置退出回调，当scrcpy退出时自动停止工具栏
	instance.SetOnExit(func() {
		w.mu.Lock()
		if tb, exists := w.toolbars[serial]; exists {
			tb.Stop()
			delete(w.toolbars, serial)
		}
		// 更新设备运行状态
		for i := range w.devices {
			if w.devices[i].Device.Serial == serial {
				w.devices[i].Running = false
				break
			}
		}
		delete(w.instances, serial)
		w.mu.Unlock()
		log.Printf("scrcpy退出，已清理工具栏: 设备 %s", serial)
	})
	
	if err := instance.Start(); err != nil {
		log.Printf("启动scrcpy失败: %v", err)
		return
	}

	w.mu.Lock()
	w.instances[serial] = instance
	// 更新设备运行状态
	for i := range w.devices {
		if w.devices[i].Device.Serial == serial {
			w.devices[i].Running = true
			break
		}
	}
	w.mu.Unlock()

	tb := toolbar.New(instance)
	if err := tb.Run(); err != nil {
		log.Printf("启动工具栏失败: %v", err)
	} else {
		w.mu.Lock()
		w.toolbars[serial] = tb
		w.mu.Unlock()
	}

	log.Printf("启动scrcpy和工具栏: 设备 %s", serial)
}

// stopScrcpy 停止scrcpy实例
func (w *Window) stopScrcpy(serial string) {
	w.mu.Lock()

	if tb, exists := w.toolbars[serial]; exists {
		tb.Stop()
		delete(w.toolbars, serial)
	}

	if instance, exists := w.instances[serial]; exists {
		if err := instance.Stop(); err != nil {
			log.Printf("停止scrcpy失败: %v", err)
		}
		delete(w.instances, serial)
	}

	// 更新设备运行状态
	for i := range w.devices {
		if w.devices[i].Device.Serial == serial {
			w.devices[i].Running = false
			break
		}
	}

	w.mu.Unlock()

	log.Printf("停止scrcpy和工具栏: 设备 %s", serial)
}
