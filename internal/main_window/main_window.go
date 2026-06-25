package main_window

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"path/filepath"
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
	"gioui.org/x/explorer"

	"scrcpy-gui/internal/adb"
	"scrcpy-gui/internal/config"
	"scrcpy-gui/internal/scrcpy"
	"scrcpy-gui/internal/toolbar"
	"scrcpy-gui/internal/tools"
	"scrcpy-gui/internal/window"
)

// DeviceItem 表示设备列表中的一项
type DeviceItem struct {
	Device     adb.Device
	Running    bool
	startBtn   widget.Clickable
	stopBtn    widget.Clickable
	menuBtn    widget.Clickable
	installBtn widget.Clickable
	argsBtn    widget.Clickable
	resBtn     widget.Clickable
}

// setupDialogState 驱动 adb/scrcpy 缺失时弹出的设置对话框。
//
// 数据字段（snapshot）受 setupMu 保护，因为下载 goroutine 会通过进度回调写入它们，
// 而窗口事件循环 goroutine 在布局时读取。widget.Clickable 按钮只在 UI goroutine 中访问。
type setupDialogState struct {
	// —— 受 setupMu 保护的数据 ——
	visible        bool
	adbOK          bool
	scrcpyOK       bool
	adbPath        string
	scrcpyPath     string
	downloadActive bool     // 是否正在下载
	downloadPct    float32  // 下载/解压百分比，<0 表示无法计算
	downloadMsg    string   // 阶段提示
	downloadErr    string   // 错误信息（非空时红色显示）
	cancelDownload chan struct{}

	// —— 仅 UI goroutine 访问 ——
	downloadBtn  widget.Clickable
	chooseAdbBtn widget.Clickable
	chooseScrcpy widget.Clickable
	skipBtn      widget.Clickable
	recheckBtn   widget.Clickable
	cancelBtn    widget.Clickable
}

// setupSnapshot 是 setup 数据字段的只读快照，供布局使用。
type setupSnapshot struct {
	visible        bool
	adbOK          bool
	scrcpyOK       bool
	adbPath        string
	scrcpyPath     string
	downloadActive bool
	downloadPct    float32
	downloadMsg    string
	downloadErr    string
}

// snapshotSetup 在 setupMu 保护下读取 setup 数据字段的快照。
func (w *Window) snapshotSetup() setupSnapshot {
	w.setupMu.Lock()
	defer w.setupMu.Unlock()
	return setupSnapshot{
		visible:        w.setup.visible,
		adbOK:          w.setup.adbOK,
		scrcpyOK:       w.setup.scrcpyOK,
		adbPath:        w.setup.adbPath,
		scrcpyPath:     w.setup.scrcpyPath,
		downloadActive: w.setup.downloadActive,
		downloadPct:    w.setup.downloadPct,
		downloadMsg:    w.setup.downloadMsg,
		downloadErr:    w.setup.downloadErr,
	}
}

// resolutionOption 表示分辨率选项
type resolutionOption struct {
	label  string
	width  int
	height int // width=0, height=0 表示 Original (reset)
}

// settingsDialog 管理设备设置弹窗状态
type settingsDialog struct {
	dialogType   string // "", "args", "resolution"
	dialogSerial string

	// Custom Args 对话框
	argsEd     widget.Editor
	argsSave   widget.Clickable
	argsCancel widget.Clickable

	// Resolution 对话框
	resSelectedW int
	resSelectedH int
	resClicks    map[string]*widget.Clickable
	resOptions   []resolutionOption
	resAddEd     widget.Editor
	resAddBtn    widget.Clickable
	resSave      widget.Clickable
	resCancel    widget.Clickable
}

// Window 主窗口
type Window struct {
	configManager *config.ConfigManager
	devices       []DeviceItem
	instances     map[string]*scrcpy.Instance
	toolbars      map[string]*toolbar.Toolbar
	window        *app.Window
	exp           *explorer.Explorer
	refreshBtn    widget.Clickable
	ipEditor      widget.Editor
	connectBtn    widget.Clickable
	historyBtn    widget.Clickable
	theme         *material.Theme
	mu            sync.Mutex
	setupMu       sync.Mutex // 保护 setupDialogState 字段（跨 goroutine 的下载进度更新）

	installing         map[string]bool
	installMsgs        map[string]string
	menuOpenFor        string

	showHistory      bool
	historyItems     []string
	deleteBtns       map[string]*widget.Clickable
	historyClickables map[string]*widget.Clickable

	// adb/scrcpy 缺失时的设置对话框
	setup setupDialogState

	// 设备设置弹窗
	dialog settingsDialog
}

