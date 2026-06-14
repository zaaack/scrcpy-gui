// Package tools 负责检测与下载 adb/scrcpy 运行时依赖。
package tools

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// 状态常量，用于 DownloadProgress.Stage
const (
	StageQuery  = "query"  // 正在查询 GitHub 最新版本
	StageFetch  = "fetch"  // 正在解析资产列表
	StagePull   = "pull"   // 正在下载
	StageUnzip  = "unzip"  // 正在解压
	StageDone   = "done"   // 全部完成
	StageError  = "error"  // 出错
	StageCancel = "cancel" // 用户取消
)

// DownloadProgress 下载/解压进度回调
//
//   - Stage=StagePull 时 Percent 为 0~100 的下载百分比
//   - Stage=StageUnzip 时 Percent 为 0~100 的解压百分比（按文件数估算）
//   - Stage=StageDone 时携带 AdbRelPath/ScrcpyRelPath（相对于 installDir）
//   - Stage=StageError 时 Message 携带错误信息
type DownloadProgress struct {
	Stage         string
	Percent       float64
	Message       string
	AdbRelPath    string
	ScrcpyRelPath string
}

// releaseURL 指向 scrcpy 最新版本查询接口
const releaseURL = "https://api.github.com/repos/Genymobile/scrcpy/releases/latest"

// githubAsset 表示 GitHub Release 资产
type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// githubRelease 表示 GitHub Release
type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

// CheckAvailable 综合判断 adb 与 scrcpy 是否可用。
//   - adbPath/scrcpyPath 非空时优先校验它们是否存在且可执行
//   - 否则回退到 PATH 查找（exec.LookPath）
func CheckAvailable(adbPath, scrcpyPath string) (adbOK bool, scrcpyOK bool) {
	adbOK = isExecutableReady(adbPath, "adb")
	scrcpyOK = isExecutableReady(scrcpyPath, "scrcpy")
	return
}

// isExecutableReady 判断某个命令是否就绪：
//   - 给定路径非空：检查文件存在，并尝试运行 --version 验证可执行
//   - 给定路径为空：用 exec.LookPath 在 PATH 中查找
func isExecutableReady(customPath, defaultName string) bool {
	target := customPath
	if target == "" {
		// 仅靠 LookPath 不足以判断（Windows 上可能匹配到无扩展名文件），但配合运行验证更稳。
		if p, err := exec.LookPath(defaultName); err == nil {
			target = p
		} else {
			return false
		}
	} else {
		if _, err := os.Stat(target); err != nil {
			return false
		}
	}

	// 运行 version 验证可执行
	var probeArgs []string
	switch defaultName {
	case "adb":
		probeArgs = []string{"version"}
	case "scrcpy":
		probeArgs = []string{"--version"}
	default:
		probeArgs = []string{"--version"}
	}
	cmd := exec.Command(target, probeArgs...)
	cmd.SysProcAttr = noWindowSysProcAttr()
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

// queryLatestRelease 查询 GitHub 最新版本。
func queryLatestRelease() (*githubRelease, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", releaseURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 GitHub 接口失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub 接口返回状态码 %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var rel githubRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, fmt.Errorf("解析 GitHub 响应失败: %w", err)
	}
	return &rel, nil
}

// downloadToFile 流式下载 url 到 dst，并按下载字节实时回调进度。
// total<=0 时无法计算百分比，回调里会收到 Percent<0。
// 注意：不设置总超时（http.Client.Timeout 涵盖整个响应读取过程，对大文件慢速连接会误判失败），
// 改由调用方通过取消机制终止。
func downloadToFile(url, dst string, total int64, onProgress func(DownloadProgress)) error {
	client := &http.Client{}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("发起下载请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载返回状态码 %d", resp.StatusCode)
	}
	if total <= 0 {
		total = resp.ContentLength
	}

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer out.Close()

	buf := make([]byte, 32*1024)
	var written int64
	last := time.Now()
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return fmt.Errorf("写入临时文件失败: %w", werr)
			}
			written += int64(n)
			// 限频回调，避免刷屏阻塞 UI goroutine
			if time.Since(last) > 80*time.Millisecond {
				last = time.Now()
				pct := float64(-1)
				if total > 0 {
					pct = float64(written) / float64(total) * 100
				}
				onProgress(DownloadProgress{Stage: StagePull, Percent: pct})
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return fmt.Errorf("读取下载流失败: %w", readErr)
		}
	}
	// 最终一次 100%
	if total > 0 {
		onProgress(DownloadProgress{Stage: StagePull, Percent: 100})
	}
	return nil
}

