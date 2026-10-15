package service

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
	"zp-rdm/internal/redisx"
)

// tctx 测试通用上下文
var tctx = context.Background()

// testRedisAddr 若设置了 ZP_RDM_TEST_REDIS_ADDR（如 CI 中的真实 Redis 容器），
// 测试将连接真实实例以覆盖 miniredis 不支持的命令（DUMP/RESTORE 等）
func testRedisAddr(t *testing.T) (host string, port int) {
	t.Helper()
	addr := os.Getenv("ZP_RDM_TEST_REDIS_ADDR")
	if addr == "" {
		addr = miniredis.RunT(t).Addr()
	}
	h, p, _ := strings.Cut(addr, ":")
	port, _ = strconv.Atoi(p)
	return h, port
}

// newTestManager 启动/连接测试实例并通过 Manager 正常打开连接。
// 返回的 seed 客户端用于种数与断言（对 miniredis 与真实 Redis 一致）。
// 真实 Redis 为共享实例，每次先 FlushDB 隔离测试间状态。
func newTestManager(t *testing.T) (*redisx.Manager, *redis.Client, string) {
	t.Helper()
	host, port := testRedisAddr(t)

	seed := redis.NewClient(&redis.Options{Addr: host + ":" + strconv.Itoa(port)})
	t.Cleanup(func() { _ = seed.Close() })
	if err := seed.FlushDB(tctx).Err(); err != nil {
		t.Fatalf("flush test db: %v", err)
	}

	m := redisx.NewManager()
	connID := "test-conn"
	p := model.ConnectionProfile{ID: connID, Name: "test", Host: host, Port: port, DB: 0}
	if _, err := m.Open(context.Background(), p); err != nil {
		t.Fatalf("open test redis connection: %v", err)
	}
	return m, seed, connID
}

func newTransferService(t *testing.T) (*TransferService, *redis.Client, string) {
	m, seed, connID := newTestManager(t)
	s := NewTransferService(m)
	s.Startup(context.Background())
	return s, seed, connID
}

// requireDump 探测服务端是否支持 DUMP/RESTORE（miniredis 不支持则跳过）
func requireDump(t *testing.T, s *TransferService, connID string) {
	t.Helper()
	c, err := s.cli(connID, 0)
	if err != nil {
		t.Fatal(err)
	}
	probe := "zp-rdm-dump-probe"
	if err := c.Set(s.ctx, probe, "1", 0).Err(); err != nil {
		t.Fatal(err)
	}
	defer c.Del(s.ctx, probe)
	if _, err := c.Dump(s.ctx, probe).Result(); err != nil {
		t.Skipf("server does not support DUMP (miniredis): %v", err)
	}
}

// ---------- 种数 / 断言便捷函数 ----------

func mustSet(t *testing.T, c *redis.Client, key, val string) {
	t.Helper()
	if err := c.Set(tctx, key, val, 0).Err(); err != nil {
		t.Fatal(err)
	}
}

func mustExpire(t *testing.T, c *redis.Client, key string, d time.Duration) {
	t.Helper()
	if err := c.Expire(tctx, key, d).Err(); err != nil {
		t.Fatal(err)
	}
}

func mustGet(t *testing.T, c *redis.Client, key string) string {
	t.Helper()
	v, err := c.Get(tctx, key).Result()
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func mustTTL(t *testing.T, c *redis.Client, key string) time.Duration {
	t.Helper()
	d, err := c.TTL(tctx, key).Result()
	if err != nil {
		t.Fatal(err)
	}
	return d
}