// New 创建主窗口
func New(configManager *config.ConfigManager) *Window {
	theme := material.NewTheme()
	// 不使用 NoSystemFonts：gofont 仅含拉丁字形，需让 Gio 回退到系统字体（如 SimSun/YaHei）才能显示中文。
	// 这样英文用 gofont，CJK 字形回退到系统字体。
	theme.Shaper = text.NewShaper(text.WithCollection(gofont.Regular()))
	w := &Window{
		configManager: configManager,
		instances:     make(map[string]*scrcpy.Instance),
		toolbars:      make(map[string]*toolbar.Toolbar),
		theme:         theme,
		installing:         make(map[string]bool),
		installMsgs:        make(map[string]string),
		deleteBtns:         make(map[string]*widget.Clickable),
		historyClickables:  make(map[string]*widget.Clickable),
	}

	// 加载配置，初始化 adb/scrcpy 命令路径并做可用性检测
	w.initToolPaths()
	return w
}

// initToolPaths 根据配置设置 adb/scrcpy 命令，并检测可用性；缺失则弹出设置对话框。
func (w *Window) initToolPaths() {
	var cfg config.ScrcpyConfig
	if w.configManager != nil {
		cfg, _ = w.configManager.Load()
	}
	// 应用到包级/实例级配置
	adb.SetAdbCommand(cfg.AdbCommand())

	adbOK, scrcpyOK := tools.CheckAvailable(cfg.AdbPath, cfg.ScrcpyPath)
	w.setupMu.Lock()
	w.setup.adbOK = adbOK
	w.setup.scrcpyOK = scrcpyOK
	w.setup.adbPath = cfg.AdbPath
	w.setup.scrcpyPath = cfg.ScrcpyPath
	w.setup.visible = !(adbOK && scrcpyOK)
	w.setupMu.Unlock()
}

// Run 运行主窗口
func (w *Window) Run() {
	w.window = new(app.Window)
	w.window.Option(app.Title("Scrcpy GUI - Devices"))
	w.window.Option(app.Size(unit.Dp(400), unit.Dp(400)))
	w.exp = explorer.NewExplorer(w.window)

	w.ipEditor.SingleLine = true
	w.ipEditor.Submit = true

	w.refreshDevices()
	w.loadHistory()

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
		e := w.window.Event()
		w.exp.ListenEvents(e)
		switch e := e.(type) {
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

	// 若 adb/scrcpy 缺失，优先显示设置对话框
	if w.snapshotSetup().visible {
		return w.layoutSetupDialog(gtx, theme)
	}

	if w.refreshBtn.Clicked(gtx) {
		go func() {
			w.refreshDevices()
			w.loadHistory()
			w.window.Invalidate()
		}()
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

	if w.historyBtn.Clicked(gtx) {
		w.showHistory = !w.showHistory
		if w.showHistory {
			w.loadHistory()
		}
	}

	// 检查每个设备的按钮
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
		if item.menuBtn.Clicked(gtx) {
			serial := item.Device.Serial
			if w.menuOpenFor == serial {
				w.menuOpenFor = ""
			} else {
				w.menuOpenFor = serial
				w.dialog.dialogType = ""
			}
		}
		if item.installBtn.Clicked(gtx) {
			serial := item.Device.Serial
			w.menuOpenFor = ""
			if !w.installing[serial] {
				go w.handleInstallAPK(serial)
			}
		}
		if item.argsBtn.Clicked(gtx) {
			serial := item.Device.Serial
			w.menuOpenFor = ""
			w.openArgsDialog(serial)
		}
		if item.resBtn.Clicked(gtx) {
			serial := item.Device.Serial
			w.menuOpenFor = ""
			w.openResDialog(serial)
		}
	}
	w.mu.Unlock()

	// 检查历史记录删除按钮
	for addr, btn := range w.deleteBtns {
		if btn.Clicked(gtx) {
			w.handleHistoryDelete(addr)
		}
	}

	// 检查历史记录点击
	for addr, btn := range w.historyClickables {
		if btn.Clicked(gtx) {
			w.handleHistorySelect(addr)
		}
	}

	// 检查设置弹窗按钮
	if w.dialog.dialogType != "" {
		if w.dialog.argsSave.Clicked(gtx) {
			w.saveArgs()
		}
		if w.dialog.argsCancel.Clicked(gtx) {
			w.dialog.dialogType = ""
		}
		if w.dialog.resSave.Clicked(gtx) {
			w.saveResolution()
		}
		if w.dialog.resCancel.Clicked(gtx) {
			w.dialog.dialogType = ""
		}
		if w.dialog.resAddBtn.Clicked(gtx) {
			w.addCustomResolution()
		}
		for k, btn := range w.dialog.resClicks {
			if btn.Clicked(gtx) {
				fmt.Sscanf(k, "%dx%d", &w.dialog.resSelectedW, &w.dialog.resSelectedH)
			}
		}
	}

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

// layoutIPInput 布局IP输入行（含历史下拉框）
func (w *Window) layoutIPInput(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	return layout.Flex{
		Axis:    layout.Vertical,
		Spacing: layout.SpaceEnd,
	}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
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
					return layout.Flex{
						Axis: layout.Horizontal,
					}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return material.Button(theme, &w.connectBtn, "Connect").Layout(gtx)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return material.Button(theme, &w.historyBtn, "▼").Layout(gtx)
						}),
					)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return w.layoutHistoryDropdown(gtx, theme)
		}),
	)
}

