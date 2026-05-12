# AGENTS.md

## What this is

A Windows-only Go GUI wrapper around **scrcpy** (Android screen mirroring). Uses the **Gio UI** framework (`gioui.org`) and Windows-specific APIs via `github.com/lxn/win`. The floating toolbar communicates with scrcpy windows through Win32 `PostMessage` and with devices via `adb` shell commands.

## Build & run

```bash
go build -ldflags "-H windowsgui" -o scrcpy-gui.exe ./cmd/scrcpy-gui/
```

- `-H windowsgui` is required — it hides the console window. Without it the app spawns a visible console.
- No test suite, linter, or CI is configured in this repo.

## Runtime prerequisites

These must be on `PATH` at runtime (not build time):

- `adb` — Android Debug Bridge
- `scrcpy` — the actual screen mirroring binary

## Architecture

```
cmd/scrcpy-gui/main.go    → entrypoint, console hide, config + window bootstrap
internal/
  config/                  → ScrcpyConfig struct, JSON load/save to UserConfigDir/scrcpy-gui/config.json
  adb/                     → wraps `adb devices -l`, parses output
  scrcpy/                  → manages scrcpy child processes (Start/Stop), finds their Win32 windows
  window/                  → Tracker interface + platform impls (WindowsTracker via lxn/win, stub for others)
  toolbar/                 → floating Gio toolbar that tracks scrcpy window position, sends adb keyevents
  main_window/             → main Gio window: device list, start/stop scrcpy per device
```

## Key patterns

- **Build tags**: `window_windows.go` (`//go:build windows`) has the real implementation; `window_other.go` (`//go:build !windows`) is all stubs returning errors. This app only works on Windows.
- **Window tracking**: The toolbar polls scrcpy window position every 100ms and repositions itself to the right edge using Win32 `SetWindowPos`.
- **Concurrency**: `scrcpy.Instance` and `toolbar.Toolbar` use `sync.Mutex` for thread safety. GUI event loops run in goroutines.
- **Config path**: `%AppData%/scrcpy-gui/config.json` (via `os.UserConfigDir()`).
- **Log language**: Log messages and comments are in Chinese (中文).

## Gotchas

- Do not attempt to build or run on non-Windows — the `window_other.go` stubs will compile but every window operation will fail at runtime.
- The toolbar finds scrcpy windows by title (the device serial). If two instances use the same serial, window lookup will collide.
- `SendKeyboardEvent` on the Tracker interface is a no-op stub on Windows; keyboard shortcuts go through `SendScrcpyShortcut` which uses `PostMessage` with `WM_KEYDOWN`/`WM_KEYUP`.
