package service

import (
	"os"
	"testing"
)

// TestGetInfoSections INFO 按 section 分组解析（监控面板的数据源）。
// 回归：Wails v2 绑定不支持变参，GetInfo 曾因此在前端调用时静默悬挂。
// 需要真实 Redis（INFO all；miniredis 不支持），CI 通过 ZP_RDM_TEST_REDIS_ADDR 提供。
func TestGetInfoSections(t *testing.T) {
	if os.Getenv("ZP_RDM_TEST_REDIS_ADDR") == "" {
		t.Skip("需要真实 Redis（ZP_RDM_TEST_REDIS_ADDR），miniredis 不支持 INFO all")
	}
	m, _, connID := newTestManager(t)
	s := NewServerService(m)
	s.Startup(tctx)

	info, err := s.GetInfo(connID, 0, nil)
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if info["server"]["redis_version"] == "" {
		t.Fatalf("expected server.redis_version, got sections: %v", keysOf(info))
	}
	if info["memory"]["used_memory"] == "" {
		t.Fatal("expected memory.used_memory")
	}

	// 指定 section 时只返回该段
	mem, err := s.GetInfo(connID, 0, []string{"memory"})
	if err != nil {
		t.Fatalf("GetInfo memory: %v", err)
	}
	if mem["memory"]["used_memory"] == "" {
		t.Fatal("expected memory section")
	}

	// MemoryInfoSection 走同一条路径
	sec, err := s.MemoryInfoSection(connID, 0)
	if err != nil {
		t.Fatalf("MemoryInfoSection: %v", err)
	}
	if sec["used_memory"] == "" {
		t.Fatal("expected used_memory in MemoryInfoSection")
	}
}

func keysOf(m map[string]map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