// layoutHistoryDropdown 布局历史连接下拉框
func (w *Window) layoutHistoryDropdown(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	if !w.showHistory || len(w.historyItems) == 0 {
		return layout.Dimensions{}
	}

	var children []layout.FlexChild
	for _, addr := range w.historyItems {
		a := addr
		delBtn, exists := w.deleteBtns[a]
		if !exists {
			delBtn = &widget.Clickable{}
			w.deleteBtns[a] = delBtn
		}
		rowBtn, exists := w.historyClickables[a]
		if !exists {
			rowBtn = &widget.Clickable{}
			w.historyClickables[a] = rowBtn
		}
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if rowBtn.Clicked(gtx) {
					w.handleHistorySelect(a)
				}
				return layout.Inset{
					Left:   unit.Dp(8),
					Right:  unit.Dp(8),
					Top:    unit.Dp(4),
					Bottom: unit.Dp(4),
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{
						Axis:    layout.Horizontal,
						Spacing: layout.SpaceBetween,
					}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return material.Button(theme, rowBtn, a).Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if delBtn.Clicked(gtx) {
								w.handleHistoryDelete(a)
							}
							return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return material.Button(theme, delBtn, "✕").Layout(gtx)
							})
						}),
					)
				})
			}),
		)
	}

	return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return widget.Border{
			Color:        color.NRGBA{R: 180, G: 180, B: 180, A: 255},
			Width:        unit.Dp(1),
			CornerRadius: unit.Dp(4),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{
					Axis: layout.Vertical,
				}.Layout(gtx, children...)
			})
		})
	})
}

// layoutDeviceList 布局设备列表
func (w *Window) layoutDeviceList(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	w.mu.Lock()
	devices := w.devices
	menuOpenFor := w.menuOpenFor
	installing := w.installing
	installMsgs := w.installMsgs
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
		isOnline := item.Device.Status == "device"
		isInstalling := installing[serial]
		statusMsg := installMsgs[serial]
		menuOpen := menuOpenFor == serial

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
							return layout.Flex{
								Axis: layout.Horizontal,
							}.Layout(gtx,
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
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										btn := material.Button(theme, &item.menuBtn, "More")
										btn.TextSize = unit.Sp(14)
										return btn.Layout(gtx)
									})
								}),
							)
						}),
					)
				})
			}),
		)

		if menuOpen && isOnline {
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(16), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return widget.Border{
							Color:        color.NRGBA{R: 180, G: 180, B: 180, A: 255},
							Width:        unit.Dp(1),
							CornerRadius: unit.Dp(4),
						}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return w.layoutWrap(gtx, theme, []func(gtx layout.Context) layout.Dimensions{
								func(gtx layout.Context) layout.Dimensions {
									btnText := "Install APK"
									if isInstalling {
										btnText = "Installing..."
									}
									return material.Button(theme, &item.installBtn, btnText).Layout(gtx)
								},
								func(gtx layout.Context) layout.Dimensions {
									return material.Button(theme, &item.argsBtn, "Custom Args").Layout(gtx)
								},
								func(gtx layout.Context) layout.Dimensions {
									return material.Button(theme, &item.resBtn, "Resolution").Layout(gtx)
								},
							}, gtx.Dp(8))
						})
						})
					})
				}),
			)
		}

		if w.dialog.dialogType != "" && w.dialog.dialogSerial == serial {
			switch w.dialog.dialogType {
			case "args":
				children = append(children, w.layoutArgsDialog(gtx, theme))
			case "resolution":
				children = append(children, w.layoutResDialog(gtx, theme))
			}
		}

		if statusMsg != "" {
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(16), Top: unit.Dp(1), Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return material.Caption(theme, statusMsg).Layout(gtx)
					})
				}),
			)
		}
	}

	return layout.Flex{
		Axis:    layout.Vertical,
		Spacing: layout.SpaceEnd,
	}.Layout(gtx, children...)
}

