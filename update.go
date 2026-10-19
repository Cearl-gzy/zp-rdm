package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"zp-rdm/internal/updatecheck"
)

// 应用内自动更新：DownloadUpdate 多源竞速下载安装包（进度事件 update:download）→
// 第一个通过 sha256 校验的来源胜出 → ApplyUpdate 退出并由辅助进程提权静默安装到原目录、重启新版。
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

// updateAPIFallbackURL API octet-stream 回退地址（github.com 在部分网络不可达，
// api.github.com 通常可用）。无资产 ID（清单通道）时为空。
func updateAPIFallbackURL(info UpdateInfo) string {
	if info.assetID == 0 {
		return ""
	}
	return fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/assets/%d",
		updateRepoOwner, updateRepoName, info.assetID)
}

// DownloadUpdate 下载最近检查到的更新包并校验。返回本地文件路径。
// 多源竞速：官方直链 + 公共镜像前缀（仅 github.com）+ API 回退并发下载，
// 慢/死源由连接超时与停滞看门狗出局；产物必须通过 sha256 校验（Auto 的前提是 Sha256 非空），
// 因此镜像只能加速、无法篡改。
func (a *App) DownloadUpdate() (string, error) {
	info := a.pendingUpdate
	if !info.HasUpdate || !info.Auto {
		return "", fmt.Errorf("当前环境不支持自动安装，请使用网页下载")
	}

	dst := filepath.Join(os.TempDir(), fmt.Sprintf("zp-rdm-update-%s.exe", info.Version))
	urls := updatecheck.RaceCandidates(info.URL, updateAPIFallbackURL(info))

	// 进度回调可能被多源并发触发：互斥节流，每 ~512KB 或完成时推一次
	var mu sync.Mutex
	var lastEmit int64
	onProgress := func(d, total int64) {
		mu.Lock()
		defer mu.Unlock()
		if d-lastEmit >= 512*1024 || (total > 0 && d == total) {
			lastEmit = d
			wailsRuntime.EventsEmit(a.ctx, "update:download", updateProgress{Downloaded: d, Total: total})
		}
	}

	if _, err := updatecheck.RaceDownload(a.ctx, urls, dst, info.Sha256, onProgress); err != nil {
		_ = os.Remove(dst)
		return "", err
	}

	var size int64
	if st, err := os.Stat(dst); err == nil {
		size = st.Size()
	}
	wailsRuntime.EventsEmit(a.ctx, "update:download", updateProgress{Downloaded: size, Total: size, Done: true})
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
