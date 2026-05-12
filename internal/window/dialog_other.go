//go:build !windows

package window

import "fmt"

// OpenFileDialog 打开文件选择对话框（其他平台存根）
func OpenFileDialog(title, filter string) (string, error) {
	return "", fmt.Errorf("当前平台不支持文件选择对话框")
}
