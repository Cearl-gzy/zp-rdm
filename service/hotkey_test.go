package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
)

func TestParseMonitorLine(t *testing.T) {
	cases := []struct {
		name string
		line string
		cmd  string
		args []string
		ok   bool
	}{
		{"simple", `1697360000.123 [0 127.0.0.1:6379] "GET" "mykey"`, "GET", []string{"mykey"}, true},
		{"multi-arg", `1697360000.123 [0 127.0.0.1] "SET" "k" "v v"`, "SET", []string{"k", "v v"}, true},
		{"lowercase→upper", `1697360000.123 [3 127.0.0.1] "get" "k"`, "GET", []string{"k"}, true},
		{"escaped-quote", `1697360000.123 [0 x] "SET" "k" "a\"b"`, "SET", []string{"k", `a"b`}, true},
		{"escaped-backslash", `1697360000.123 [0 x] "GET" "a\\b"`, "GET", []string{`a\b`}, true},
		{"empty-arg", `1697360000.123 [0 x] "SET" "k" ""`, "SET", []string{"k", ""}, true},
		{"no-bracket", `+OK`, "", nil, false},
		{"unclosed-quote", `1697360000.1 [0 x] "GET" "k`, "", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd, args, ok := parseMonitorLine(c.line)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if !ok {
				return
			}
			if cmd != c.cmd {
				t.Fatalf("cmd = %q, want %q", cmd, c.cmd)
			}
			if len(args) != len(c.args) {
				t.Fatalf("args = %v, want %v", args, c.args)
			}
			for i := range args {
				if args[i] != c.args[i] {
					t.Fatalf("args[%d] = %q, want %q", i, args[i], c.args[i])
				}
			}
		})
	}
}

