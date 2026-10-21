package service

import (
	"strings"
	"testing"
)

// MEMORY STATS/DOCTOR/PURGE 测试；miniredis 不支持 MEMORY 命令 → 探测跳过。

func requireMemory(t *testing.T, s *ServerService, connID string) {
	t.Helper()
	if _, err := s.MemoryStats(connID, 0); err != nil {
		t.Skipf("server does not support MEMORY STATS: %v", err)
	}
}

func TestMemoryStats(t *testing.T) {
	s, _, connID := newServerService(t)
	requireMemory(t, s, connID)

	m, err := s.MemoryStats(connID, 0)
	if err != nil {
		t.Fatal(err)
	}
	// 必备字段（Redis 4.0+ 稳定存在）
	for _, k := range []string{"peak.allocated", "total.allocated", "startup.allocated", "dataset.bytes", "overhead.total", "keys.count"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("MEMORY STATS 缺字段 %s: %v", k, m)
		}
	}
	if toInt64(m["total.allocated"]) <= 0 {
		t.Fatalf("total.allocated 应为正数: %v", m["total.allocated"])
	}
	if toInt64(m["keys.count"]) < 0 {
		t.Fatalf("keys.count 越界: %v", m["keys.count"])
	}
}

func TestMemoryDoctor(t *testing.T) {
	s, _, connID := newServerService(t)
	requireMemory(t, s, connID)

	doc, err := s.MemoryDoctor(connID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc, "Sam") {
		t.Fatalf("MEMORY DOCTOR 返回异常: %q", doc)
	}
}

func TestMemoryPurge(t *testing.T) {
	s, _, connID := newServerService(t)
	requireMemory(t, s, connID)
	// jemalloc 平台应无错；个别 allocator 报错属服务端环境差异，仅记录
	if err := s.MemoryPurge(connID, 0); err != nil {
		t.Logf("MEMORY PURGE 不支持（非 jemalloc?）: %v", err)
	}
}

func TestMemoryInfoSection(t *testing.T) {
	s, _, connID := newServerService(t)
	sec, err := s.MemoryInfoSection(connID, 0)
	if err != nil {
		if strings.Contains(err.Error(), "not supported") {
			t.Skipf("INFO memory 段不可用（miniredis）: %v", err)
		}
		t.Fatal(err)
	}
	for _, k := range []string{"used_memory", "used_memory_rss", "used_memory_peak"} {
		if _, ok := sec[k]; !ok {
			t.Fatalf("INFO memory 缺字段 %s", k)
		}
	}
	// 碎片率/分配器字段仅真实服务端提供（miniredis 不模拟）
	if _, ok := sec["mem_allocator"]; ok {
		if _, ok := sec["mem_fragmentation_ratio"]; !ok {
			t.Fatal("INFO memory 缺字段 mem_fragmentation_ratio")
		}
	}
}