// layoutWrap 横向自动换行布局
func (w *Window) layoutWrap(gtx layout.Context, theme *material.Theme, widgets []func(gtx layout.Context) layout.Dimensions, spacing int) layout.Dimensions {
	type item struct {
		fn   func(gtx layout.Context) layout.Dimensions
		size layout.Dimensions
	}
	items := make([]item, len(widgets))

	// 第一遍：测量每个 widget 的宽度
	var totalW int
	for i, fn := range widgets {
		gtx2 := gtx
		gtx2.Constraints = layout.Constraints{Min: image.Point{}, Max: image.Point{X: 1<<30, Y: 1<<30}}
		sz := fn(gtx2)
		items[i] = item{fn: fn, size: sz}
		if i > 0 {
			totalW += spacing
		}
		totalW += sz.Size.X
	}

	// 如果总宽度不超过容器宽度，直接一行排列
	if totalW <= gtx.Constraints.Max.X && len(items) > 0 {
		children := make([]layout.FlexChild, len(items))
		for i := range items {
			idx := i
			children[i] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if idx > 0 {
					return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, items[idx].fn)
				}
				return items[idx].fn(gtx)
			})
		}
		return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd}.Layout(gtx, children...)
	}

	// 第二遍：按行排列
	var lines [][]item
	var currentLine []item
	var currentW int

	for _, it := range items {
		w := it.size.Size.X
		if len(currentLine) > 0 {
			w += spacing
		}
		if currentW+w > gtx.Constraints.Max.X && len(currentLine) > 0 {
			lines = append(lines, currentLine)
			currentLine = []item{it}
			currentW = it.size.Size.X
		} else {
			currentLine = append(currentLine, it)
			currentW += w
		}
	}
	if len(currentLine) > 0 {
		lines = append(lines, currentLine)
	}

	// 布局各行
	var lineWidgets []layout.FlexChild
	for _, line := range lines {
		line := line
		lineWidgets = append(lineWidgets, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			children := make([]layout.FlexChild, len(line))
			for i := range line {
				idx := i
				children[i] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if idx > 0 {
						return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, line[idx].fn)
					}
					return line[idx].fn(gtx)
				})
			}
			return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd}.Layout(gtx, children...)
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx, lineWidgets...)
}

// layoutSetupDialog 布局 adb/scrcpy 缺失时的设置对话框
func (w *Window) layoutSetupDialog(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	snap := w.snapshotSetup()

	// 处理按钮点击（在下载进行中禁用相关按钮）
	if !snap.downloadActive {
		if w.setup.downloadBtn.Clicked(gtx) {
			go w.handleDownload()
		}
		if w.setup.chooseAdbBtn.Clicked(gtx) {
			go w.handleChoosePath("adb")
		}
		if w.setup.chooseScrcpy.Clicked(gtx) {
			go w.handleChoosePath("scrcpy")
		}
		if w.setup.skipBtn.Clicked(gtx) {
			w.setupMu.Lock()
			w.setup.visible = false
			w.setupMu.Unlock()
			w.refreshDevices()
		}
		if w.setup.recheckBtn.Clicked(gtx) {
			go w.handleRecheck()
		}
	}
	if snap.downloadActive {
		if w.setup.cancelBtn.Clicked(gtx) {
			w.handleCancelDownload()
		}
	}

	// 状态指示
	adbStatus := "✓ ready"
	adbColor := color.NRGBA{R: 0, G: 128, B: 0, A: 255}
	if !snap.adbOK {
		adbStatus = "✗ not found"
		adbColor = color.NRGBA{R: 200, G: 0, B: 0, A: 255}
	}
	scrcpyStatus := "✓ ready"
	scrcpyColor := color.NRGBA{R: 0, G: 128, B: 0, A: 255}
	if !snap.scrcpyOK {
		scrcpyStatus = "✗ not found"
		scrcpyColor = color.NRGBA{R: 200, G: 0, B: 0, A: 255}
	}

	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			// 标题
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.H5(theme, "Setup Runtime Dependencies").Layout(gtx)
				})
			}),
			// 说明
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body1(theme, "adb / scrcpy were not detected. You can download scrcpy automatically or choose the executable paths manually.")
					lbl.Color = color.NRGBA{R: 80, G: 80, B: 80, A: 255}
					return lbl.Layout(gtx)
				})
			}),
			// 状态行
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(theme, "adb: "+adbStatus)
					lbl.Color = adbColor
					return lbl.Layout(gtx)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(theme, "scrcpy: "+scrcpyStatus)
					lbl.Color = scrcpyColor
					return lbl.Layout(gtx)
				})
			}),
			// 下载进度 / 错误区
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return w.layoutDownloadArea(gtx, theme)
			}),
			// 按钮区
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								btn := material.Button(theme, &w.setup.downloadBtn, "Download scrcpy")
								if snap.downloadActive {
									btn.Background = color.NRGBA{R: 180, G: 180, B: 180, A: 255}
								}
								return btn.Layout(gtx)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return material.Button(theme, &w.setup.chooseAdbBtn, "Choose adb").Layout(gtx)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return material.Button(theme, &w.setup.chooseScrcpy, "Choose scrcpy").Layout(gtx)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return material.Button(theme, &w.setup.recheckBtn, "Recheck").Layout(gtx)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return material.Button(theme, &w.setup.skipBtn, "Skip").Layout(gtx)
						}),
					)
				})
			}),
		)
	})
}

