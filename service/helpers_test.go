package service

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"zp-rdm/internal/model"
	"zp-rdm/internal/redisx"
)

// testRedisAddr 若设置了 ZP_RDM_TEST_REDIS_ADDR（如 CI 中的真实 Redis 容器），
// 测试将连接真实实例以覆盖 miniredis 不支持的命令（DUMP/RESTORE 等）
func testRedisAddr(t *testing.T) (host string, port int, mr *miniredis.Miniredis) {
	t.Helper()
	addr := os.Getenv("ZP_RDM_TEST_REDIS_ADDR")
	if addr == "" {
		mr = miniredis.RunT(t)
		addr = mr.Addr()
	}
	h, p, _ := strings.Cut(addr, ":")
	port, _ = strconv.Atoi(p)
	return h, port, mr
}

// newTestManager 启动/连接测试实例并通过 Manager 正常打开连接
func newTestManager(t *testing.T) (*redisx.Manager, *miniredis.Miniredis, string) {
	t.Helper()
	host, port, mr := testRedisAddr(t)

	m := redisx.NewManager()
	connID := "test-conn"
	p := model.ConnectionProfile{ID: connID, Name: "test", Host: host, Port: port, DB: 0}
	if _, err := m.Open(context.Background(), p); err != nil {
		t.Fatalf("open test redis connection: %v", err)
	}
	return m, mr, connID
}

func newTransferService(t *testing.T) (*TransferService, *miniredis.Miniredis, string) {
	m, mr, connID := newTestManager(t)
	s := NewTransferService(m)
	s.Startup(context.Background())
	return s, mr, connID
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
