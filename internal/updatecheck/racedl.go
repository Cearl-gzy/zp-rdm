package updatecheck

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MirrorPrefixes 公共 GitHub 加速代理（按近期实测速度排序）。
// 仅用于 github.com 的 Release 直链；产物必须通过 sha256 校验才被接受，
// 镜像只能加速、无法篡改内容。镜像为社区免费服务，失效时竞速自动跳过。
var MirrorPrefixes = []string{
	"https://gh-proxy.com/",
	"https://ghproxy.net/",
	"https://ghfast.top/",
}

// RaceCandidates 构造竞速下载候选地址：官方直链在前，github.com 地址追加镜像前缀变体；
// extra（如 API octet-stream 回退）原样追加、不加镜像。重复地址去重，保持顺序。
func RaceCandidates(url string, extra ...string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(u string) {
		if u != "" && !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	add(url)
	if strings.HasPrefix(url, "https://github.com/") {
		for _, p := range MirrorPrefixes {
			add(p + url)
		}
	}
	for _, e := range extra {
		add(e)
	}
	return out
}

// raceClient 竞速下载共用客户端：连接/响应头超时收紧（慢源快速出局），
// 整体 30 分钟覆盖慢网络大文件；显式走系统代理（自定义 Transport 默认不走）。
var raceClient = &http.Client{
	Timeout: 30 * time.Minute,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
	},
}

// stallTimeout 下载体停滞判定：超过该时长没有任何字节到达即放弃该来源
const stallTimeout = 45 * time.Second

// RaceDownload 并发从多个候选地址下载同一文件：第一个「完整下载且 sha256 校验通过」
// 的来源胜出，产物移动到 dst，其余来源取消并清理。返回胜出来源地址。
// expectedSha256 支持裸 hex 或 "sha256:" 前缀；为空时第一个完整下载胜出（无校验，仅兼容旧清单）。
// onProgress 按各来源的最大已下载字节数回调（可能并发调用，调用方需保证线程安全）。
func RaceDownload(ctx context.Context, urls []string, dst, expectedSha256 string, onProgress func(downloaded, total int64)) (string, error) {
	if len(urls) == 0 {
		return "", fmt.Errorf("无可用下载地址")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg      sync.WaitGroup
		won     atomic.Bool
		winner  atomic.Value // string
		errs    = make([]error, len(urls))
		progMax atomic.Int64
		total   atomic.Int64
	)
	total.Store(-1) // 未知；首个带 Content-Length 的响应确定

	for i, u := range urls {
		wg.Add(1)
		go func(i int, u string) {
			defer wg.Done()
			part := fmt.Sprintf("%s.part%d", dst, i)
			err := raceOne(ctx, u, part, expectedSha256, &progMax, &total, onProgress)
			if err != nil {
				errs[i] = err
				os.Remove(part) //nolint:errcheck 失败/被取消源的清理
				return
			}
			// 产物已下载并校验通过：抢到旗标则转正为 dst，落选则清理
			if won.CompareAndSwap(false, true) {
				if merr := moveFile(part, dst); merr != nil {
					// 同目录 rename+复制回退几乎不可能失败；真失败则整场竞速作废
					errs[i] = merr
					cancel()
					return
				}
				winner.Store(u)
				cancel()
			} else {
				os.Remove(part) //nolint:errcheck 竞速落败，清理产物
			}
		}(i, u)
	}
	wg.Wait()

	if w, _ := winner.Load().(string); w != "" {
		return w, nil
	}
	for _, err := range errs {
		if err != nil {
			return "", fmt.Errorf("所有下载源均失败（首个错误）: %w", err)
		}
	}
	return "", fmt.Errorf("所有下载源均失败")
}

// raceOne 单源下载到 part 文件并校验。返回 nil 表示产物可用（part 文件保留，
// 由调用方决定转正或清理）；任何失败由调用方负责删除 part。
func raceOne(ctx context.Context, url, part, expectedSha256 string, progMax, total *atomic.Int64, onProgress func(int64, int64)) error {
	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if strings.HasPrefix(url, "https://api.github.com/") {
		req.Header.Set("Accept", "application/octet-stream")
	}
	resp, err := raceClient.Do(req) //nolint:gosec URL 来自 Release 资产 / 编译期镜像前缀
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > 0 {
		total.CompareAndSwap(-1, resp.ContentLength)
	}

	f, err := os.Create(part) //nolint:gosec 路径为应用内部临时文件
	if err != nil {
		return err
	}

	// 停滞看门狗：慢不怕（20KB/s 也有稳定字节流），怕的是半开连接永远挂着
	var activity atomic.Int64
	activity.Store(time.Now().UnixNano())
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-reqCtx.Done():
				return
			case <-t.C:
				if time.Since(time.Unix(0, activity.Load())) > stallTimeout {
					cancel()
					return
				}
			}
		}
	}()

	buf := make([]byte, 256*1024)
	var downloaded int64
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			activity.Store(time.Now().UnixNano())
			if _, werr := f.Write(buf[:n]); werr != nil {
				_ = f.Close()
				return werr
			}
			downloaded += int64(n)
			// 聚合进度取各来源最大值（慢源不会拖回进度条）
			for {
				cur := progMax.Load()
				if downloaded <= cur || progMax.CompareAndSwap(cur, downloaded) {
					break
				}
			}
			if onProgress != nil {
				onProgress(progMax.Load(), total.Load())
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			_ = f.Close()
			return fmt.Errorf("下载中断: %w", rerr)
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if cl := resp.ContentLength; cl > 0 && downloaded != cl {
		return fmt.Errorf("下载不完整：%d / %d 字节", downloaded, cl)
	}
	if err := VerifyFileSHA256(part, expectedSha256); err != nil {
		return fmt.Errorf("安装包校验失败: %w", err)
	}
	return nil
}

// moveFile 同目录改名；失败时（如杀软锁定）回退复制 + 删除
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src) //nolint:gosec 路径为应用内部临时文件
	if err != nil {
		return err
	}
	defer in.Close()           //nolint:errcheck
	out, err := os.Create(dst) //nolint:gosec 路径为应用内部临时文件
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}