// layoutDownloadArea 布局下载进度条 / 错误信息
func (w *Window) layoutDownloadArea(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	snap := w.snapshotSetup()
	if snap.downloadActive {
		pct := snap.downloadPct
		return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					msg := snap.downloadMsg
					if msg == "" {
						msg = "Preparing..."
					}
					return material.Body2(theme, msg).Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						// ProgressBar 接收 0~1 的 float32；负值表示无法计算总大小
						v := pct / 100
						if v < 0 {
							v = 0
						}
						if v > 1 {
							v = 1
						}
						return material.ProgressBar(theme, v).Layout(gtx)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							text := fmt.Sprintf("%.0f%%", pct)
							if pct < 0 {
								text = "..."
							}
							return material.Caption(theme, text).Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return material.Button(theme, &w.setup.cancelBtn, "Cancel").Layout(gtx)
						}),
					)
				}),
			)
		})
	}
	if snap.downloadErr != "" {
		return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body2(theme, "Download failed: "+snap.downloadErr)
			lbl.Color = color.NRGBA{R: 200, G: 0, B: 0, A: 255}
			return lbl.Layout(gtx)
		})
	}
	return layout.Dimensions{}
}

// handleDownload 处理下载 scrcpy 的逻辑
func (w *Window) handleDownload() {
	installDir, err := binInstallDir()
	if err != nil {
		w.setupMu.Lock()
		w.setup.downloadErr = fmt.Sprintf("cannot locate program directory: %v", err)
		w.setupMu.Unlock()
		w.window.Invalidate()
		return
	}

	cancelCh := make(chan struct{})
	w.setupMu.Lock()
	w.setup.downloadActive = true
	w.setup.downloadErr = ""
	w.setup.downloadPct = 0
	w.setup.downloadMsg = "Preparing..."
	w.setup.cancelDownload = cancelCh
	w.setupMu.Unlock()
	w.window.Invalidate()

	onProgress := func(p tools.DownloadProgress) {
		select {
		case <-cancelCh:
			return
		default:
		}
		w.setupMu.Lock()
		switch p.Stage {
		case tools.StageQuery:
			w.setup.downloadMsg = "Querying latest release from GitHub..."
		case tools.StageFetch:
			w.setup.downloadMsg = "Locating archive for this platform..."
		case tools.StagePull:
			w.setup.downloadMsg = "Downloading..."
			w.setup.downloadPct = float32(p.Percent)
		case tools.StageUnzip:
			w.setup.downloadMsg = "Extracting..."
			w.setup.downloadPct = float32(p.Percent)
		case tools.StageError:
			w.setup.downloadErr = p.Message
		}
		w.setupMu.Unlock()
		w.window.Invalidate()
	}

	adbRel, scrcpyRel, derr := tools.DownloadScrcpy(installDir, onProgress)

	select {
	case <-cancelCh:
		w.setupMu.Lock()
		w.setup.downloadActive = false
		w.setup.downloadMsg = "Cancelled"
		w.setupMu.Unlock()
		w.window.Invalidate()
		return
	default:
	}

	if derr != nil {
		w.setupMu.Lock()
		w.setup.downloadActive = false
		if w.setup.downloadErr == "" {
			w.setup.downloadErr = derr.Error()
		}
		w.setupMu.Unlock()
		w.window.Invalidate()
		return
	}

	// 解析绝对路径
	scrcpyAbs := ""
	if scrcpyRel != "" {
		scrcpyAbs = filepath.Join(installDir, scrcpyRel)
	}
	adbAbs := ""
	if adbRel != "" {
		adbAbs = filepath.Join(installDir, adbRel)
	}

	adbOK, scrcpyOK := tools.CheckAvailable(adbAbs, scrcpyAbs)

	w.setupMu.Lock()
	w.setup.downloadActive = false
	w.setup.scrcpyPath = scrcpyAbs
	w.setup.adbPath = adbAbs
	w.setup.adbOK = adbOK
	w.setup.scrcpyOK = scrcpyOK
	w.setup.downloadMsg = "Done"
	allOK := adbOK && scrcpyOK
	w.setupMu.Unlock()

	w.persistToolPaths()
	if allOK {
		w.setupMu.Lock()
		w.setup.visible = false
		w.setupMu.Unlock()
		w.refreshDevices()
	}
	w.window.Invalidate()
}

