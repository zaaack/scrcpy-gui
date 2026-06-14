//go:build !windows && !darwin && !linux

package window

import "fmt"

// OpenFileDialog 打开文件选择对话框（不支持平台的存根）。
func OpenFileDialog(title, filter string) (string, error) {
	return "", fmt.Errorf("当前平台不支持文件选择对话框")
}
