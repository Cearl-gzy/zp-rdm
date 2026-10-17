package service

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// requireMemoryUsage 探测服务端是否支持 MEMORY USAGE（miniredis 不支持则跳过 size 断言）
func supportsMemoryUsage(t *testing.T, s *KeyService, connID string) bool {
	t.Helper()
	c, err := s.cli(connID, 0)
	if err != nil {
		t.Fatal(err)
	}
	probe := "zp-rdm-mem-probe"
	if err := c.Set(s.ctx, probe, "1", 0).Err(); err != nil {
		t.Fatal(err)
	}
	defer c.Del(s.ctx, probe)
	_, err = c.Do(s.ctx, "MEMORY", "USAGE", probe).Int64()
	return err == nil
}

func TestBigKeyReportBasic(t *testing.T) {
	s, c, connID := newKeyService(t)
	mustSet(t, c, "str:1", strings.Repeat("x", 100))
	if err := c.HSet(tctx, "hash:1", "a", "1", "b", "2", "c", "3").Err(); err != nil {
		t.Fatal(err)
	}
	if err := c.RPush(tctx, "list:1", "a", "b").Err(); err != nil {
		t.Fatal(err)
	}
	mustSet(t, c, "str:ttl", "v")
	mustExpire(t, c, "str:ttl", time.Hour)

	rep, err := s.BigKeyReport(connID, 0, "*", 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scanned != 4 || rep.TotalKeys != 4 {
		t.Fatalf("scanned=%d total=%d, want 4/4", rep.Scanned, rep.TotalKeys)
	}
	if rep.Truncated {
		t.Fatal("unexpected truncation")
	}

	typeCount := map[string]int64{}
	for _, ts := range rep.Types {
		typeCount[ts.Type] = ts.Count
	}
	for typ, want := range map[string]int64{"string": 2, "hash": 1, "list": 1} {
		if typeCount[typ] != want {
			t.Fatalf("type %s count = %d, want %d (%+v)", typ, typeCount[typ], want, rep.Types)
		}
	}

	byKey := map[string]int64{}
	ttlByKey := map[string]int64{}
	for _, e := range rep.Top {
		byKey[e.Key] = e.Len
		ttlByKey[e.Key] = e.TTL
	}
	if byKey["str:1"] != 100 {
		t.Fatalf("str:1 len = %d, want 100", byKey["str:1"])
	}
	if byKey["hash:1"] != 3 || byKey["list:1"] != 2 {
		t.Fatalf("element counts wrong: %+v", byKey)
	}
	if ttlByKey["str:ttl"] <= 0 || ttlByKey["str:ttl"] > 3600 {
		t.Fatalf("str:ttl ttl = %d, want (0, 3600]", ttlByKey["str:ttl"])
	}
	if ttlByKey["str:1"] != -1 {
		t.Fatalf("str:1 should be persistent, ttl = %d", ttlByKey["str:1"])
	}
}

func TestBigKeyReportSizeOrdering(t *testing.T) {
	s, c, connID := newKeyService(t)
	if !supportsMemoryUsage(t, s, connID) {
		t.Skip("MEMORY USAGE not supported (miniredis)")
	}
	mustSet(t, c, "big", strings.Repeat("x", 10000))
	mustSet(t, c, "small", "v")
	mustSet(t, c, "mid", strings.Repeat("y", 100))

	rep, err := s.BigKeyReport(connID, 0, "*", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Top) != 3 {
		t.Fatalf("top = %d, want 3", len(rep.Top))
	}
	if rep.Top[0].Key != "big" || rep.Top[1].Key != "mid" || rep.Top[2].Key != "small" {
		t.Fatalf("ordering wrong: %+v", rep.Top)
	}
	if rep.Top[0].Size <= rep.Top[2].Size {
		t.Fatalf("big size %d should exceed small %d", rep.Top[0].Size, rep.Top[2].Size)
	}
}

func TestBigKeyReportPattern(t *testing.T) {
	s, c, connID := newKeyService(t)
	mustSet(t, c, "user:1", "v")
	mustSet(t, c, "user:2", "v")
	mustSet(t, c, "sess:1", "v")

	rep, err := s.BigKeyReport(connID, 0, "user:*", 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scanned != 2 {
		t.Fatalf("scanned = %d, want 2 (pattern user:*)", rep.Scanned)
	}
	if rep.TotalKeys != 3 {
		t.Fatalf("total = %d, want 3（DBSize 不受 pattern 影响）", rep.TotalKeys)
	}
}

func TestBigKeyReportMaxScanTruncated(t *testing.T) {
	s, c, connID := newKeyService(t)
	// 种 1200 键（超过 SCAN COUNT 1000 单批量，保证截断时游标非 0）
	pipe := c.Pipeline()
	for i := 0; i < 1200; i++ {
		pipe.Set(tctx, fmt.Sprintf("k:%04d", i), "v", 0)
	}
	if _, err := pipe.Exec(tctx); err != nil {
		t.Fatal(err)
	}
	rep, err := s.BigKeyReport(connID, 0, "*", 10, 50)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Truncated {
		t.Fatal("expected truncation when maxScan < total keys beyond one batch")
	}
	if rep.Scanned > 2000 { // 截断发生在批次边界，最多扫两个批次
		t.Fatalf("scanned = %d, way beyond maxScan", rep.Scanned)
	}
	if rep.TotalKeys != 1200 {
		t.Fatalf("total = %d, want 1200", rep.TotalKeys)
	}
}

func TestBigKeyReportTopNClamp(t *testing.T) {
	s, c, connID := newKeyService(t)
	for i := 0; i < 30; i++ {
		mustSet(t, c, fmt.Sprintf("k:%03d", i), "v")
	}
	rep, err := s.BigKeyReport(connID, 0, "*", 0, 5) // <10 → 提升为 10
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Top) > 10 {
		t.Fatalf("top = %d, want <= 10", len(rep.Top))
	}
}

func TestBigKeyReportEmpty(t *testing.T) {
	s, _, connID := newKeyService(t)
	rep, err := s.BigKeyReport(connID, 0, "*", 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scanned != 0 || len(rep.Top) != 0 || len(rep.Types) != 0 {
		t.Fatalf("empty db should give empty report: %+v", rep)
	}
}
