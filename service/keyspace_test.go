package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"zp-rdm/internal/model"
)

// 键空间事件端到端：CONFIG SET notify-keyspace-events + PSUBSCRIBE __keyevent@0__:*
// 验证事件流经 emit 推送（前端键空间事件面板的服务端链路）。
func TestKeyspaceEvents(t *testing.T) {
	s, seed, connID := newServerService(t)

	// 捕获 emit 推送
	var mu sync.Mutex
	var got []model.PubSubMessage
	SetEventEmitter(func(_ context.Context, _ string, data ...any) {
		if len(data) == 0 {
			return
		}
		if m, ok := data[0].(model.PubSubMessage); ok {
			mu.Lock()
			got = append(got, m)
			mu.Unlock()
		}
	})
	t.Cleanup(func() { SetEventEmitter(nil) })

	if err := s.SetConfig(connID, 0, "notify-keyspace-events", "Ex"); err != nil {
		t.Skipf("CONFIG SET 不可用（托管实例）: %v", err)
	}
	t.Cleanup(func() { _ = s.SetConfig(connID, 0, "notify-keyspace-events", "") })

	if err := s.Subscribe(connID, 0, "__keyevent@0__:*", true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.UnsubscribeAll(connID, 0) })

	// 等订阅在服务端生效后再写键（PSUBSCRIBE 下发是异步的）
	time.Sleep(200 * time.Millisecond)
	if err := seed.Set(tctx, "ks:probe", "1", 300*time.Millisecond).Err(); err != nil {
		t.Fatal(err)
	}

	// 轮询等待 expired 事件（最多 3s）
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		for _, m := range got {
			// keyevent 频道：频道名带事件，payload 为键名（与 keyspace 频道相反）
			if m.Channel == "__keyevent@0__:expired" && m.Payload == "ks:probe" {
				mu.Unlock()
				return // 成功
			}
		}
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("未收到 expired 事件: %+v", got)
}