// handleChoosePath 处理手动选择 adb.exe / scrcpy.exe 路径
func (w *Window) handleChoosePath(which string) {
	var path string
	var derr error
	if which == "adb" {
		path, derr = window.OpenFileDialog("Choose adb.exe", "adb.exe (adb.exe)|adb.exe|Executables (*.exe)|*.exe|All files|*.*")
	} else {
		path, derr = window.OpenFileDialog("Choose scrcpy.exe", "scrcpy.exe (scrcpy.exe)|scrcpy.exe|Executables (*.exe)|*.exe|All files|*.*")
	}
	if derr != nil {
		// 用户取消或出错，回到对话框
		w.window.Invalidate()
		return
	}

	w.setupMu.Lock()
	if which == "adb" {
		w.setup.adbPath = path
	} else {
		w.setup.scrcpyPath = path
	}
	adbPath := w.setup.adbPath
	scrcpyPath := w.setup.scrcpyPath
	w.setupMu.Unlock()

	adbOK, scrcpyOK := tools.CheckAvailable(adbPath, scrcpyPath)

	w.setupMu.Lock()
	w.setup.adbOK = adbOK
	w.setup.scrcpyOK = scrcpyOK
	allOK := adbOK && scrcpyOK
	w.setupMu.Unlock()

	w.persistToolPaths()
	if allOK {
		w.setupMu.Lock()
		w.setup.visible = false
		w.setupMu.Unlock()
		w.refreshDevices()
	}
	w.window.Invalidate()
}

// handleRecheck 重新检测 adb/scrcpy 可用性
func (w *Window) handleRecheck() {
	w.setupMu.Lock()
	adbPath := w.setup.adbPath
	scrcpyPath := w.setup.scrcpyPath
	w.setupMu.Unlock()

	// 先把当前路径应用到 adb 包（保证 adb 包使用最新路径）
	adbCommand := "adb"
	if adbPath != "" {
		adbCommand = adbPath
	}
	adb.SetAdbCommand(adbCommand)
	// scrcpy 路径在创建实例时才生效，这里仅刷新状态

	adbOK, scrcpyOK := tools.CheckAvailable(adbPath, scrcpyPath)

	w.setupMu.Lock()
	w.setup.adbOK = adbOK
	w.setup.scrcpyOK = scrcpyOK
	allOK := adbOK && scrcpyOK
	w.setupMu.Unlock()

	if allOK {
		w.setupMu.Lock()
		w.setup.visible = false
		w.setupMu.Unlock()
		w.refreshDevices()
	}
	w.window.Invalidate()
}

// handleCancelDownload 取消正在进行的下载
func (w *Window) handleCancelDownload() {
	w.setupMu.Lock()
	ch := w.setup.cancelDownload
	w.setup.cancelDownload = nil
	w.setupMu.Unlock()
	if ch != nil {
		select {
		case <-ch:
		default:
			close(ch)
		}
	}
}

// persistToolPaths 把当前 setup 中的 adb/scrcpy 路径保存到配置文件
func (w *Window) persistToolPaths() {
	if w.configManager == nil {
		return
	}
	cfg, err := w.configManager.Load()
	if err != nil {
		log.Printf("加载配置失败: %v", err)
		return
	}
	w.setupMu.Lock()
	cfg.AdbPath = w.setup.adbPath
	cfg.ScrcpyPath = w.setup.scrcpyPath
	w.setupMu.Unlock()
	if err := w.configManager.Save(cfg); err != nil {
		log.Printf("保存配置失败: %v", err)
		return
	}
	// 让 adb 包即时生效
	adb.SetAdbCommand(cfg.AdbCommand())
}

