package service

import (
	"context"
	"strings"
	"testing"
	"time"
)

// PubSub 频道洞察测试（PUBSUB CHANNELS + NUMSUB；miniredis 支持则直接跑）

func newServerServiceForTest(t *testing.T) (*ServerService, string) {
	t.Helper()
	m, _, connID := newTestManager(t)
	s := NewServerService(m)
	s.Startup(context.Background())
	return s, connID
}

func TestPubSubInsight(t *testing.T) {
	s, connID := newServerServiceForTest(t)

	// 无任何订阅时：空洞察而非报错
	ins, err := s.PubSubInsight(connID, 0, "")
	if err != nil {
		if strings.Contains(err.Error(), "unknown") {
			t.Skipf("server does not support PUBSUB subcommands: %v", err)
		}
		t.Fatal(err)
	}
	if len(ins.Channels) != 0 {
		t.Fatalf("空实例 channels = %+v", ins.Channels)
	}

	// 另开一个订阅者客户端（复用同一测试实例地址）
	c, err := s.cli(connID, 0)
	if err != nil {
		t.Fatal(err)
	}
	sub := c.Subscribe(context.Background(), "news", "alert")
	defer sub.Close()
	if _, err := sub.Receive(context.Background()); err != nil {
		t.Fatal(err)
	}

	// 轮询等待服务端登记订阅（避免时序抖动）
	var found map[string]int64
	for i := 0; i < 50; i++ {
		ins, err = s.PubSubInsight(connID, 0, "")
		if err != nil {
			t.Fatal(err)
		}
		found = map[string]int64{}
		for _, ch := range ins.Channels {
			found[ch.Name] = ch.Subs
		}
		if found["news"] >= 1 && found["alert"] >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if found["news"] < 1 || found["alert"] < 1 {
		t.Fatalf("insight channels = %+v, want news/alert 各 >= 1 订阅者", found)
	}

	// 模式过滤
	ins2, err := s.PubSubInsight(connID, 0, "ne*")
	if err != nil {
		t.Fatal(err)
	}
	if len(ins2.Channels) != 1 || ins2.Channels[0].Name != "news" {
		t.Fatalf("pattern insight = %+v", ins2.Channels)
	}
}
