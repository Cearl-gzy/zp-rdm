package updatecheck

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// chunkServer 按 chunk 大小延迟吐内容的测试服务器（感知客户端取消）
func chunkServer(t *testing.T, content []byte, chunk int, delay time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(content)))
		for off := 0; off < len(content); off += chunk {
			end := off + chunk
			if end > len(content) {
				end = len(content)
			}
			if _, err := w.Write(content[off:end]); err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			if delay > 0 {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(delay):
				}
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testContent(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i * 7)
	}
	return b
}

func TestRaceCandidates(t *testing.T) {
	url := "https://github.com/o/r/releases/download/v1/x.exe"
	got := RaceCandidates(url, "https://api.github.com/repos/o/r/releases/assets/1")
	want := 1 + len(MirrorPrefixes) + 1
	if len(got) != want {
		t.Fatalf("候选数 = %d, want %d: %v", len(got), want, got)
	}
	if got[0] != url {
		t.Errorf("首个候选应为官方直链, got %s", got[0])
	}
	for i, p := range MirrorPrefixes {
		if got[1+i] != p+url {
			t.Errorf("镜像候选 %d = %s, want %s", i, got[1+i], p+url)
		}
	}
	if last := got[len(got)-1]; !strings.HasPrefix(last, "https://api.github.com/") {
		t.Errorf("末位应为 API 回退, got %s", last)
	}

	// 非 github.com 地址不加镜像
	if got := RaceCandidates("https://example.com/x"); len(got) != 1 {
		t.Errorf("非 github 地址候选数 = %d, want 1", len(got))
	}
	// 去重
	if got := RaceCandidates(url, url); len(got) != 1+len(MirrorPrefixes) {
		t.Errorf("重复地址未去重: %v", got)
	}
}

func TestRaceDownloadFastWins(t *testing.T) {
	content := testContent(64 * 1024)
	fast := chunkServer(t, content, 4096, 0)
	slow := chunkServer(t, content, 1024, 20*time.Millisecond) // ~1.3s

	dst := filepath.Join(t.TempDir(), "pkg.exe")
	start := time.Now()
	// 慢源在前，证明胜出与顺序无关
	winner, err := RaceDownload(context.Background(),
		[]string{slow.URL, fast.URL}, dst, sha256Hex(content), nil)
	if err != nil {
		t.Fatal(err)
	}
	if winner != fast.URL {
		t.Errorf("胜出来源 = %s, want fast %s", winner, fast.URL)
	}
	if time.Since(start) > time.Second {
		t.Errorf("竞速耗时 %v，应远快于慢源的 ~1.3s", time.Since(start))
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != string(content) {
		t.Errorf("产物内容不符 (err=%v)", err)
	}
	// 失败的 .part 临时文件应清理干净
	if parts, _ := filepath.Glob(dst + ".part*"); len(parts) > 0 {
		t.Errorf("残留临时文件: %v", parts)
	}
}

func TestRaceDownloadBadHashRejectedGoodWins(t *testing.T) {
	good := testContent(64 * 1024)
	bad := append([]byte(nil), good...)
	bad[0] ^= 0xFF // 同长度坏内容

	badSrv := chunkServer(t, bad, 4096, 0)                    // 快但坏
	goodSrv := chunkServer(t, good, 1024, 5*time.Millisecond) // 慢但好（~320ms）

	dst := filepath.Join(t.TempDir(), "pkg.exe")
	winner, err := RaceDownload(context.Background(),
		[]string{badSrv.URL, goodSrv.URL}, dst, sha256Hex(good), nil)
	if err != nil {
		t.Fatal(err)
	}
	if winner != goodSrv.URL {
		t.Errorf("坏源应被校验拒绝，胜出来源 = %s, want %s", winner, goodSrv.URL)
	}
	if got, _ := os.ReadFile(dst); string(got) != string(good) {
		t.Error("产物应为好源内容")
	}
}

func TestRaceDownloadHashMismatchFails(t *testing.T) {
	content := testContent(4096)
	srv := chunkServer(t, content, 4096, 0)
	dst := filepath.Join(t.TempDir(), "pkg.exe")
	_, err := RaceDownload(context.Background(), []string{srv.URL}, dst, sha256Hex([]byte("别的内容")), nil)
	if err == nil || !strings.Contains(err.Error(), "校验失败") {
		t.Errorf("err = %v, want 校验失败", err)
	}
	if _, serr := os.Stat(dst); !os.IsNotExist(serr) {
		t.Error("校验失败不应留下产物")
	}
}

func TestRaceDownloadAllFail(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "pkg.exe")
	_, err := RaceDownload(context.Background(),
		[]string{"http://127.0.0.1:1/a", "http://127.0.0.1:1/b"}, dst, "", nil)
	if err == nil {
		t.Error("全部失败应返回错误")
	}
	if _, serr := os.Stat(dst); !os.IsNotExist(serr) {
		t.Error("全部失败不应留下产物")
	}
}

func TestRaceDownloadProgress(t *testing.T) {
	content := testContent(256 * 1024)
	srv := chunkServer(t, content, 4096, time.Millisecond)

	var mu sync.Mutex
	var maxD, lastTotal int64
	dst := filepath.Join(t.TempDir(), "pkg.exe")
	_, err := RaceDownload(context.Background(), []string{srv.URL}, dst, sha256Hex(content),
		func(d, total int64) {
			mu.Lock()
			if d > maxD {
				maxD = d
			}
			lastTotal = total
			mu.Unlock()
		})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if maxD != int64(len(content)) {
		t.Errorf("最大进度 = %d, want %d", maxD, len(content))
	}
	if lastTotal != int64(len(content)) {
		t.Errorf("total = %d, want %d", lastTotal, len(content))
	}
}

func TestRaceDownloadContextCancel(t *testing.T) {
	content := testContent(1024 * 1024)
	slow := chunkServer(t, content, 1024, 50*time.Millisecond) // ~50s，必须被取消打断
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	dst := filepath.Join(t.TempDir(), "pkg.exe")
	start := time.Now()
	if _, err := RaceDownload(ctx, []string{slow.URL}, dst, "", nil); err == nil {
		t.Error("取消后应返回错误")
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("取消后耗时 %v，应快速返回", time.Since(start))
	}
}
