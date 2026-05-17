@echo off
echo Building scrcpy-gui...
go generate
if %errorlevel% equ 0 (
    echo Build successful!
) else (
    echo Build failed!
    pause
)
