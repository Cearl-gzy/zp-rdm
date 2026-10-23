package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestSuggestKeys(t *testing.T) {
	s, seed, connID := newKeyService(t)

	mustSet(t, seed, "user:1", "a")
	mustSet(t, seed, "user:2", "b")
	mustSet(t, seed, "user:10", "c")
	mustSet(t, seed, "order:1", "d")
	// glob 元字符键名：补全前缀必须按字面匹配而非模式
	mustSet(t, seed, "spec[ial]:1", "e")
	mustSet(t, seed, "star*key", "f")

	got, err := s.SuggestKeys(connID, 0, "user:")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("user: 前缀应命中 3 个键，实际 %v", got)
	}

	got, err = s.SuggestKeys(connID, 0, "spec[ial]:")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "spec[ial]:1" {
		t.Fatalf("glob 元字符前缀应按字面命中 spec[ial]:1，实际 %v", got)
	}

	got, err = s.SuggestKeys(connID, 0, "star*")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "star*key" {
		t.Fatalf("含 * 的前缀应只字面命中 star*key，实际 %v", got)
	}

	got, err = s.SuggestKeys(connID, 0, "no-such-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("无匹配应返回空，实际 %v", got)
	}
}

func TestBatchSetTTL(t *testing.T) {
	s, seed, connID := newKeyService(t)

	mustSet(t, seed, "ttl:1", "a")
	mustSet(t, seed, "ttl:2", "b")

	// 批量设置过期
	n, err := s.BatchSetTTL(connID, 0, []string{"ttl:1", "ttl:2", "ttl:not-exist"}, 60)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("应成功设置 2 个键，实际 %d", n)
	}
	if ttl := seed.TTL(tctx, "ttl:1").Val(); ttl < 50*time.Second {
		t.Fatalf("ttl:1 TTL 应接近 60s，实际 %v", ttl)
	}

	// 批量清除过期
	n, err = s.BatchSetTTL(connID, 0, []string{"ttl:1", "ttl:2"}, -1)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("应成功清除 2 个键，实际 %d", n)
	}
	if ttl := seed.TTL(tctx, "ttl:1").Val(); ttl != -1 {
		t.Fatalf("ttl:1 清除后 TTL 应为 -1，实际 %v", ttl)
	}
}

func TestMoveKeys(t *testing.T) {
	s, seed, connID := newKeyService(t)

	// 目标 db1 的独立客户端（go-redis 连接池下 SELECT 不可靠）。
	// 注意必须从 seed 取地址：testRedisAddr 在无环境变量时会另起 miniredis 实例
	seed1 := redis.NewClient(&redis.Options{Addr: seed.Options().Addr, DB: 1})
	t.Cleanup(func() { _ = seed1.Close() })
	// 真实 Redis 为共享实例，db1 也需清空
	if err := seed1.FlushDB(tctx).Err(); err != nil {
		t.Fatal(err)
	}

	mustSet(t, seed, "mv:1", "a")
	mustSet(t, seed, "mv:2", "b")
	// 目标 db1 预置同名键：mv:2 应因冲突被跳过
	mustSet(t, seed1, "mv:2", "exist-in-dst")

	n, err := s.MoveKeys(connID, 0, []string{"mv:1", "mv:2", "mv:not-exist"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("只有 mv:1 应移动成功，实际成功 %d", n)
	}
	if seed.Exists(tctx, "mv:1").Val() != 0 {
		t.Error("mv:1 移动后不应留在 db0")
	}
	if mustGet(t, seed, "mv:2") != "b" {
		t.Error("mv:2 因冲突应保留在 db0")
	}

	// 目标 db 与当前相同应报错
	if _, err = s.MoveKeys(connID, 0, []string{"mv:2"}, 0); err == nil {
		t.Error("目标 db 与当前相同时应返回错误")
	}
}

func TestSuggestKeysLimit(t *testing.T) {
	s, seed, connID := newKeyService(t)

	for i := 0; i < 30; i++ {
		mustSet(t, seed, fmt.Sprintf("bulk:%02d", i), "v")
	}
	got, err := s.SuggestKeys(connID, 0, "bulk:")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > suggestMax {
		t.Fatalf("返回数不应超过 %d，实际 %d", suggestMax, len(got))
	}
	if len(got) == 0 {
		t.Fatal("应有匹配结果")
	}
}

func TestDeleteByPattern(t *testing.T) {
	s, seed, connID := newKeyService(t)

	mustSet(t, seed, "cache:a", "1")
	mustSet(t, seed, "cache:b", "2")
	mustSet(t, seed, "cache:sub:c", "3")
	mustSet(t, seed, "user:1", "4")

	// 计数预览
	n, err := s.CountByPattern(connID, 0, "cache:*")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("cache:* 应匹配 3 个键，实际 %d", n)
	}

	// 删除
	d, err := s.DeleteByPattern(connID, 0, "cache:*")
	if err != nil {
		t.Fatal(err)
	}
	if d != 3 {
		t.Fatalf("应删除 3 个键，实际 %d", d)
	}
	if got := mustGet(t, seed, "user:1"); got != "4" {
		t.Fatal("非匹配键不应受影响")
	}
	if n, _ := s.CountByPattern(connID, 0, "cache:*"); n != 0 {
		t.Fatalf("删除后应无匹配，实际 %d", n)
	}

	// 空模式与无匹配
	if _, err = s.DeleteByPattern(connID, 0, ""); err == nil {
		t.Error("空模式应报错")
	}
	if d, _ = s.DeleteByPattern(connID, 0, "nomatch:*"); d != 0 {
		t.Errorf("无匹配应删除 0 个，实际 %d", d)
	}
}
