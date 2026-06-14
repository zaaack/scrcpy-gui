package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ScrcpyConfig 表示scrcpy的配置参数
type ScrcpyConfig struct {
	// 命令路径（空表示使用PATH中的adb/scrcpy）
	AdbPath    string `json:"adb_path"`    // 自定义adb可执行文件路径
	ScrcpyPath string `json:"scrcpy_path"` // 自定义scrcpy可执行文件路径

	// 基本参数
	MaxSize   int    `json:"max_size"`   // 最大尺寸（0表示默认）
	BitRate   int    `json:"bit_rate"`   // 比特率（Mbps）
	MaxFPS    int    `json:"max_fps"`    // 最大帧率
	LockVideoOrientation int `json:"lock_video_orientation"` // 锁定视频方向
	
	// 窗口参数
	WindowX      int    `json:"window_x"`      // 窗口X位置
	WindowY      int    `json:"window_y"`      // 窗口Y位置
	WindowWidth  int    `json:"window_width"`  // 窗口宽度
	WindowHeight int    `json:"window_height"` // 窗口高度
	WindowTitle  string `json:"window_title"`  // 窗口标题
	Fullscreen   bool   `json:"fullscreen"`    // 是否全屏
	AlwaysOnTop  bool   `json:"always_on_top"` // 是否置顶
	
	// 控制参数
	Control         bool `json:"control"`          // 是否启用控制
	ShowTouches     bool `json:"show_touches"`     // 是否显示触摸
	StayAwake       bool `json:"stay_awake"`       // 是否保持唤醒
	TurnScreenOff   bool `json:"turn_screen_off"`  // 是否关闭屏幕
	
	// 保存的手动连接设备
	SavedDevices []string `json:"saved_devices"` // 保存的设备地址列表
}

// DefaultConfig 返回默认配置
func DefaultConfig() ScrcpyConfig {
	return ScrcpyConfig{
		MaxSize:   0,
		BitRate:   8,
		MaxFPS:    0,
		LockVideoOrientation: -1,
		WindowX:      -1,
		WindowY:      -1,
		WindowWidth:  0,
		WindowHeight: 0,
		WindowTitle:  "",
		Fullscreen:   false,
		AlwaysOnTop:  false,
		Control:         true,
		ShowTouches:     false,
		StayAwake:       false,
		TurnScreenOff:   false,
	}
}

// AdbCommand 返回应使用的adb命令（自定义路径或默认"adb"）
func (c ScrcpyConfig) AdbCommand() string {
	if c.AdbPath != "" {
		return c.AdbPath
	}
	return "adb"
}

// ScrcpyCommand 返回应使用的scrcpy命令（自定义路径或默认"scrcpy"）
func (c ScrcpyConfig) ScrcpyCommand() string {
	if c.ScrcpyPath != "" {
		return c.ScrcpyPath
	}
	return "scrcpy"
}

// ConfigManager 管理配置的保存和加载
type ConfigManager struct {
	configPath string
}

// NewConfigManager 创建配置管理器
func NewConfigManager() (*ConfigManager, error) {
	// 获取用户配置目录
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("获取用户配置目录失败: %w", err)
	}
	
	// 创建应用配置目录
	appDir := filepath.Join(configDir, "scrcpy-gui")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return nil, fmt.Errorf("创建配置目录失败: %w", err)
	}
	
	return &ConfigManager{
		configPath: filepath.Join(appDir, "config.json"),
	}, nil
}

// Load 加载配置
func (cm *ConfigManager) Load() (ScrcpyConfig, error) {
	config := DefaultConfig()
	
	// 检查配置文件是否存在
	if _, err := os.Stat(cm.configPath); os.IsNotExist(err) {
		return config, nil
	}
	
	// 读取配置文件
	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		return config, fmt.Errorf("读取配置文件失败: %w", err)
	}
	
	// 解析JSON
	if err := json.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("解析配置文件失败: %w", err)
	}
	
	return config, nil
}

// Save 保存配置
func (cm *ConfigManager) Save(config ScrcpyConfig) error {
	// 序列化为JSON
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	
	// 写入文件
	if err := os.WriteFile(cm.configPath, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	
	return nil
}

// AddSavedDevice 添加保存的设备地址（去重，最新在前，最多10个）
func (cm *ConfigManager) AddSavedDevice(addr string) error {
	cfg, err := cm.Load()
	if err != nil {
		return err
	}
	var result []string
	result = append(result, addr)
	for _, d := range cfg.SavedDevices {
		if d != addr {
			result = append(result, d)
		}
	}
	if len(result) > 10 {
		result = result[:10]
	}
	cfg.SavedDevices = result
	return cm.Save(cfg)
}

// RemoveSavedDevice 移除保存的设备地址
func (cm *ConfigManager) RemoveSavedDevice(addr string) error {
	cfg, err := cm.Load()
	if err != nil {
		return err
	}
	var result []string
	for _, d := range cfg.SavedDevices {
		if d != addr {
			result = append(result, d)
		}
	}
	cfg.SavedDevices = result
	return cm.Save(cfg)
}

// BuildArgs 构建scrcpy命令行参数
func (config ScrcpyConfig) BuildArgs(serial string) []string {
	args := []string{"-s", serial}
	
	// 基本参数
	if config.MaxSize > 0 {
		args = append(args, "--max-size", fmt.Sprintf("%d", config.MaxSize))
	}
	if config.BitRate > 0 {
		args = append(args, "--video-bit-rate", fmt.Sprintf("%dM", config.BitRate))
	}
	if config.MaxFPS > 0 {
		args = append(args, "--max-fps", fmt.Sprintf("%d", config.MaxFPS))
	}
	if config.LockVideoOrientation >= 0 {
		args = append(args, "--capture-orientation", fmt.Sprintf("%d", config.LockVideoOrientation))
	}
	
	// 窗口参数
	if config.WindowX >= 0 {
		args = append(args, "--window-x", fmt.Sprintf("%d", config.WindowX))
	}
	if config.WindowY >= 0 {
		args = append(args, "--window-y", fmt.Sprintf("%d", config.WindowY))
	}
	if config.WindowWidth > 0 {
		args = append(args, "--window-width", fmt.Sprintf("%d", config.WindowWidth))
	}
	if config.WindowHeight > 0 {
		args = append(args, "--window-height", fmt.Sprintf("%d", config.WindowHeight))
	}
	if config.WindowTitle != "" {
		args = append(args, "--window-title", config.WindowTitle)
	}
	if config.Fullscreen {
		args = append(args, "--fullscreen")
	}
	if config.AlwaysOnTop {
		args = append(args, "--always-on-top")
	}
	
	// 控制参数
	if !config.Control {
		args = append(args, "--no-control")
	}
	if config.ShowTouches {
		args = append(args, "--show-touches")
	}
	if config.StayAwake {
		args = append(args, "--stay-awake")
	}
	if config.TurnScreenOff {
		args = append(args, "--turn-screen-off")
	}
	
	return args
}