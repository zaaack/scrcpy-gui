//go:build !windows

package main

func hideConsoleWindow() {
	// 非Windows平台无需隐藏控制台窗口
}
