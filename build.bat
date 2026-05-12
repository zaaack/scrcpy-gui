@echo off
echo Building scrcpy-gui...
go build -ldflags "-H windowsgui -s -w" -o scrcpy-gui.exe ./cmd/scrcpy-gui/
if %errorlevel% equ 0 (
    echo Build successful!
) else (
    echo Build failed!
    pause
)
