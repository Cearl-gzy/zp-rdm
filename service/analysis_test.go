package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func newKeyService(t *testing.T) (*KeyService, *redis.Client, string) {
	m, seed, connID := newTestManager(t)
	s := NewKeyService(m)
	s.Startup(context.Background())
	return s, seed, connID
}

func TestAnalyzeKeysTTLBuckets(t *testing.T) {
	s, c, connID := newKeyService(t)
	set := func(k string, ttl time.Duration) {
		mustSet(t, c, k, "v")
		if ttl > 0 {
			mustExpire(t, c, k, ttl)
		}
	}
	set("perm:1", 0)
	set("perm:2", 0)
	set("h:1", 30*time.Minute)  // <1h
	set("h:2", 12*time.Hour)    // <24h
	set("d:1", 3*24*time.Hour)  // <7d
	set("d:2", 15*24*time.Hour) // <30d
	set("d:3", 40*24*time.Hour) // ≥30d

	a, err := s.AnalyzeKeys(connID, 0, ":")
	if err != nil {
		t.Fatal(err)
	}
	if a.TotalKeys != 7 || a.Scanned != 7 {
		t.Fatalf("total=%d scanned=%d, want 7/7", a.TotalKeys, a.Scanned)
	}
	if a.Truncated {
		t.Fatal("unexpected truncation")
	}
	want := map[string]int64{
		"永久": 2, "< 1 小时": 1, "< 24 小时": 1, "< 7 天": 1, "< 30 天": 1, "≥ 30 天": 1,
	}
	got := map[string]int64{}
	for _, b := range a.TTLBuckets {
		got[b.Label] = b.Count
	}
	for label, wantN := range want {
		if got[label] != wantN {
			t.Fatalf("bucket %q = %d, want %d (all: %+v)", label, got[label], wantN, got)
		}
	}
}

func TestAnalyzeKeysPrefixes(t *testing.T) {
	s, c, connID := newKeyService(t)
	for _, k := range []string{"user:1", "user:2", "user:3", "sess:1", "sess:2", "orphan"} {
		mustSet(t, c, k, "v")
	}
	a, err := s.AnalyzeKeys(connID, 0, ":")
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int64{}
	for _, p := range a.Prefixes {
		counts[p.Prefix] = p.Count
	}
	if counts["user:"] != 3 {
		t.Fatalf("user: count = %d, want 3 (%+v)", counts["user:"], counts)
	}
	if counts["sess:"] != 2 {
		t.Fatalf("sess: count = %d, want 2", counts["sess:"])
	}
	if counts["(无前缀)"] != 1 {
		t.Fatalf("no-prefix count = %d, want 1", counts["(无前缀)"])
	}
}

func TestAnalyzeKeysDelimiter(t *testing.T) {
	s, c, connID := newKeyService(t)
	for _, k := range []string{"a-b-1", "a-b-2", "a-c-1"} {
		mustSet(t, c, k, "v")
	}
	a, err := s.AnalyzeKeys(connID, 0, "-")
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int64{}
	for _, p := range a.Prefixes {
		counts[p.Prefix] = p.Count
	}
	if counts["a-"] != 3 {
		t.Fatalf("delimiter '-' prefix count = %d, want 3 (%+v)", counts["a-"], counts)
	}
}

func TestAnalyzeKeysEmptyDB(t *testing.T) {
	s, _, connID := newKeyService(t)
	a, err := s.AnalyzeKeys(connID, 0, ":")
	if err != nil {
		t.Fatal(err)
	}
	if a.TotalKeys != 0 || a.Scanned != 0 {
		t.Fatalf("empty db: total=%d scanned=%d", a.TotalKeys, a.Scanned)
	}
	if len(a.TopKeys) != 0 || len(a.Prefixes) != 0 {
		t.Fatal("empty db should have no top keys / prefixes")
	}
	// 6 个 TTL 桶应始终存在（前端按固定结构渲染）
	if len(a.TTLBuckets) != 6 {
		t.Fatalf("ttl buckets = %d, want 6", len(a.TTLBuckets))
	}
}

func TestAnalyzeKeysTopKeysBounded(t *testing.T) {
	s, c, connID := newKeyService(t)
	for i := 0; i < 50; i++ {
		mustSet(t, c, fmt.Sprintf("k:%03d", i), "v")
	}
	a, err := s.AnalyzeKeys(connID, 0, ":")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.TopKeys) > 20 {
		t.Fatalf("top keys = %d, want <= 20", len(a.TopKeys))
	}
}