// extractZip 将 zip 解压到 destDir，按文件数回调解压进度。
// 返回 zip 中所有解压后的相对路径（使用 filepath.Separator）。
func extractZip(src, destDir string, onProgress func(DownloadProgress)) ([]string, error) {
	r, err := zip.OpenReader(src)
	if err != nil {
		return nil, fmt.Errorf("打开 zip 失败: %w", err)
	}
	defer r.Close()

	total := len(r.File)
	for i, f := range r.File {
		destPath := filepath.Join(destDir, f.Name)
		// 防 zip slip
		if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(destDir)+string(os.PathSeparator)) && destPath != destDir {
			return nil, fmt.Errorf("非法的 zip 路径: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return nil, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return nil, err
		}
		out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return nil, err
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return nil, err
		}
		if _, err := io.Copy(out, rc); err != nil {
			rc.Close()
			out.Close()
			return nil, err
		}
		rc.Close()
		out.Close()
		if total > 0 && (i%3 == 0 || i == total-1) {
			pct := float64(i+1) / float64(total) * 100
			onProgress(DownloadProgress{Stage: StageUnzip, Percent: pct})
		}
	}

	// 收集所有文件相对路径
	var files []string
	for _, f := range r.File {
		if !f.FileInfo().IsDir() {
			files = append(files, filepath.FromSlash(f.Name))
		}
	}
	return files, nil
}

// locateExe 在已解压的文件列表里寻找名为 baseName 的可执行文件。
// Windows 上查找 "baseName.exe"；其他平台查找不带扩展名的 "baseName"。
// 返回相对路径。找不到返回空串。
func locateExe(files []string, baseName string) string {
	want := strings.ToLower(baseName)
	if runtime.GOOS == "windows" {
		want = strings.ToLower(baseName + ".exe")
	}
	for _, f := range files {
		if strings.ToLower(filepath.Base(f)) == want {
			return f
		}
	}
	return ""
}

// extractTarGz 将 .tar.gz 解压到 destDir，按文件数回调解压进度。
// 返回解压后的所有文件相对路径（使用 filepath.Separator）。
// 会保留 tar 条目的权限位（macOS/Linux 的 scrcpy 需要可执行位）。
func extractTarGz(src, destDir string, onProgress func(DownloadProgress)) ([]string, error) {
	f, err := os.Open(src)
	if err != nil {
		return nil, fmt.Errorf("打开 tar.gz 失败: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("解压 gzip 失败: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)

	// 先统计总条目数（需要完整读一遍 tar；为避免双重解压，这里用流式计数近似：
	// 在解压过程中根据已处理数与一个估计值取较小者。但更稳妥的做法是两遍扫描）。
	// 这里采用一遍扫描：边解压边累计，进度按已处理数线性增长，
	// 最终达到 100%。对大文件（scrcpy ~50MB）足够。
	var files []string
	processed := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("读取 tar 条目失败: %w", err)
		}

		destPath := filepath.Join(destDir, hdr.Name)
		// 防 tar slip
		if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(destDir)+string(os.PathSeparator)) && destPath != destDir {
			return nil, fmt.Errorf("非法的 tar 路径: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, os.FileMode(hdr.Mode)&0777); err != nil {
				return nil, err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return nil, err
			}
			out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(hdr.Mode)&0777)
			if err != nil {
				return nil, err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return nil, err
			}
			out.Close()
			files = append(files, filepath.FromSlash(hdr.Name))
		case tar.TypeSymlink:
			// 符号链接：创建链接（相对路径基于 destDir）
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return nil, err
			}
			_ = os.Symlink(hdr.Linkname, destPath)
			files = append(files, filepath.FromSlash(hdr.Name))
		}

		processed++
		// 进度无法知道总数，用对数式增长避免卡在低百分比；
		// 每 5 个文件更新一次，上限 95%，最后统一到 100%。
		if processed%5 == 0 {
			pct := 95.0 * float64(processed) / float64(processed+20)
			onProgress(DownloadProgress{Stage: StageUnzip, Percent: pct})
		}
	}
	onProgress(DownloadProgress{Stage: StageUnzip, Percent: 100})
	return files, nil
}

