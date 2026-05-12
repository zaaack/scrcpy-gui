package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/debug"
	"syscall"

	"gioui.org/app"

	"scrcpy-gui/internal/config"
	"scrcpy-gui/internal/main_window"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	getConsoleWindow = kernel32.NewProc("GetConsoleWindow")
	showWindow       = syscall.NewLazyDLL("user32.dll").NewProc("ShowWindow")
)

func hideConsoleWindow() {
	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd != 0 {
		showWindow.Call(hwnd, 0) // SW_HIDE = 0
	}
}

func main() {
	hideConsoleWindow()

	// 强制软件渲染，避免GPU驱动内存映射（RSS会下降，CPU占用上升）
//	os.Setenv("GIO_RENDERER", "software")

	// 降低GC目标，减少内存占用（默认100，GUI应用数据量小可以更积极回收）
	debug.SetGCPercent(20)

	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	configManager, err := config.NewConfigManager()
	if err != nil {
		log.Printf("创建配置管理器失败: %v", err)
	}

	window := main_window.New(configManager)
	window.Run()

	// 窗口初始化后强制回收一次，释放启动阶段的临时内存
	runtime.GC()
	debug.FreeOSMemory()

	// 启动 pprof，访问 http://localhost:6060/debug/pprof/heap 查看堆内存
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	app.Main()
}
