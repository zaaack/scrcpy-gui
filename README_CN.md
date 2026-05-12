# Scrcpy GUI

[English](README.md) | 中文

一个基于 Gio UI 的 scrcpy 图形界面工具，提供设备管理和悬浮工具栏控制。

## 前置要求

在使用本工具之前，你需要手动安装以下依赖并配置到系统环境变量：

### 1. 安装 ADB (Android Debug Bridge)

ADB 是用于与 Android 设备通信的命令行工具。

**下载地址：** https://developer.android.com/tools/releases/platform-tools

**安装步骤：**
1. 下载对应操作系统的 Platform Tools
2. 解压到任意目录（如 `C:\platform-tools`）
3. 将该目录添加到系统 PATH 环境变量

**验证安装：**
```bash
adb version
```

### 2. 安装 scrcpy

scrcpy 是 Android 屏幕镜像和控制工具。

**下载地址：** https://github.com/Genymobile/scrcpy/releases

**安装步骤：**
1. 从 GitHub Releases 下载最新版本
2. 解压到任意目录（如 `C:\scrcpy`）
3. 将该目录添加到系统 PATH 环境变量

**验证安装：**
```bash
scrcpy --version
```

### 3. 配置环境变量

**Windows 系统：**
1. 右键"此电脑" -> "属性" -> "高级系统设置"
2. 点击"环境变量"
3. 在"系统变量"中找到 `Path`，点击"编辑"
4. 添加 ADB 和 scrcpy 的安装路径
5. 点击"确定"保存

**验证配置：**
打开新的命令行窗口，运行以下命令确认可以全局访问：
```bash
adb devices
scrcpy --version
```

## 使用方法

### 运行程序

```bash
# 直接运行已编译的程序
./scrcpy-gui.exe

# 或者从源码编译运行
go build -o scrcpy-gui.exe ./cmd/scrcpy-gui/
./scrcpy-gui.exe
```

### 功能说明

1. **设备列表**：自动显示已连接的 ADB 设备
2. **手动连接**：通过 IP 地址连接远程设备
3. **启动/停止**：为每个设备启动或停止 scrcpy 镜像
4. **悬浮工具栏**：启动 scrcpy 后会自动显示控制工具栏，提供以下功能：
   - Back：返回键
   - Home：主页键
   - Recent：最近应用
   - Vol+/Vol-：音量调节
   - Power：电源键
   - Rotate：旋转屏幕
   - Full：全屏切换

### 行为特性

- 关闭 scrcpy 窗口时，对应的悬浮工具栏会自动关闭
- 工具栏会自动跟随 scrcpy 窗口位置
- scrcpy 最小化时，工具栏也会隐藏

## 从源码编译

```bash
# 克隆项目
git clone <repository-url>
cd scrcpy-gui

# 安装依赖
go mod tidy

# 编译
go build -o scrcpy-gui.exe ./cmd/scrcpy-gui/
```

## 常见问题

### "adb 不是内部或外部命令"
说明 ADB 未正确安装或未添加到 PATH。请按照上述步骤重新安装并配置环境变量。

### "scrcpy 不是内部或外部命令"
说明 scrcpy 未正确安装或未添加到 PATH。请按照上述步骤重新安装并配置环境变量。

### 设备未显示
1. 确保设备已开启 USB 调试
2. 运行 `adb devices` 确认设备已连接
3. 点击"Refresh"按钮刷新设备列表

## 许可证

MIT License
