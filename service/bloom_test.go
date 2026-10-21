package service

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
)

// RedisBloom 集成测试（与 TimeSeries 共用 newStackFixture）。

func newBfService(t *testing.T) (*BfService, *redis.Client, string) {
	t.Helper()
	m, seed, connID := newStackFixture(t, "BF.RESERVE", "zp-rdm-stack-probe", 0.01, 100)
	s := NewBfService(m)
	s.Startup(context.Background())
	return s, seed, connID
}

func TestBfAddAndExists(t *testing.T) {
	s, _, connID := newBfService(t)

	// BF.ADD 自动创建（默认参数）
	added, err := s.BfAdd(connID, 0, "bf:a", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Fatal("首次添加应返回 true")
	}
	// 重复添加（可能误判，但刚插入的元素一定命中）
	added, err = s.BfAdd(connID, 0, "bf:a", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Fatal("重复添加同一元素不应再返回 true")
	}
	// 空元素报错
	if _, err := s.BfAdd(connID, 0, "bf:a", ""); err == nil {
		t.Fatal("空元素应返回错误")
	}

	res, err := s.BfMExists(connID, 0, "bf:a", []string{"hello", "definitely-not-present-xyz"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || !res[0] || res[1] {
		t.Fatalf("MEXISTS 结果异常: %v", res)
	}
	// 空列表/超限校验
	if _, err := s.BfMExists(connID, 0, "bf:a", nil); err == nil {
		t.Fatal("空列表应返回错误")
	}
	if _, err := s.BfMExists(connID, 0, "bf:a", make([]string, 101)); err == nil {
		t.Fatal("超过 100 个元素应返回错误")
	}
}

func TestBfInfo(t *testing.T) {
	s, seed, connID := newBfService(t)

	if err := seed.Do(tctx, "BF.RESERVE", "bf:i", 0.001, 5000).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BfAdd(connID, 0, "bf:i", "x"); err != nil {
		t.Fatal(err)
	}
	info, err := s.BfInfo(connID, 0, "bf:i")
	if err != nil {
		t.Fatal(err)
	}
	if info.Capacity != 5000 {
		t.Fatalf("Capacity 应为 5000: %+v", info)
	}
	if info.Inserted != 1 {
		t.Fatalf("Inserted 应为 1: %+v", info)
	}
	if info.Size <= 0 || info.Filters < 1 {
		t.Fatalf("Size/Filters 异常: %+v", info)
	}
}

func TestGetValueBfType(t *testing.T) {
	m, seed, connID := newStackFixture(t, "BF.RESERVE", "zp-rdm-stack-probe", 0.01, 100)
	s := NewValueService(m)
	s.Startup(context.Background())

	if err := seed.Do(tctx, "BF.ADD", "bf:v", "hello").Err(); err != nil {
		t.Fatal(err)
	}
	d, err := s.GetValue(connID, 0, "bf:v")
	if err != nil {
		t.Fatal(err)
	}
	if d.Type != "MBbloom--" || d.Length != 1 {
		t.Fatalf("BF 类型键详情异常: %+v", d)
	}

	// CreateKey bf 分支（自动创建过滤器）
	if err := s.CreateKey(connID, 0, "bf:created", "bf", "first-item", "", 0, 0); err != nil {
		t.Fatal(err)
	}
	d, err = s.GetValue(connID, 0, "bf:created")
	if err != nil {
		t.Fatal(err)
	}
	if d.Type != "MBbloom--" || d.Length != 1 {
		t.Fatalf("CreateKey bf 异常: %+v", d)
	}
	// 空元素应报错
	if err := s.CreateKey(connID, 0, "bf:bad", "bf", "  ", "", 0, 0); err == nil {
		t.Fatal("空元素应返回错误")
	}
}
