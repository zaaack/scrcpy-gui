//go:build windows

package window

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	ofnFileMustExist  = 0x00001000
	ofnNoChangeDir    = 0x00000008
	maxPath           = 260
	maxFile           = 8192
)

type openFileName struct {
	StructSize      uint32
	Owner           uintptr
	Instance        uintptr
	Filter          *uint16
	CustomFilter    *uint16
	MaxCustomFilter uint32
	FilterIndex     uint32
	File            *uint16
	MaxFile         uint32
	FileTitle       *uint16
	MaxFileTitle    uint32
	InitialDir      *uint16
	Title           *uint16
	Flags           uint32
	FileOffset      uint16
	FileExtension   uint16
	DefExt          *uint16
	CustData        uintptr
	FnHook          uintptr
	TemplateName    *uint16
	PvReserved      uintptr
	DwReserved      uint32
	FlagsEx         uint32
}

var (
	comdlg32          = syscall.NewLazyDLL("comdlg32.dll")
	procGetOpenFileName = comdlg32.NewProc("GetOpenFileNameW")
)

// OpenFileDialog 打开Windows文件选择对话框，返回选中的文件路径
func OpenFileDialog(title, filter string) (string, error) {
	titlePtr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return "", fmt.Errorf("转换标题失败: %w", err)
	}
	filterPtr, err := syscall.UTF16PtrFromString(filter)
	if err != nil {
		return "", fmt.Errorf("转换过滤器失败: %w", err)
	}

	fileBuf := make([]uint16, maxFile)

	ofn := openFileName{
		StructSize: uint32(unsafe.Sizeof(openFileName{})),
		Filter:     filterPtr,
		File:       &fileBuf[0],
		MaxFile:    maxFile,
		Title:      titlePtr,
		Flags:      ofnFileMustExist | ofnNoChangeDir,
	}

	ret, _, _ := procGetOpenFileName.Call(uintptr(unsafe.Pointer(&ofn)))
	if ret == 0 {
		return "", fmt.Errorf("用户取消选择")
	}

	return syscall.UTF16ToString(fileBuf), nil
}
