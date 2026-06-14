# Scrcpy GUI

English | [中文](README_CN.md)

A Gio UI-based graphical interface for scrcpy, providing device management and floating toolbar control.

![Scrcpy GUI Screenshot](docs/pic.png)

## Prerequisites

On first launch the app automatically detects whether **adb** and **scrcpy** are available. If either is missing, a setup dialog prompts you to download them automatically from GitHub Releases.

- **Windows** – scrcpy releases bundle `adb.exe`, so clicking **Download scrcpy** usually installs both tools at once.
- **macOS / Linux** – scrcpy releases do **not** include adb. The app will download scrcpy automatically, but you still need to install adb separately (e.g. `brew install android-platform-tools` or `apt install android-tools-adb`).

### Manual installation (optional)

If you prefer to manage the binaries yourself, install them manually and the app will detect them via `PATH` or a custom path you configure:

**ADB:** https://developer.android.com/tools/releases/platform-tools  
**scrcpy:** https://github.com/Genymobile/scrcpy/releases

## Usage

### Run the Program

```bash
# Run the compiled executable directly
./scrcpy-gui.exe

# Or build and run from source
go build -ldflags "-H windowsgui" -o scrcpy-gui.exe ./cmd/scrcpy-gui/ 2>&1
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
git clone https://github.com/zaaack/scrcpy-gui.git
cd scrcpy-gui

# Install dependencies
go mod tidy

# Build
go build -ldflags "-H windowsgui" -o scrcpy-gui.exe ./cmd/scrcpy-gui/ 2>&1
```

## Troubleshooting

### "adb / scrcpy were not detected" on first launch
Click **Download scrcpy** in the setup dialog to fetch the latest release automatically.
- On Windows this usually installs both scrcpy and adb.
- On macOS / Linux you still need to install adb separately because scrcpy releases do not bundle it.

If automatic download fails (e.g. network issues), follow the **Manual installation** steps in [Prerequisites](#prerequisites).

### Device not showing
1. Make sure USB debugging is enabled on the device
2. Run `adb devices` to confirm the device is connected
3. Click the "Refresh" button to refresh the device list

## License

MIT License
