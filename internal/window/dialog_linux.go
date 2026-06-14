//go:build linux

package window

import (
	"fmt"
	"os/exec"
	"strings"
)

// OpenFileDialog 在 Linux 上通过 zenity 弹出文件选择对话框。
//
// 选择 zenity 的原因：
//   - 它是 GNOME/GTK 环境下最常见的原生文件对话框工具，多数发行版预装；
//   - 不依赖 CGo 或特定 GUI 框架，调用简单；
//   - 若系统未安装 zenity，返回错误，调用方可提示用户手动输入路径。
//
// filter 参数沿用 Windows 端的管道符格式 "描述|*.ext|..."，这里提取扩展名
// 转换为 zenity 的 --file-filter 参数。
func OpenFileDialog(title, filter string) (string, error) {
	if _, err := exec.LookPath("zenity"); err != nil {
		return "", fmt.Errorf("未找到 zenity，请安装后重试（如 apt install zenity）或手动填写路径")
	}

	args := []string{"--file-selection", "--title=" + title}
	for _, f := range parseZenityFilters(filter) {
		args = append(args, "--file-filter="+f)
	}

	out, err := exec.Command("zenity", args...).Output()
	if err != nil {
		// zenity 在用户取消时以非零状态退出；这里统一当作取消处理
		return "", fmt.Errorf("用户取消选择")
	}
	path := strings.TrimRight(string(out), "\r\n")
	if path == "" {
		return "", fmt.Errorf("用户取消选择")
	}
	return path, nil
}

// parseZenityFilters 把 Windows 风格的 filter 字符串转成 zenity 的 --file-filter 参数。
// 例如 "Executables (*.exe)|*.exe|All files|*.*"
// 转成 ["Executables | *.exe", "All files | *.*"]
func parseZenityFilters(filter string) []string {
	if filter == "" {
		return nil
	}
	parts := strings.Split(filter, "|")
	// parts 是交替的 "描述" 和 "通配符"
	var result []string
	for i := 0; i+1 < len(parts); i += 2 {
		desc := strings.TrimSpace(parts[i])
		pattern := strings.TrimSpace(parts[i+1])
		if desc == "" || pattern == "" {
			continue
		}
		result = append(result, fmt.Sprintf("%s | %s", desc, pattern))
	}
	return result
}
