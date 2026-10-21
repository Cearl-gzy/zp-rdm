package service

import (
	"strings"
	"testing"
)

// LATENCY 延迟事件测试。miniredis 不支持 LATENCY → 探测跳过；真实 Redis 用
// DEBUG SLEEP 制造一次 command 事件（阈值临时设为 1ms，测试后恢复 0）。

func TestLatencyMonitor(t *testing.T) {
	s, connID := newServerServiceForTest(t)

	// 探测：miniredis 或老版本不支持 LATENCY
	if _, err := s.LatencyLatest(connID, 0); err != nil {
		if strings.Contains(err.Error(), "unknown") {
			t.Skipf("server does not support LATENCY: %v", err)
		}
		t.Fatal(err)
	}

	c, err := s.cli(connID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Do(s.ctx, "CONFIG", "SET", "latency-monitor-threshold-ms", "1").Err(); err != nil {
		t.Skipf("cannot set latency-monitor-threshold-ms: %v", err)
	}
	defer c.Do(s.ctx, "CONFIG", "SET", "latency-monitor-threshold-ms", "0")

	// DEBUG SLEEP 10ms 必然超过 1ms 阈值，产生 command 事件
	if err := c.Do(s.ctx, "DEBUG", "SLEEP", "0.01").Err(); err != nil {
		t.Skipf("DEBUG SLEEP unavailable: %v", err)
	}

	events, err := s.LatencyLatest(connID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var cmd *struct{ latest, max int64 }
	for i := range events {
		if events[i].Name == "command" {
			cmd = &struct{ latest, max int64 }{events[i].LatestMs, events[i].MaxMs}
		}
	}
	if cmd == nil {
		t.Fatalf("LATENCY LATEST 无 command 事件: %+v", events)
	}
	if cmd.latest < 1 || cmd.max < cmd.latest {
		t.Fatalf("command 事件数值异常: latest=%d max=%d", cmd.latest, cmd.max)
	}

	// 历史采样：至少含本次 DEBUG SLEEP
	samples, err := s.LatencyHistory(connID, 0, "command")
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) == 0 {
		t.Fatal("LATENCY HISTORY command 为空")
	}
	for i := 1; i < len(samples); i++ {
		if samples[i].Timestamp < samples[i-1].Timestamp {
			t.Fatalf("历史采样时间未升序: %+v", samples)
		}
	}
	if samples[len(samples)-1].Ms < 1 {
		t.Fatalf("最近采样 %dms，应 >= 1ms", samples[len(samples)-1].Ms)
	}

	// 未发生过的事件：空而非报错
	none, err := s.LatencyHistory(connID, 0, "aof-fsync-always")
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("未发生事件 history = %+v", none)
	}
}
