package adb

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

func noWindowCmd(name string, args ...string) *exec.Cmd {
	return newNoWindowCmd(name, args...)
}

// adbCmd 默认从 PATH 查找 adb；可通过 SetAdbCommand 改为自定义路径（如下载到程序目录的 adb.exe）。
var (
	adbCmdMu sync.RWMutex
	adbCmd   = "adb"
)

// SetAdbCommand 设置全局使用的 adb 命令路径。空字符串表示恢复为 PATH 中的 adb。
func SetAdbCommand(path string) {
	adbCmdMu.Lock()
	defer adbCmdMu.Unlock()
	adbCmd = path
	if adbCmd == "" {
		adbCmd = "adb"
	}
}

// adbCommand 返回当前生效的 adb 命令。
func adbCommand() string {
	adbCmdMu.RLock()
	defer adbCmdMu.RUnlock()
	return adbCmd
}

// Device 表示一个ADB设备
type Device struct {
	Serial   string // 设备序列号
	Model    string // 设备型号
	Status   string // 设备状态
	TransportID string // 传输ID
}

// ListDevices 返回已连接的设备列表
func ListDevices() ([]Device, error) {
	cmd := noWindowCmd(adbCommand(), "devices", "-l")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("执行adb devices失败: %w", err)
	}

	return parseDevicesOutput(string(output))
}

// parseDevicesOutput 解析adb devices -l的输出
func parseDevicesOutput(output string) ([]Device, error) {
	lines := strings.Split(output, "\n")
	var devices []Device

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of devices") || strings.HasPrefix(line, "*") {
			continue
		}

		// 解析设备行
		// 格式: <serial> <status> <properties>
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		device := Device{
			Serial: parts[0],
			Status: parts[1],
		}

		// 解析属性
		for _, part := range parts[2:] {
			if strings.HasPrefix(part, "model:") {
				device.Model = strings.TrimPrefix(part, "model:")
			} else if strings.HasPrefix(part, "transport_id:") {
				device.TransportID = strings.TrimPrefix(part, "transport_id:")
			}
		}

		// 只添加已授权的设备
		if device.Status == "device" {
			devices = append(devices, device)
		}
	}

	return devices, nil
}

// GetDeviceModel 获取设备型号（如果未在属性中找到）
func GetDeviceModel(serial string) (string, error) {
	cmd := noWindowCmd(adbCommand(), "-s", serial, "shell", "getprop", "ro.product.model")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("获取设备型号失败: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// ConnectDevice 连接到指定IP的设备
func ConnectDevice(addr string) error {
	cmd := noWindowCmd(adbCommand(), "connect", addr)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("连接设备失败: %s, %w", strings.TrimSpace(string(output)), err)
	}
	out := strings.TrimSpace(string(output))
	if !strings.HasPrefix(out, "connected") && !strings.HasPrefix(out, "already connected") {
		return fmt.Errorf("连接设备失败: %s", out)
	}
	return nil
}

// DisconnectDevice 断开指定IP的设备
func DisconnectDevice(addr string) error {
	cmd := noWindowCmd(adbCommand(), "disconnect", addr)
	_, err := cmd.CombinedOutput()
	return err
}

// InstallAPK 安装APK到指定设备，通过onProgress回调输出进度
func InstallAPK(serial, apkPath string, onProgress func(string)) error {
	onProgress("正在安装...")
	cmd := noWindowCmd(adbCommand(), "-s", serial, "install", "-r", apkPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		onProgress("安装失败: " + string(output))
		return fmt.Errorf("安装APK失败: %s, %w", strings.TrimSpace(string(output)), err)
	}
	out := strings.TrimSpace(string(output))
	if strings.Contains(out, "Success") {
		onProgress("安装成功")
		return nil
	}
	onProgress("安装失败: " + out)
	return fmt.Errorf("安装APK失败: %s", out)
}