package service

import (
	"testing"

	"github.com/redis/go-redis/v9"
)

// Stream 消费者组集成测试。
// miniredis v2.35 已支持 XGROUP/XREADGROUP/XACK/XPENDING，可在无真实 Redis 时运行；
// 设置 ZP_RDM_TEST_REDIS_ADDR 则走真实实例双验证。
func TestStreamGroupLifecycle(t *testing.T) {
	s, seed, connID := newValueService(t)
	key := "st:g"

	// 种 3 条消息
	for _, v := range []string{"a", "b", "c"} {
		if _, err := seed.XAdd(tctx, &redis.XAddArgs{Stream: key, Values: []string{"v", v}}).Result(); err != nil {
			t.Fatal(err)
		}
	}

	// 创建组（从头消费）
	if err := s.StreamGroupCreate(connID, 0, key, "g1", "0", false); err != nil {
		t.Fatal(err)
	}
	gs, err := s.StreamGroups(connID, 0, key)
	if err != nil {
		t.Fatal(err)
	}
	if len(gs) != 1 || gs[0].Name != "g1" {
		t.Fatalf("groups = %+v", gs)
	}

	// 消费者 c1 读 2 条 → 产生 pending
	msgs, err := seed.XReadGroup(tctx, &redis.XReadGroupArgs{
		Group: "g1", Consumer: "c1", Streams: []string{key, ">"}, Count: 2,
	}).Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || len(msgs[0].Messages) != 2 {
		t.Fatalf("xreadgroup messages = %+v", msgs)
	}
	ids := []string{msgs[0].Messages[0].ID, msgs[0].Messages[1].ID}

	cs, err := s.StreamConsumers(connID, 0, key, "g1")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || cs[0].Name != "c1" || cs[0].Pending != 2 {
		t.Fatalf("consumers = %+v", cs)
	}

	ps, err := s.StreamPending(connID, 0, key, "g1", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Fatalf("pending = %+v", ps)
	}
	for _, p := range ps {
		if p.Consumer != "c1" || p.DeliveryCount != 1 {
			t.Errorf("pending entry = %+v", p)
		}
	}

	// ACK 第一条
	n, err := s.StreamAck(connID, 0, key, "g1", ids[:1])
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("ack = %d, want 1", n)
	}
	ps, _ = s.StreamPending(connID, 0, key, "g1", 100)
	if len(ps) != 1 {
		t.Fatalf("pending after ack = %+v", ps)
	}

	// 删除消费者（清除其 pending）
	n, err = s.StreamConsumerDelete(connID, 0, key, "g1", "c1")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("del consumer cleared pending = %d, want 1", n)
	}
	cs, _ = s.StreamConsumers(connID, 0, key, "g1")
	if len(cs) != 0 {
		t.Fatalf("consumers after delete = %+v", cs)
	}

	// 销毁组
	if err := s.StreamGroupDestroy(connID, 0, key, "g1"); err != nil {
		t.Fatal(err)
	}
	gs, _ = s.StreamGroups(connID, 0, key)
	if len(gs) != 0 {
		t.Fatalf("groups after destroy = %+v", gs)
	}
}

func TestStreamGroupEdgeCases(t *testing.T) {
	s, seed, connID := newValueService(t)

	// mkStream：stream 不存在时自动创建
	if err := s.StreamGroupCreate(connID, 0, "st:auto", "g", "$", true); err != nil {
		t.Fatal(err)
	}
	if n := seed.Exists(tctx, "st:auto").Val(); n != 1 {
		t.Error("mkStream 应创建空 stream")
	}
	gs, err := s.StreamGroups(connID, 0, "st:auto")
	if err != nil || len(gs) != 1 {
		t.Fatalf("groups on auto stream = %+v, %v", gs, err)
	}

	// 不存在的组查询 pending 应报错
	if _, err := s.StreamPending(connID, 0, "st:auto", "no-such-group", 100); err == nil {
		t.Error("查询不存在组的 pending 应报错")
	}

	// startID 缺省为 "$"：已存在消息不产生 entries-read
	if err := seed.XAdd(tctx, &redis.XAddArgs{Stream: "st:d", Values: []string{"v", "1"}}).Err(); err != nil {
		t.Fatal(err)
	}
	if err := s.StreamGroupCreate(connID, 0, "st:d", "g", "", false); err != nil {
		t.Fatal(err)
	}
	gs, _ = s.StreamGroups(connID, 0, "st:d")
	if len(gs) != 1 || gs[0].LastDeliveredID == "0-0" {
		t.Fatalf("默认 startID 应为 $，groups = %+v", gs)
	}
}
