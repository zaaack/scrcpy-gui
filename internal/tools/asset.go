package tools

import (
	"fmt"
	"runtime"
	"strings"
)

// assetSpec 描述一个平台对应的 scrcpy 发布资产匹配规则与归档类型。
type assetSpec struct {
	// nameContains 用于在 release 资产名中子串匹配（小写比较）
	nameContains string
	// nameSuffix 资产名后缀（如 ".zip" / ".tar.gz"）
	nameSuffix string
	// archive 归档类型
	archive string // "zip" 或 "targz"
}

// assetSpecFor 返回给定 GOOS/GOARCH 对应的 scrcpy 资产匹配规则。
//
// scrcpy 各平台发布资产命名（以 vX.Y.Z 为例）：
//   - windows/amd64 → scrcpy-win64-vX.Y.Z.zip
//   - windows/386   → scrcpy-win32-vX.Y.Z.zip
//   - linux/amd64   → scrcpy-linux-x86_64-vX.Y.Z.tar.gz
//   - darwin/arm64  → scrcpy-macos-aarch64-vX.Y.Z.tar.gz
//   - darwin/amd64  → scrcpy-macos-x86_64-vX.Y.Z.tar.gz
//
// 不支持的组合返回错误。
func assetSpecFor(goos, goarch string) (assetSpec, error) {
	switch goos {
	case "windows":
		switch goarch {
		case "amd64":
			return assetSpec{nameContains: "win64", nameSuffix: ".zip", archive: "zip"}, nil
		case "386":
			return assetSpec{nameContains: "win32", nameSuffix: ".zip", archive: "zip"}, nil
		}
	case "linux":
		if goarch == "amd64" {
			return assetSpec{nameContains: "linux-x86_64", nameSuffix: ".tar.gz", archive: "targz"}, nil
		}
	case "darwin":
		switch goarch {
		case "arm64":
			return assetSpec{nameContains: "macos-aarch64", nameSuffix: ".tar.gz", archive: "targz"}, nil
		case "amd64":
			return assetSpec{nameContains: "macos-x86_64", nameSuffix: ".tar.gz", archive: "targz"}, nil
		}
	}
	return assetSpec{}, fmt.Errorf("不支持的平台 %s/%s：请通过包管理器安装 scrcpy 后手动指定路径", goos, goarch)
}

// findAsset 在 release 资产中按平台规则匹配。
func findAsset(rel *githubRelease, goos, goarch string) (githubAsset, assetSpec, error) {
	spec, err := assetSpecFor(goos, goarch)
	if err != nil {
		return githubAsset{}, spec, err
	}
	for _, a := range rel.Assets {
		name := strings.ToLower(a.Name)
		if strings.Contains(name, spec.nameContains) && strings.HasSuffix(name, spec.nameSuffix) {
			return a, spec, nil
		}
	}
	return githubAsset{}, spec, fmt.Errorf("未找到 %s/%s 对应的 scrcpy 压缩包资产", goos, goarch)
}

// findWin64Asset 兼容旧调用：匹配 windows/amd64 资产。
func findWin64Asset(rel *githubRelease) (githubAsset, error) {
	a, _, err := findAsset(rel, "windows", "amd64")
	return a, err
}

// currentPlatformAssetLabel 返回当前平台的可读名称（用于进度提示）。
func currentPlatformAssetLabel() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}
