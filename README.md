# Scrcpy GUI

English | [中文](README_CN.md)

A Gio UI-based graphical interface for scrcpy, providing device management and floating toolbar control.

## Prerequisites

Before using this tool, you need to manually install the following dependencies and add them to the system PATH:

### 1. Install ADB (Android Debug Bridge)

ADB is a command-line tool for communicating with Android devices.

**Download:** https://developer.android.com/tools/releases/platform-tools

**Installation steps:**
1. Download Platform Tools for your operating system
2. Extract to any directory (e.g., `C:\platform-tools`)
3. Add the directory to the system PATH environment variable

**Verify installation:**
```bash
adb version
```

### 2. Install scrcpy

scrcpy is a screen mirroring and control tool for Android.

**Download:** https://github.com/Genymobile/scrcpy/releases

**Installation steps:**
1. Download the latest version from GitHub Releases
2. Extract to any directory (e.g., `C:\scrcpy`)
3. Add the directory to the system PATH environment variable

**Verify installation:**
```bash
scrcpy --version
```

### 3. Configure Environment Variables

**Windows:**
1. Right-click "This PC" -> "Properties" -> "Advanced system settings"
2. Click "Environment Variables"
3. Find `Path` in "System variables" and click "Edit"
4. Add the installation paths for ADB and scrcpy
5. Click "OK" to save

**Verify configuration:**
Open a new command prompt and run the following commands to confirm global access:
```bash
adb devices
scrcpy --version
```

## Usage

### Run the Program

```bash
# Run the compiled executable directly
./scrcpy-gui.exe

# Or build and run from source
go build -o scrcpy-gui.exe ./cmd/scrcpy-gui/
./scrcpy-gui.exe
```

### Features

1. **Device List**: Automatically displays connected ADB devices
2. **Manual Connect**: Connect remote devices via IP address
3. **Start/Stop**: Start or stop scrcpy mirroring for each device
4. **Floating Toolbar**: A control toolbar automatically appears when scrcpy starts, providing:
   - Back: Back button
   - Home: Home button
   - Recent: Recent apps
   - Vol+/Vol-: Volume control
   - Power: Power button
   - Rotate: Rotate screen
   - Full: Toggle fullscreen

### Behavior

- Closing the scrcpy window automatically closes the corresponding floating toolbar
- The toolbar automatically follows the scrcpy window position
- The toolbar hides when scrcpy is minimized

## Build from Source

```bash
# Clone the repository
git clone <repository-url>
cd scrcpy-gui

# Install dependencies
go mod tidy

# Build
go build -o scrcpy-gui.exe ./cmd/scrcpy-gui/
```

## Troubleshooting

### "adb is not recognized as an internal or external command"
ADB is not properly installed or not added to PATH. Please follow the steps above to reinstall and configure environment variables.

### "scrcpy is not recognized as an internal or external command"
scrcpy is not properly installed or not added to PATH. Please follow the steps above to reinstall and configure environment variables.

### Device not showing
1. Make sure USB debugging is enabled on the device
2. Run `adb devices` to confirm the device is connected
3. Click the "Refresh" button to refresh the device list

## License

MIT License