// binInstallDir 返回程序目录下的 bin\ 子目录（用于解压 scrcpy）。
func binInstallDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(exePath)
	return filepath.Join(dir, "bin", "scrcpy"), nil
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
	tb := toolbar.New(instance)
	// 让工具栏使用配置中的 adb 命令路径（默认为 "adb"，使用 PATH）
	tb.SetAdbCommand(cfg.AdbCommand())

	// 设置退出回调，当scrcpy退出时自动停止工具栏
	instance.SetOnExit(func() {
		w.mu.Lock()
		if existTb, exists := w.toolbars[serial]; exists {
			existTb.Stop()
			delete(w.toolbars, serial)
		}
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

	// 先注册到 map 再启动，避免 onExit 在注册前触发导致僵尸 toolbar
	w.mu.Lock()
	if _, exists := w.instances[serial]; exists {
		w.mu.Unlock()
		log.Printf("设备 %s 已有scrcpy实例运行", serial)
		return
	}
	w.instances[serial] = instance
	w.toolbars[serial] = tb
	for i := range w.devices {
		if w.devices[i].Device.Serial == serial {
			w.devices[i].Running = true
			break
		}
	}
	w.mu.Unlock()

	if err := instance.Start(); err != nil {
		log.Printf("启动scrcpy失败: %v", err)
		w.mu.Lock()
		delete(w.instances, serial)
		delete(w.toolbars, serial)
		for i := range w.devices {
			if w.devices[i].Device.Serial == serial {
				w.devices[i].Running = false
				break
			}
		}
		w.mu.Unlock()
		return
	}

	if err := tb.Run(); err != nil {
		log.Printf("启动工具栏失败: %v", err)
		w.mu.Lock()
		delete(w.toolbars, serial)
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

// loadHistory 加载历史连接记录
func (w *Window) loadHistory() {
	if w.configManager == nil {
		return
	}
	cfg, err := w.configManager.Load()
	if err != nil {
		log.Printf("加载历史记录失败: %v", err)
		return
	}
	w.historyItems = cfg.SavedDevices
}

// handleHistorySelect 选择历史记录项（填充并自动连接）
func (w *Window) handleHistorySelect(addr string) {
	w.showHistory = false
	w.ipEditor.SetText(addr)
	w.handleConnect()
}

// handleHistoryDelete 删除历史记录项
func (w *Window) handleHistoryDelete(addr string) {
	if w.configManager == nil {
		return
	}
	if err := w.configManager.RemoveSavedDevice(addr); err != nil {
		log.Printf("删除历史记录失败: %v", err)
		return
	}
	delete(w.deleteBtns, addr)
	w.loadHistory()
	w.refreshDevices()
}

// handleInstallAPK 处理安装APK（在goroutine中调用，使用explorer选择文件后异步安装）
func (w *Window) handleInstallAPK(serial string) {
	w.mu.Lock()
	if w.installing[serial] {
		w.mu.Unlock()
		return
	}
	w.installing[serial] = true
	w.installMsgs[serial] = ""
	w.mu.Unlock()

	f, err := w.exp.ChooseFile(".apk")
	if err != nil {
		w.mu.Lock()
		w.installing[serial] = false
		if !errors.Is(err, explorer.ErrUserDecline) {
			w.installMsgs[serial] = fmt.Sprintf("选择文件失败: %v", err)
		}
		w.mu.Unlock()
		return
	}

	var path string
	if of, ok := f.(*os.File); ok {
		path = of.Name()
	}
	f.Close()

	if path == "" {
		w.mu.Lock()
		w.installing[serial] = false
		w.installMsgs[serial] = "无法获取文件路径"
		w.mu.Unlock()
		return
	}

	w.mu.Lock()
	w.installMsgs[serial] = fmt.Sprintf("正在安装: %s", path)
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.installing[serial] = false
		w.mu.Unlock()
	}()

	err = adb.InstallAPK(serial, path, func(msg string) {
		w.mu.Lock()
		w.installMsgs[serial] = msg
		w.mu.Unlock()
	})

	if err != nil {
		log.Printf("安装APK失败: %v", err)
	} else {
		log.Printf("安装APK成功: %s -> %s", serial, path)
	}
}

// openArgsDialog 打开自定义参数弹窗
func (w *Window) openArgsDialog(serial string) {
	cfg := config.DefaultConfig()
	if w.configManager != nil {
		if c, err := w.configManager.Load(); err == nil {
			cfg = c
		}
	}
	w.dialog = settingsDialog{
		dialogType:   "args",
		dialogSerial: serial,
	}
	w.dialog.argsEd.SetText(cfg.ExtraArgs)
}

// openResDialog 打开分辨率弹窗
func (w *Window) openResDialog(serial string) {
	cfg := config.DefaultConfig()
	if w.configManager != nil {
		if c, err := w.configManager.Load(); err == nil {
			cfg = c
		}
	}
	opts := w.getResolutionOptions(cfg)
	clicks := make(map[string]*widget.Clickable)
	for _, o := range opts {
		k := fmt.Sprintf("%dx%d", o.width, o.height)
		clicks[k] = &widget.Clickable{}
	}
	sW, sH := 0, 0
	if cfg.DeviceResolution != "" {
		fmt.Sscanf(cfg.DeviceResolution, "%dx%d", &sW, &sH)
	}
	w.dialog = settingsDialog{
		dialogType:   "resolution",
		dialogSerial: serial,
		resSelectedW: sW,
		resSelectedH: sH,
		resClicks:    clicks,
		resOptions:   opts,
	}
	w.dialog.resAddEd.SingleLine = true
}

// saveArgs 保存自定义参数
func (w *Window) saveArgs() {
	if w.configManager == nil {
		w.dialog.dialogType = ""
		return
	}
	cfg, err := w.configManager.Load()
	if err != nil {
		log.Printf("加载配置失败: %v", err)
		w.dialog.dialogType = ""
		return
	}
	cfg.ExtraArgs = w.dialog.argsEd.Text()
	if err := w.configManager.Save(cfg); err != nil {
		log.Printf("保存配置失败: %v", err)
	}
	w.dialog.dialogType = ""
}

// saveResolution 保存分辨率设置并应用到设备
func (w *Window) saveResolution() {
	if w.configManager == nil {
		w.dialog.dialogType = ""
		return
	}
	cfg, err := w.configManager.Load()
	if err != nil {
		log.Printf("加载配置失败: %v", err)
		w.dialog.dialogType = ""
		return
	}
	w2, h2 := w.dialog.resSelectedW, w.dialog.resSelectedH
	if w2 == 0 && h2 == 0 {
		cfg.DeviceResolution = ""
	} else {
		cfg.DeviceResolution = fmt.Sprintf("%dx%d", w2, h2)
	}
	if err := w.configManager.Save(cfg); err != nil {
		log.Printf("保存配置失败: %v", err)
	}

	// 通过adb设置设备分辨率
	go func() {
		if err := adb.SetResolution(w.dialog.dialogSerial, w2, h2); err != nil {
			log.Printf("设置设备分辨率失败: %v", err)
		} else {
			log.Printf("已设置设备分辨率: %s", cfg.DeviceResolution)
		}
	}()

	w.dialog.dialogType = ""
}

// addCustomResolution 添加自定义分辨率
func (w *Window) addCustomResolution() {
	text := strings.TrimSpace(w.dialog.resAddEd.Text())
	if text == "" {
		return
	}
	var cw, ch int
	if _, err := fmt.Sscanf(text, "%dx%d", &cw, &ch); err != nil || cw <= 0 || ch <= 0 {
		return
	}
	label := fmt.Sprintf("%dx%d", cw, ch)
	w.dialog.resOptions = append(w.dialog.resOptions, resolutionOption{label, cw, ch})
	k := fmt.Sprintf("%dx%d", cw, ch)
	w.dialog.resClicks[k] = &widget.Clickable{}
	w.dialog.resSelectedW = cw
	w.dialog.resSelectedH = ch
	w.dialog.resAddEd.SetText("")

	if w.configManager != nil {
		cfg, err := w.configManager.Load()
		if err == nil {
			exists := false
			for _, c := range cfg.CustomDeviceResolutions {
				if c == label {
					exists = true
					break
				}
			}
			if !exists {
				cfg.CustomDeviceResolutions = append(cfg.CustomDeviceResolutions, label)
				w.configManager.Save(cfg)
			}
		}
	}
}

// getResolutionOptions 获取所有可用的分辨率选项
func (w *Window) getResolutionOptions(cfg config.ScrcpyConfig) []resolutionOption {
	opts := []resolutionOption{
		{"Original (reset)", 0, 0},
		{"1080p (1080x1920)", 1080, 1920},
		{"720p (720x1280)", 720, 1280},
		{"480p (480x854)", 480, 854},
	}
	for _, custom := range cfg.CustomDeviceResolutions {
		var cw, ch int
		if _, err := fmt.Sscanf(custom, "%dx%d", &cw, &ch); err != nil || cw <= 0 || ch <= 0 {
			continue
		}
		skip := false
		for _, o := range opts {
			if o.width == cw && o.height == ch {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		opts = append(opts, resolutionOption{custom, cw, ch})
	}
	return opts
}

// layoutArgsDialog 布局自定义参数弹窗
func (w *Window) layoutArgsDialog(gtx layout.Context, theme *material.Theme) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(16), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return widget.Border{
				Color:        color.NRGBA{R: 180, G: 180, B: 180, A: 255},
				Width:        unit.Dp(1),
				CornerRadius: unit.Dp(4),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return material.Body2(theme, "Extra arguments for scrcpy:").Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							ed := material.Editor(theme, &w.dialog.argsEd, "e.g. --no-audio --rotation 1")
							return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, ed.Layout)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return material.Button(theme, &w.dialog.argsCancel, "Cancel").Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return material.Button(theme, &w.dialog.argsSave, "Save").Layout(gtx)
								}),
							)
						}),
					)
				})
			})
		})
	})
}