func TestHotKeyTargets(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		args []string
		want []string
	}{
		{"single", "GET", []string{"k"}, []string{"k"}},
		{"set-first-only", "SET", []string{"k", "v", "EX", "10"}, []string{"k"}},
		{"del-all", "DEL", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"unlink-all", "UNLINK", []string{"a", "b"}, []string{"a", "b"}},
		{"exists-all", "EXISTS", []string{"a", "b"}, []string{"a", "b"}},
		{"mget-all", "MGET", []string{"a", "b"}, []string{"a", "b"}},
		{"mset-pairs", "MSET", []string{"k1", "v1", "k2", "v2"}, []string{"k1", "k2"}},
		{"rename-two", "RENAME", []string{"old", "new"}, []string{"old", "new"}},
		{"eval-numkeys", "EVAL", []string{"return 1", "2", "k1", "k2", "arg"}, []string{"k1", "k2"}},
		{"eval-zero-keys", "EVAL", []string{"return 1", "0"}, nil},
		{"keyless", "PING", nil, nil},
		{"unknown-default-first", "GEOSEARCH", []string{"geo", "FROMLONLAT", "1", "2"}, []string{"geo"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := hotKeyTargets(c.cmd, c.args)
			if len(got) != len(c.want) {
				t.Fatalf("targets = %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("targets[%d] = %q, want %q", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestHotKeyCmdClass(t *testing.T) {
	for _, cmd := range []string{"GET", "MGET", "HGETALL", "ZRANGE", "TTL", "JSON.GET"} {
		if !hotKeyCmdClass(cmd) {
			t.Fatalf("%s should be read", cmd)
		}
	}
	for _, cmd := range []string{"SET", "HSET", "LPUSH", "ZADD", "DEL", "EXPIRE", "EVAL", "GETDEL"} {
		if hotKeyCmdClass(cmd) {
			t.Fatalf("%s should be write", cmd)
		}
	}
}

func TestInsertBigKey(t *testing.T) {
	var top []model.BigKeyEntry
	for _, sz := range []int64{5, 9, 1, 7, 3, 8} {
		top = insertBigKey(top, model.BigKeyEntry{Key: strings.Repeat("k", int(sz)), Size: sz}, 3)
	}
	if len(top) != 3 {
		t.Fatalf("len = %d, want 3", len(top))
	}
	for i, want := range []int64{9, 8, 7} {
		if top[i].Size != want {
			t.Fatalf("top[%d] = %d, want %d (%+v)", i, top[i].Size, want, top)
		}
	}
	// 比最后一名小 → 不插入
	top = insertBigKey(top, model.BigKeyEntry{Key: "tiny", Size: 2}, 3)
	if len(top) != 3 || top[2].Size != 7 {
		t.Fatalf("small entry should not displace: %+v", top)
	}
}

func TestInsertFreqKey(t *testing.T) {
	var top []model.HotKeyFreqEntry
	for _, f := range []int64{3, 10, 1, 5} {
		top = insertFreqKey(top, model.HotKeyFreqEntry{Freq: f}, 2)
	}
	if len(top) != 2 || top[0].Freq != 10 || top[1].Freq != 5 {
		t.Fatalf("unexpected top: %+v", top)
	}
}

func newServerService(t *testing.T) (*ServerService, *redis.Client, string) {
	m, seed, connID := newTestManager(t)
	s := NewServerService(m)
	s.Startup(context.Background())
	return s, seed, connID
}

// TestHotKeyFreqNotLFU 非 LFU 策略时返回 Available=false 与当前策略名
func TestHotKeyFreqNotLFU(t *testing.T) {
	s, _, connID := newServerService(t)
	rep, err := s.HotKeyFreq(connID, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Policy == "" {
		t.Skip("server does not expose maxmemory-policy (miniredis)")
	}
	if strings.Contains(rep.Policy, "lfu") {
		t.Skip("test instance already in LFU policy")
	}
	if rep.Available {
		t.Fatalf("policy %q should not be available for OBJECT FREQ", rep.Policy)
	}
}

// TestHotKeySample 需要服务端支持 MONITOR（miniredis 不支持则跳过）。
// 真实 Redis：采样期间从另一个客户端发命令，断言读/写分离计数。
func TestHotKeySample(t *testing.T) {
	s, seed, connID := newServerService(t)
	if err := s.HotKeySampleStart(connID, 0); err != nil {
		t.Skipf("MONITOR not supported (miniredis): %v", err)
	}
	// 重复启动应报错
	if err := s.HotKeySampleStart(connID, 0); err == nil {
		t.Fatal("second start should fail")
	}

	for i := 0; i < 3; i++ {
		_, _ = seed.Get(tctx, "hot").Result()
	}
	mustSet(t, seed, "hot", "v")
	_, _ = seed.Get(tctx, "cold").Result()
	_, _ = seed.Get(tctx, "cold").Result()

	// 等 MONITOR 流到达
	deadline := time.Now().Add(3 * time.Second)
	for {
		st := s.HotKeySampleStats(connID, 0)
		if st.Running && st.TotalCmds >= 6 {
			break
		}
		if time.Now().After(deadline) {
			_ = stopSampling(t, s, connID)
			t.Fatalf("stats not catching up: %+v", st)
		}
		time.Sleep(50 * time.Millisecond)
	}

	rep, err := s.HotKeySampleStop(connID, 0)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]model.HotKeyEntry{}
	for _, e := range rep.Keys {
		byKey[e.Key] = e
	}
	hot, ok := byKey["hot"]
	if !ok {
		t.Fatalf("hot key missing in report: %+v", rep.Keys)
	}
	if hot.Hits != 4 || hot.Reads != 3 || hot.Writes != 1 {
		t.Fatalf("hot = %+v, want hits 4 / reads 3 / writes 1", hot)
	}
	if cold := byKey["cold"]; cold.Hits != 2 || cold.Reads != 2 {
		t.Fatalf("cold = %+v, want hits 2 / reads 2", cold)
	}
	if rep.DurationSec <= 0 {
		t.Fatal("duration should be positive")
	}

	// 取走后状态清除
	if _, err := s.HotKeySampleStop(connID, 0); err == nil {
		t.Fatal("second stop should fail")
	}
}

func stopSampling(t *testing.T, s *ServerService, connID string) error {
	t.Helper()
	_, err := s.HotKeySampleStop(connID, 0)
	return err
}