// DownloadScrcpy 下载当前平台对应的 scrcpy 压缩包并解压到 installDir。
//
// 支持平台：windows/amd64(386)、linux/amd64、darwin/arm64(amd64)。
// Windows 包通常内置 adb；macOS/Linux 包不含 adb（用户需自行安装）。
//
// 成功时回调 StageDone，并返回 (adbRelPath, scrcpyRelPath)。
// 失败时返回 error，调用方一般已通过 onProgress(StageError) 通知 UI。
func DownloadScrcpy(installDir string, onProgress func(DownloadProgress)) (adbRelPath, scrcpyRelPath string, err error) {
	if onProgress == nil {
		onProgress = func(DownloadProgress) {}
	}

	// 1. 查询最新版本
	onProgress(DownloadProgress{Stage: StageQuery, Message: "查询 scrcpy 最新版本..."})
	rel, qerr := queryLatestRelease()
	if qerr != nil {
		onProgress(DownloadProgress{Stage: StageError, Message: qerr.Error()})
		return "", "", qerr
	}

	// 2. 匹配当前平台资产
	onProgress(DownloadProgress{Stage: StageFetch, Message: "定位 " + currentPlatformAssetLabel() + " 对应的压缩包..."})
	asset, spec, aerr := findAsset(rel, runtime.GOOS, runtime.GOARCH)
	if aerr != nil {
		onProgress(DownloadProgress{Stage: StageError, Message: aerr.Error()})
		return "", "", aerr
	}

	// 3. 准备目录与临时文件
	if err := os.MkdirAll(installDir, 0755); err != nil {
		msg := fmt.Sprintf("创建安装目录失败: %v", err)
		onProgress(DownloadProgress{Stage: StageError, Message: msg})
		return "", "", errors.New(msg)
	}
	tmpSuffix := "-*.tar.gz"
	if spec.archive == "zip" {
		tmpSuffix = "-*.zip"
	}
	tmpFile, err := os.CreateTemp("", "scrcpy-"+runtime.GOOS+tmpSuffix)
	if err != nil {
		msg := fmt.Sprintf("创建临时文件失败: %v", err)
		onProgress(DownloadProgress{Stage: StageError, Message: msg})
		return "", "", errors.New(msg)
	}
	tmpPath := tmpFile.Name()
	// 先关闭，交给 downloadToFile 重新打开（避免与 downloader 句柄冲突）
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// 4. 下载
	if derr := downloadToFile(asset.BrowserDownloadURL, tmpPath, asset.Size, onProgress); derr != nil {
		onProgress(DownloadProgress{Stage: StageError, Message: derr.Error()})
		return "", "", derr
	}

	// 5. 解压（按归档类型分派）
	onProgress(DownloadProgress{Stage: StageUnzip, Percent: 0, Message: "解压中..."})
	var files []string
	if spec.archive == "zip" {
		files, err = extractZip(tmpPath, installDir, onProgress)
	} else {
		files, err = extractTarGz(tmpPath, installDir, onProgress)
	}
	if err != nil {
		onProgress(DownloadProgress{Stage: StageError, Message: err.Error()})
		return "", "", err
	}

	// 6. 定位可执行文件
	adbRelPath = locateExe(files, "adb")
	scrcpyRelPath = locateExe(files, "scrcpy")
	if scrcpyRelPath == "" {
		exeName := "scrcpy"
		if runtime.GOOS == "windows" {
			exeName = "scrcpy.exe"
		}
		msg := "解压完成但未找到 " + exeName
		onProgress(DownloadProgress{Stage: StageError, Message: msg})
		return "", "", errors.New(msg)
	}
	// 确保 macOS/Linux 下 scrcpy 有可执行权限（解压时已按 tar mode 保留，
	// 但保险起见再 chmod 一次）
	if runtime.GOOS != "windows" {
		_ = os.Chmod(filepath.Join(installDir, scrcpyRelPath), 0755)
	}

	if adbRelPath == "" {
		if runtime.GOOS == "windows" {
			// Windows 包通常内置 adb，缺失则告警
			onProgress(DownloadProgress{Stage: StageDone, Message: "未在压缩包中发现 adb.exe，请另行配置 adb 路径"})
		} else {
			// macOS/Linux 的 scrcpy 包不含 adb，提示用户自行安装
			onProgress(DownloadProgress{Stage: StageDone, Message: "当前平台压缩包不含 adb，请通过包管理器安装（如 apt/brew install android-platform-tools）"})
		}
	} else {
		onProgress(DownloadProgress{
			Stage:         StageDone,
			Percent:       100,
			AdbRelPath:    adbRelPath,
			ScrcpyRelPath: scrcpyRelPath,
		})
	}
	return adbRelPath, scrcpyRelPath, nil
}