// layoutResDialog 布局分辨率弹窗
func (w *Window) layoutResDialog(gtx layout.Context, theme *material.Theme) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(16), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return widget.Border{
				Color:        color.NRGBA{R: 180, G: 180, B: 180, A: 255},
				Width:        unit.Dp(1),
				CornerRadius: unit.Dp(4),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					children := []layout.FlexChild{
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return material.Body2(theme, "Resolution:").Layout(gtx)
						}),
					}

					for _, opt := range w.dialog.resOptions {
						o := opt
						k := fmt.Sprintf("%dx%d", o.width, o.height)
						btn := material.Button(theme, w.dialog.resClicks[k], o.label)
						if w.dialog.resSelectedW == o.width && w.dialog.resSelectedH == o.height {
							btn.Background = color.NRGBA{R: 0, G: 120, B: 215, A: 255}
						}
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return btn.Layout(gtx)
							})
						}))
					}

					children = append(children,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										ed := material.Editor(theme, &w.dialog.resAddEd, "Custom (e.g. 960x1920)")
										return ed.Layout(gtx)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return material.Button(theme, &w.dialog.resAddBtn, "Add").Layout(gtx)
									}),
								)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return material.Button(theme, &w.dialog.resCancel, "Cancel").Layout(gtx)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return material.Button(theme, &w.dialog.resSave, "Apply").Layout(gtx)
									}),
								)
							})
						}),
					)

					return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx, children...)
				})
			})
		})
	})
}
