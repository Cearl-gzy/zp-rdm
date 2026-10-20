package service

import (
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// Stream XRANGE / XAUTOCLAIM 集成测试。
// miniredis 支持 XRANGE；XAUTOCLAIM 若桩版本不支持则探测跳过（真实 Redis 双验证）。

func TestStreamRange(t *testing.T) {
	s, seed, connID := newValueService(t)
	key := "st:range"
	var ids []string
	for _, v := range []string{"a", "b", "c", "d"} {
		id, err := seed.XAdd(tctx, &redis.XAddArgs{Stream: key, Values: []string{"v", v}}).Result()
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}

	// 全区间
	items, err := s.StreamRange(connID, 0, key, "-", "+", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 || items[0].ID != ids[0] || items[3].Fields["v"] != "d" {
		t.Fatalf("range all = %+v", items)
	}

	// 指定起止（含边界）
	items2, err := s.StreamRange(connID, 0, key, ids[1], ids[2], 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items2) != 2 || items2[0].ID != ids[1] || items2[1].ID != ids[2] {
		t.Fatalf("range mid = %+v", items2)
	}

	// COUNT 截断
	items3, err := s.StreamRange(connID, 0, key, "-", "+", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(items3) != 2 {
		t.Fatalf("range count = %+v", items3)
	}
}

func TestStreamAutoClaim(t *testing.T) {
	s, seed, connID := newValueService(t)
	key := "st:claim"
	id, err := seed.XAdd(tctx, &redis.XAddArgs{Stream: key, Values: []string{"v", "x"}}).Result()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.StreamGroupCreate(connID, 0, key, "g", "0", false); err != nil {
		t.Fatal(err)
	}
	// c1 读走 → pending
	if _, err := seed.XReadGroup(tctx, &redis.XReadGroupArgs{
		Group: "g", Consumer: "c1", Streams: []string{key, ">"}, Count: 1,
	}).Result(); err != nil {
		t.Fatal(err)
	}

	// minIdle=0 立即可认领；miniredis 若不支持 XAUTOCLAIM 则跳过
	items, _, err := s.StreamAutoClaim(connID, 0, key, "g", "c2", 0, "0", 10)
	if err != nil {
		if strings.Contains(err.Error(), "unknown command") {
			t.Skipf("miniredis does not support XAUTOCLAIM: %v", err)
		}
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != id || items[0].Fields["v"] != "x" {
		t.Fatalf("autoclaim = %+v", items)
	}
	// 认领后挂到 c2 名下
	p, err := s.StreamPending(connID, 0, key, "g", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 1 || p[0].Consumer != "c2" {
		t.Fatalf("pending after claim = %+v", p)
	}

	// 刚被认领，minIdle 很大时不再命中
	time.Sleep(10 * time.Millisecond)
	items2, _, err := s.StreamAutoClaim(connID, 0, key, "g", "c3", 3600000, "0", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items2) != 0 {
		t.Fatalf("autoclaim with high minIdle = %+v", items2)
	}
}
