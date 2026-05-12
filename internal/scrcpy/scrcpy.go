package scrcpy

import (
	"fmt"
	"log"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"scrcpy-gui/internal/config"
	"scrcpy-gui/internal/window"
)

// Instance 表示一个scrcpy实例
type Instance struct {
	serial    string
	config    config.ScrcpyConfig
	cmd       *exec.Cmd
	tracker   window.Tracker
	windowHandle uintptr
	running   bool
	mu        sync.Mutex
	stopCh    chan struct{}
	onExit    func()
}

// NewInstance 创建新的scrcpy实例
func NewInstance(serial string, cfg config.ScrcpyConfig) *Instance {
	return &Instance{
		serial:  serial,
		config:  cfg,
		tracker: window.NewTracker(),
		stopCh:  make(chan struct{}),
	}
}

// Start 启动scrcpy
func (inst *Instance) Start() error {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	
	if inst.running {
		return fmt.Errorf("scrcpy实例已在运行")
	}
	
	// 构建命令行参数
	args := inst.config.BuildArgs(inst.serial)
	
	// 创建命令
	inst.cmd = exec.Command("scrcpy", args...)
	inst.cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	
	// 启动进程
	if err := inst.cmd.Start(); err != nil {
		return fmt.Errorf("启动scrcpy失败: %w", err)
	}
	
	inst.running = true
	
	// 启动监控goroutine
	go inst.monitor()
	
	// 等待窗口出现
	go inst.waitForWindow()
	
	log.Printf("启动scrcpy: 设备 %s, PID %d", inst.serial, inst.cmd.Process.Pid)
	
	return nil
}

// Stop 停止scrcpy
func (inst *Instance) Stop() error {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	
	if !inst.running {
		return nil
	}
	
	// 发送停止信号
	close(inst.stopCh)
	
	// 终止进程
	if inst.cmd != nil && inst.cmd.Process != nil {
		if err := inst.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("终止scrcpy进程失败: %w", err)
		}
	}
	
	inst.running = false
	log.Printf("停止scrcpy: 设备 %s", inst.serial)
	
	return nil
}

// IsRunning 检查是否正在运行
func (inst *Instance) IsRunning() bool {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return inst.running
}

// GetWindowHandle 获取窗口句柄
func (inst *Instance) GetWindowHandle() uintptr {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return inst.windowHandle
}

// GetSerial 获取设备序列号
func (inst *Instance) GetSerial() string {
	return inst.serial
}

// SetOnExit 设置进程退出时的回调函数
func (inst *Instance) SetOnExit(callback func()) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	inst.onExit = callback
}

// monitor 监控scrcpy进程状态
func (inst *Instance) monitor() {
	if inst.cmd == nil || inst.cmd.Process == nil {
		return
	}
	
	// 等待进程结束
	err := inst.cmd.Wait()
	
	inst.mu.Lock()
	inst.running = false
	onExit := inst.onExit
	inst.mu.Unlock()
	
	if err != nil {
		log.Printf("scrcpy进程异常退出: 设备 %s, 错误: %v", inst.serial, err)
	} else {
		log.Printf("scrcpy进程正常退出: 设备 %s", inst.serial)
	}
	
	// 调用退出回调
	if onExit != nil {
		onExit()
	}
}

// waitForWindow 等待scrcpy窗口出现
func (inst *Instance) waitForWindow() {
	// 等待一段时间让窗口创建
	time.Sleep(2 * time.Second)
	
	// 如果没有设置窗口标题，使用默认标题格式
	windowTitle := inst.config.WindowTitle
	if windowTitle == "" {
		windowTitle = inst.serial
	}
	
	// 尝试查找窗口
	maxAttempts := 30
	for i := 0; i < maxAttempts; i++ {
		select {
		case <-inst.stopCh:
			return
		default:
		}
		
		handle, err := inst.tracker.FindWindow(windowTitle)
		if err == nil {
			inst.mu.Lock()
			inst.windowHandle = handle
			inst.mu.Unlock()
			
			log.Printf("找到scrcpy窗口: 设备 %s, 句柄 %d", inst.serial, handle)
			return
		}
		
		time.Sleep(500 * time.Millisecond)
	}
	
	log.Printf("警告: 未找到scrcpy窗口: 设备 %s", inst.serial)
}

// GetWindowRect 获取窗口位置和大小
func (inst *Instance) GetWindowRect() (x, y, width, height int, err error) {
	inst.mu.Lock()
	handle := inst.windowHandle
	inst.mu.Unlock()
	
	if handle == 0 {
		return 0, 0, 0, 0, fmt.Errorf("窗口句柄无效")
	}
	
	return inst.tracker.GetWindowRect(handle)
}

// Rotate 发送旋转快捷键
func (inst *Instance) Rotate() error {
	inst.mu.Lock()
	handle := inst.windowHandle
	inst.mu.Unlock()

	if handle == 0 {
		return fmt.Errorf("窗口句柄无效")
	}

	return inst.tracker.SendRotateShortcut(handle)
}

// ToggleFullscreen 发送全屏快捷键
func (inst *Instance) ToggleFullscreen() error {
	inst.mu.Lock()
	handle := inst.windowHandle
	inst.mu.Unlock()

	if handle == 0 {
		return fmt.Errorf("窗口句柄无效")
	}

	return inst.tracker.SendFullscreenShortcut(handle)
}