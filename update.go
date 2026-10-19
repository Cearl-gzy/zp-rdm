package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"zp-rdm/internal/updatecheck"
)

// 应用内自动更新：DownloadUpdate 下载安装包（进度事件 update:download）→
// sha256 校验 → ApplyUpdate 退出并由辅助进程提权静默安装到原目录、重启新版。
// 仅 Windows 自动安装；其他平台仍走浏览器下载（macOS 建议 brew / Linux 包管理器）。

// updateProgress 下载进度事件负载
type updateProgress struct {
	Downloaded int64 `json:"downloaded"`
	Total      int64 `json:"total"`
	Done       bool  `json:"done"`
}

// updateRepoOwner/updateRepoName 下载回退（API octet-stream）用的仓库坐标
const (
	updateRepoOwner = "Cearl-gzy"
	updateRepoName  = "zp-rdm"
)

// updateDownloadURLs 候选下载地址：先是 Release 资产直链；有资产 ID 时追加
// API octet-stream 地址作为回退（github.com 在部分网络不可达，api.github.com 通常可用）。
func updateDownloadURLs(info UpdateInfo) []string {
	urls := []string{info.URL}
	if info.assetID != 0 {
		urls = append(urls, fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/assets/%d",
			updateRepoOwner, updateRepoName, info.assetID))
	}
	return urls
}

// fetchFirstOK 依次尝试候选地址，返回第一个 HTTP 200 的响应（调用方负责 Close）。
// 首个之后的地址按 API 资产接口对待，声明 Accept: application/octet-stream。
func fetchFirstOK(client *http.Client, urls []string) (*http.Response, error) {
	var lastErr error
	for i, u := range urls {
		if u == "" {
			continue
		}
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			lastErr = err
			continue
		}
		if i > 0 {
			req.Header.Set("Accept", "application/octet-stream")
		}
		resp, err := client.Do(req) //nolint:gosec URL 来自 Release 资产 / 编译期常量拼接
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		_ = resp.Body.Close()
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("无可用下载地址")
	}
	return nil, fmt.Errorf("下载失败: %w", lastErr)
}

// fetchUpdate 按候选地址下载更新包响应。
// 连接/响应头超时收紧：首个地址（github.com）不可达时快速失败，及时回退 API 通道；
// 整体 30 分钟覆盖慢网络大文件下载。
func fetchUpdate(info UpdateInfo) (*http.Response, error) {
	client := &http.Client{
		Timeout: 30 * time.Minute,
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 20 * time.Second,
		},
	}
	return fetchFirstOK(client, updateDownloadURLs(info))
}

// DownloadUpdate 下载最近检查到的更新包并校验。返回本地文件路径。
func (a *App) DownloadUpdate() (string, error) {
	info := a.pendingUpdate
	if !info.HasUpdate || !info.Auto {
		return "", fmt.Errorf("当前环境不支持自动安装，请使用网页下载")
	}

	resp, err := fetchUpdate(info)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck

	dst := filepath.Join(os.TempDir(), fmt.Sprintf("zp-rdm-update-%s.exe", info.Version))
	f, err := os.Create(dst) //nolint:gosec 固定临时目录 + 版本号文件名
	if err != nil {
		return "", err
	}
	removeOnErr := func(err error) (string, error) {
		_ = f.Close()
		_ = os.Remove(dst)
		return "", err
	}

	total := resp.ContentLength // 未知时为 -1，前端按不确定进度显示
	var downloaded int64
	var lastEmit int64
	buf := make([]byte, 256*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return removeOnErr(werr)
			}
			downloaded += int64(n)
			d := downloaded
			// 节流：每 ~512KB 或完成时推一次进度
			if d-lastEmit >= 512*1024 || d == total {
				lastEmit = d
				wailsRuntime.EventsEmit(a.ctx, "update:download", updateProgress{Downloaded: d, Total: total})
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return removeOnErr(fmt.Errorf("下载中断: %w", rerr))
		}
	}
	if err := f.Close(); err != nil {
		return removeOnErr(err)
	}
	if total > 0 && downloaded != total {
		return removeOnErr(fmt.Errorf("下载不完整：%d / %d 字节", downloaded, total))
	}

	// sha256 校验（Auto 的前提是 Sha256 非空）
	if err := updatecheck.VerifyFileSHA256(dst, info.Sha256); err != nil {
		return removeOnErr(fmt.Errorf("安装包校验失败: %w", err))
	}
	wailsRuntime.EventsEmit(a.ctx, "update:download", updateProgress{Downloaded: downloaded, Total: total, Done: true})
	a.updateFile = dst
	return dst, nil
}

// ApplyUpdate 退出应用并静默安装已下载的更新包（Windows）。
// 辅助进程等主进程退出后弹一次 UAC，提权执行 installer /S /D=<当前安装目录>，
// 保留原安装位置，随后启动新版（拒绝 UAC 则回退启动旧版）。
func (a *App) ApplyUpdate() error {
	if a.updateFile == "" {
		return fmt.Errorf("尚未下载更新包")
	}
	if _, err := os.Stat(a.updateFile); err != nil {
		return fmt.Errorf("更新包不存在: %w", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe = filepath.Clean(exe)
	targetDir := filepath.Dir(exe)
	exeName := filepath.Base(exe) // 通常为 zp-rdm.exe

	if err := startUpdateHelper(a.updateFile, targetDir, exeName); err != nil {
		return err
	}
	// 辅助进程已接管，退出主程序（触发 onShutdown 保存状态）
	wailsRuntime.Quit(a.ctx)
	return nil
}
