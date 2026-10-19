package redisx

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"zp-rdm/internal/model"
)

func TestSentinelAddrList(t *testing.T) {
	p := model.ConnectionProfile{Host: "10.0.0.1", Port: 26379}
	// 仅主地址
	if got := sentinelAddrList(p); len(got) != 1 || got[0] != "10.0.0.1:26379" {
		t.Fatalf("got %v", got)
	}
	// 换行 + 逗号混合、空白、重复
	p.SentinelAddrs = " 10.0.0.2:26379 \n10.0.0.3:26379, 10.0.0.2:26379\n\n"
	got := sentinelAddrList(p)
	want := []string{"10.0.0.1:26379", "10.0.0.2:26379", "10.0.0.3:26379"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	// 主地址与附加地址重复时去重
	p.SentinelAddrs = "10.0.0.1:26379"
	if got := sentinelAddrList(p); len(got) != 1 {
		t.Fatalf("got %v", got)
	}
}

// TestNewClientSentinelValidation Sentinel 参数校验（无需真实实例）
func TestNewClientSentinelValidation(t *testing.T) {
	m := NewManager()
	// 缺 master 名
	_, err := m.newClient(model.ConnectionProfile{
		Host: "127.0.0.1", Port: 26379, Sentinel: true,
	}, 0, nil)
	if err == nil || !strings.Contains(err.Error(), "master") {
		t.Fatalf("err = %v, want master 名称提示", err)
	}
	// 无任何哨兵地址
	_, err = m.newClient(model.ConnectionProfile{
		MasterName: "mymaster", Sentinel: true,
	}, 0, nil)
	if err == nil || !strings.Contains(err.Error(), "哨兵地址") {
		t.Fatalf("err = %v, want 哨兵地址提示", err)
	}
}

// ---------- 以下为集成测试 ----------
// 需要 scripts/start-sentinel.sh 拉起的 WSL 哨兵环境
// （master 6395 + replica 6396 + sentinel 26379，master 名 mymaster），
// 未设置 ZP_RDM_TEST_SENTINEL_ADDR 时跳过。

func sentinelTestProfile(t *testing.T) model.ConnectionProfile {
	t.Helper()
	addr := os.Getenv("ZP_RDM_TEST_SENTINEL_ADDR")
	if addr == "" {
		t.Skip("未设置 ZP_RDM_TEST_SENTINEL_ADDR，跳过哨兵集成测试")
	}
	host, portStr, _ := strings.Cut(addr, ":")
	port, _ := strconv.Atoi(portStr)
	return model.ConnectionProfile{
		ID: "test-sentinel-conn", Name: "sentinel-test",
		Host: host, Port: port, Sentinel: true, MasterName: "mymaster",
	}
}

func TestSentinelOpenAndRW(t *testing.T) {
	p := sentinelTestProfile(t)
	m := NewManager()
	ctx := context.Background()

	st, err := m.Open(ctx, p)
	if err != nil {
		t.Fatalf("open sentinel: %v", err)
	}
	t.Cleanup(func() { m.Close(p.ID) })

	if st.Mode != "sentinel" {
		t.Errorf("mode = %q, want sentinel", st.Mode)
	}
	if st.DBCount < 1 {
		t.Errorf("dbCount = %d, want >= 1（哨兵模式 master 多 db）", st.DBCount)
	}

	c, err := m.Get(p.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Set(ctx, "zp:sentinel:test", "ok", time.Minute).Err(); err != nil {
		t.Fatalf("set: %v", err)
	}
	v, err := c.Get(ctx, "zp:sentinel:test").Result()
	if err != nil || v != "ok" {
		t.Fatalf("get = %q, %v", v, err)
	}

	// 多 db：FailoverClient 按 db 分别缓存
	c5, err := m.Get(p.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if c5 == c {
		t.Error("db5 与 db0 应是不同 client")
	}
	if err := c5.Set(ctx, "zp:sentinel:db5", "x", time.Minute).Err(); err != nil {
		t.Fatalf("db5 set: %v", err)
	}
}
