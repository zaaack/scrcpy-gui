//go:build darwin

package window

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// macOS 文件对话框通过 NSOpenPanel 实现。
//
// 流程：
//   1. [NSOpenPanel openPanel] 取得面板实例
//   2. 设置 title、allowedContentTypes（或旧 API 的 allowedFileTypes）、canChooseFiles 等
//   3. -[NSOpenPanel runModal] 弹出模态对话框，返回 NSModalResponseOK(1) / Cancel(0)
//   4. 取 [panel URLs] 的第一个，[NSURL path] 得到文件路径
//
// filter 参数沿用 Windows 端的管道符格式 "描述|*.ext|..."，这里仅提取扩展名供面板过滤。

var (
	dialogOnce      sync.Once
	selOpenPanel    = objc.RegisterName("openPanel")
	selSetTitle     = objc.RegisterName("setTitle:")
	selSetCanChoose = objc.RegisterName("setCanChooseFiles:")
	selRunModal     = objc.RegisterName("runModal")
	selURLs         = objc.RegisterName("URLs")
	selPath         = objc.RegisterName("path")
	selSetAllowed   = objc.RegisterName("setAllowedFileTypes:")
	selStringWith   = objc.RegisterName("stringWithUTF8String:")
	selArrayWith    = objc.RegisterName("arrayWithObjects:count:")
)

// ensureCocoa 确保 Cocoa 框架已加载，AppKit 类可用。
func ensureCocoa() {
	dialogOnce.Do(func() {
		_, _ = purego.Dlopen("/System/Library/Frameworks/Cocoa.framework/Cocoa", purego.RTLD_GLOBAL|purego.RTLD_LAZY)
	})
}

// OpenFileDialog 打开 macOS 文件选择对话框，返回选中的文件路径。
//
// filter 使用管道符 | 作为分隔符，格式为 "描述|*.ext|另一描述|*.ext"。
// 实际只取其中的扩展名（去 * 和 .）应用到 NSOpenPanel 的 allowedFileTypes。
func OpenFileDialog(title, filter string) (string, error) {
	ensureCocoa()

	nsOpenPanel := objc.GetClass("NSOpenPanel")
	if nsOpenPanel == 0 {
		return "", fmt.Errorf("NSOpenPanel 类不可用")
	}
	panel := objc.ID(nsOpenPanel).Send(selOpenPanel)
	if panel == 0 {
		return "", fmt.Errorf("无法创建 NSOpenPanel")
	}

	// 设置标题
	nsString := objc.GetClass("NSString")
	if nsString != 0 {
		titleObj := objc.ID(nsString).Send(selStringWith, title+"\x00")
		if titleObj != 0 {
			panel.Send(selSetTitle, titleObj)
		}
	}

	// 解析 filter，提取扩展名
	exts := parseFilterExts(filter)
	if len(exts) > 0 {
		// 构造 NSArray*（用 arrayWithObjects:count:），元素是带点的扩展名 NSString，如 "exe"
		NSArray := objc.GetClass("NSArray")
		if NSArray != 0 {
			objs := make([]objc.ID, 0, len(exts))
			for _, ext := range exts {
				if nsString != 0 {
					s := objc.ID(nsString).Send(selStringWith, ext+"\x00")
					if s != 0 {
						objs = append(objs, s)
					}
				}
			}
			if len(objs) > 0 {
				arr := objc.ID(NSArray).Send(selArrayWith, &objs[0], uintptr(len(objs)))
				if arr != 0 {
					panel.Send(selSetAllowed, arr)
				}
			}
		}
	}

	// 只允许选择文件
	panel.Send(selSetCanChoose, true)

	// runModal 返回 NSModalResponse；1 = OK, 0 = Cancel
	result := panel.Send(selRunModal)
	if result != 1 {
		return "", fmt.Errorf("用户取消选择")
	}

	urls := panel.Send(selURLs) // NSArray*
	if urls == 0 {
		return "", fmt.Errorf("未获取到文件")
	}
	count := urls.Send(objc.RegisterName("count"))
	if count == 0 {
		return "", fmt.Errorf("未选择文件")
	}
	firstURL := urls.Send(objc.RegisterName("objectAtIndex:"), uintptr(0))
	if firstURL == 0 {
		return "", fmt.Errorf("未选择文件")
	}
	// [NSURL path] 返回 NSString；再取 UTF8String 得到 C 字符串
	pathStr := firstURL.Send(selPath)
	if pathStr == 0 {
		return "", fmt.Errorf("无法获取文件路径")
	}
	if msgSendString == nil {
		darwinInit()
	}
	if msgSendString == nil {
		return "", fmt.Errorf("ObjC 运行时不可用")
	}
	path := msgSendString(pathStr, selUTF8String)
	return path, nil
}

// parseFilterExts 从 Windows 风格的 filter 字符串中提取扩展名（不含 *. 和点）。
// 例如 "adb.exe (adb.exe)|adb.exe|Executables (*.exe)|*.exe|All files|*.*"
// 返回 ["adb.exe", "exe"]（保留原始大小写，含点的形式如 "adb.exe" 也会被 split）。
func parseFilterExts(filter string) []string {
	if filter == "" {
		return nil
	}
	parts := strings.Split(filter, "|")
	var exts []string
	seen := make(map[string]bool)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// 描述段（不含 *）跳过
		if !strings.Contains(p, "*") {
			continue
		}
		// 形如 *.exe 或 *.* 或 adb.exe
		p = strings.TrimPrefix(p, "*")
		p = strings.TrimPrefix(p, ".")
		if p == "" || p == "*" || seen[p] {
			continue
		}
		seen[p] = true
		exts = append(exts, p)
	}
	return exts
}
