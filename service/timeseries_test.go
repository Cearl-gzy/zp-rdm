package service

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
	"zp-rdm/internal/redisx"
)

// Redis TimeSeries / RedisBloom 集成测试。
// 与 JSON/Search 共用「模块实例」：优先 ZP_RDM_TEST_STACK_ADDR，回退 ZP_RDM_TEST_JSON_ADDR
// （同一个 redis-stack 实例，6380 上四个模块全加载）；未设置或不支持模块时跳过。
func newStackFixture(t *testing.T, probeCmd ...interface{}) (*redisx.Manager, *redis.Client, string) {
	t.Helper()
	addr := os.Getenv("ZP_RDM_TEST_STACK_ADDR")
	if addr == "" {
		addr = os.Getenv("ZP_RDM_TEST_JSON_ADDR")
	}
	if addr == "" {
		t.Skip("未设置 ZP_RDM_TEST_STACK_ADDR / ZP_RDM_TEST_JSON_ADDR，跳过模块集成测试")
	}
	host, portStr, _ := strings.Cut(addr, ":")
	port, _ := strconv.Atoi(portStr)

	seed := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = seed.Close() })
	if err := seed.FlushDB(tctx).Err(); err != nil {
		t.Fatalf("flush test db: %v", err)
	}
	if err := seed.Do(tctx, probeCmd...).Err(); err != nil {
		t.Skipf("实例不支持所需模块（%v）: %v", probeCmd[0], err)
	}
	seed.Del(tctx, "zp-rdm-stack-probe") //nolint:errcheck

	m := redisx.NewManager()
	connID := "test-stack-conn"
	p := model.ConnectionProfile{ID: connID, Name: "stack-test", Host: host, Port: port, DB: 0}
	if _, err := m.Open(context.Background(), p); err != nil {
		t.Fatalf("open stack test connection: %v", err)
	}
	t.Cleanup(func() { m.Close(connID) })
	return m, seed, connID
}

func newTsService(t *testing.T) (*TsService, *redis.Client, string) {
	t.Helper()
	m, seed, connID := newStackFixture(t, "TS.CREATE", "zp-rdm-stack-probe")
	s := NewTsService(m)
	s.Startup(context.Background())
	return s, seed, connID
}

func TestTsAddAndInfo(t *testing.T) {
	s, seed, connID := newTsService(t)

	// TS.ADD 自动创建序列；ts<=0 → 服务端当前时间（*）
	if _, err := s.TsAdd(connID, 0, "ts:a", 1000, 1.5); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TsAdd(connID, 0, "ts:a", 2000, 2.5); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TsAdd(connID, 0, "ts:a", 0, 3.5); err != nil {
		t.Fatal(err)
	}

	info, err := s.TsInfo(connID, 0, "ts:a")
	if err != nil {
		t.Fatal(err)
	}
	if info.TotalSamples != 3 {
		t.Fatalf("TotalSamples 应为 3: %+v", info)
	}
	if info.FirstTimestamp != 1000 || info.LastTimestamp <= 2000 {
		t.Fatalf("首末时间戳异常: %+v", info)
	}

	// 标签与降采样规则解析
	if err := seed.Do(tctx, "TS.CREATE", "ts:src", "LABELS", "app", "zp", "env", "test").Err(); err != nil {
		t.Fatal(err)
	}
	if err := seed.Do(tctx, "TS.CREATE", "ts:dst").Err(); err != nil {
		t.Fatal(err)
	}
	if err := seed.Do(tctx, "TS.CREATERULE", "ts:src", "ts:dst", "AGGREGATION", "avg", 60000).Err(); err != nil {
		t.Fatal(err)
	}
	info2, err := s.TsInfo(connID, 0, "ts:src")
	if err != nil {
		t.Fatal(err)
	}
	if info2.Labels["app"] != "zp" || info2.Labels["env"] != "test" {
		t.Fatalf("标签解析异常: %+v", info2.Labels)
	}
	if len(info2.Rules) != 1 || info2.Rules[0].DestKey != "ts:dst" || info2.Rules[0].BucketMs != 60000 {
		t.Fatalf("规则解析异常: %+v", info2.Rules)
	}
}

func TestTsRange(t *testing.T) {
	s, _, connID := newTsService(t)
	for i, v := range []float64{1, 2, 3, 4, 5} {
		if _, err := s.TsAdd(connID, 0, "ts:r", int64(1000*(i+1)), v); err != nil {
			t.Fatal(err)
		}
	}

	// 全区间
	samples, err := s.TsRange(connID, 0, "ts:r", 0, -1, "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 5 || samples[0].Ts != 1000 || samples[4].Value != 5 {
		t.Fatalf("全区间样本异常: %+v", samples)
	}

	// 半开区间
	samples, err = s.TsRange(connID, 0, "ts:r", 2000, 3000, "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 {
		t.Fatalf("区间样本数应为 2: %+v", samples)
	}

	// 聚合（avg，bucket=2000ms，桶按 0 对齐）：[0,2000)→1 [2000,4000)→2.5 [4000,6000)→4.5
	samples, err = s.TsRange(connID, 0, "ts:r", 0, -1, "avg", 2000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 3 || samples[0].Ts != 0 || samples[0].Value != 1 ||
		samples[1].Value != 2.5 || samples[2].Ts != 4000 || samples[2].Value != 4.5 {
		t.Fatalf("聚合结果异常: %+v", samples)
	}

	// 聚合缺 bucket 应报错
	if _, err := s.TsRange(connID, 0, "ts:r", 0, -1, "avg", 0, 0); err == nil {
		t.Fatal("聚合缺 bucket 应返回错误")
	}
}

func TestGetValueTsType(t *testing.T) {
	m, seed, connID := newStackFixture(t, "TS.CREATE", "zp-rdm-stack-probe")
	s := NewValueService(m)
	s.Startup(context.Background())

	if err := seed.Do(tctx, "TS.ADD", "ts:v", 1000, 42).Err(); err != nil {
		t.Fatal(err)
	}
	d, err := s.GetValue(connID, 0, "ts:v")
	if err != nil {
		t.Fatal(err)
	}
	if d.Type != "TSDB-TYPE" || d.Length != 1 {
		t.Fatalf("TS 类型键详情异常: %+v", d)
	}

	// CreateKey ts 分支（自动创建序列）
	if err := s.CreateKey(connID, 0, "ts:created", "ts", "3.14", "", 0, 0); err != nil {
		t.Fatal(err)
	}
	d, err = s.GetValue(connID, 0, "ts:created")
	if err != nil {
		t.Fatal(err)
	}
	if d.Type != "TSDB-TYPE" || d.Length != 1 {
		t.Fatalf("CreateKey ts 异常: %+v", d)
	}
	// 非数值应报错
	if err := s.CreateKey(connID, 0, "ts:bad", "ts", "abc", "", 0, 0); err == nil {
		t.Fatal("非数值样本应返回错误")
	}
}
