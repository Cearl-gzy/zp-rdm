package redisx

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
)

func TestIsWriteCommand(t *testing.T) {
	writes := []struct {
		name string
		args []string
	}{
		{"set", []string{"k", "v"}},
		{"getex", []string{"k"}}, // 可改 TTL，官方 @write
		{"getdel", []string{"k"}},
		{"del", []string{"k"}},
		{"expire", []string{"k", "10"}},
		{"rename", []string{"a", "b"}},
		{"sort", []string{"k"}}, // 含 STORE 选项，官方 @write
		{"hset", []string{"k", "f", "v"}},
		{"lpush", []string{"k", "v"}},
		{"zadd", []string{"k", "1", "m"}},
		{"xadd", []string{"k", "*", "f", "v"}},
		{"eval", []string{"return 1", "0"}},
		{"fcall", []string{"fn", "0"}},
		{"publish", []string{"ch", "msg"}},
		{"flushdb", nil},
		{"shutdown", nil},
		{"debug", []string{"sleep", "0"}},
		{"restore-asking", []string{"k", "0", "x"}},
		// 容器命令写子命令
		{"config", []string{"set", "maxmemory", "1mb"}},
		{"config", []string{"rewrite"}},
		{"acl", []string{"setuser", "u"}},
		{"acl", []string{"deluser", "u"}},
		{"script", []string{"load", "return 1"}},
		{"function", []string{"flush"}},
		{"slowlog", []string{"reset"}},
		{"client", []string{"kill", "id", "1"}},
		{"client", []string{"pause", "100"}},
		{"cluster", []string{"setslot", "1", "stable"}},
		{"cluster", []string{"forget", "node"}},
		{"memory", []string{"purge"}},
		{"latency", []string{"reset"}},
		{"module", []string{"load", "/x.so"}},
		// 模块
		{"json.set", []string{"k", "$", "{}"}},
		{"json.arrappend", []string{"k", "$.a", "1"}},
		{"ft.create", []string{"idx", "on", "json"}},
		{"ft.dropindex", []string{"idx"}},
	}
	for _, c := range writes {
		if !IsWriteCommand(c.name, c.args) {
			t.Errorf("IsWriteCommand(%s %v) = false, want true", c.name, c.args)
		}
	}

	reads := []struct {
		name string
		args []string
	}{
		{"get", []string{"k"}},
		{"mget", []string{"a", "b"}},
		{"scan", []string{"0"}},
		{"keys", []string{"*"}},
		{"hgetall", []string{"k"}},
		{"lrange", []string{"k", "0", "-1"}},
		{"zrange", []string{"k", "0", "-1"}},
		{"xrange", []string{"k", "-", "+"}},
		{"eval_ro", []string{"return 1", "0"}},
		{"fcall_ro", []string{"fn", "0"}},
		{"subscribe", []string{"ch"}},
		{"monitor", nil},
		{"dbsize", nil},
		{"info", nil},
		{"multi", nil}, // 事务壳本身不写；入队的写命令会被单独拦截
		{"exec", nil},
		{"auth", []string{"pw"}},
		{"select", []string{"1"}},
		// 容器命令读子命令
		{"config", []string{"get", "maxmemory"}},
		{"acl", []string{"list"}},
		{"acl", []string{"getuser", "u"}},
		{"acl", []string{"genpass"}},
		{"script", []string{"exists", "sha"}},
		{"function", []string{"list"}},
		{"slowlog", []string{"get"}},
		{"client", []string{"setname", "x"}}, // 会话级，无害
		{"client", []string{"list"}},
		{"cluster", []string{"info"}},
		{"cluster", []string{"nodes"}},
		{"cluster", []string{"keyslot", "k"}},
		{"memory", []string{"usage", "k"}},
		{"latency", []string{"history", "cmd"}},
		{"module", []string{"list"}},
		{"command", []string{"docs"}},
		{"pubsub", []string{"channels"}},
		// 模块读命令与未知命令默认放行
		{"json.get", []string{"k"}},
		{"ft.search", []string{"idx", "*"}},
		{"ts.add", []string{"k", "*", "1"}}, // 未收录的模块写命令：默认放行（文档化行为）
	}
	for _, c := range reads {
		if IsWriteCommand(c.name, c.args) {
			t.Errorf("IsWriteCommand(%s %v) = true, want false", c.name, c.args)
		}
	}
}

func guardTestClient(t *testing.T, p model.ConnectionProfile) redis.UniversalClient {
	t.Helper()
	mr := miniredis.RunT(t)
	c := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	p.Name = "测试连接"
	attachGuard(c, p)
	return c
}

func TestGuardReadonlyBlocksWrites(t *testing.T) {
	ctx := context.Background()
	c := guardTestClient(t, model.ConnectionProfile{Readonly: true})

	if err := c.Set(ctx, "k", "v", 0).Err(); err == nil || !strings.Contains(err.Error(), "只读模式") {
		t.Fatalf("SET err = %v, want 只读模式拦截", err)
	}
	if err := c.Eval(ctx, "return 1", nil).Err(); err == nil || !strings.Contains(err.Error(), "只读模式") {
		t.Fatalf("EVAL err = %v, want 拦截", err)
	}
	// 容器写子命令同样在客户端侧拦截（miniredis 不支持 CONFIG，但钩子在发往服务端前触发）
	if err := c.Do(ctx, "CONFIG", "SET", "maxmemory", "1mb").Err(); err == nil || !strings.Contains(err.Error(), "只读模式") {
		t.Fatalf("CONFIG SET err = %v, want 只读模式拦截", err)
	}
	// 读命令不受影响
	if err := c.Get(ctx, "k").Err(); err != redis.Nil {
		t.Fatalf("GET err = %v, want redis.Nil", err)
	}
	if err := c.Do(ctx, "SCAN", "0").Err(); err != nil {
		t.Fatalf("SCAN err = %v", err)
	}
	// 管道：含写命令整管中止
	pipe := c.Pipeline()
	pipe.Set(ctx, "k2", "v", 0)
	if _, err := pipe.Exec(ctx); err == nil || !strings.Contains(err.Error(), "只读模式") {
		t.Fatalf("pipeline err = %v, want 拦截", err)
	}
}

func TestGuardAuditLogsWrites(t *testing.T) {
	AuditDir = t.TempDir()
	defer func() { AuditDir = "" }()
	ctx := context.Background()
	c := guardTestClient(t, model.ConnectionProfile{Audit: true})

	if err := c.Set(ctx, "audit-key", "v1", 0).Err(); err != nil {
		t.Fatal(err)
	}
	_ = c.Get(ctx, "audit-key") // 读命令不审计
	// 容器写子命令：审计在客户端侧记录（miniredis 不支持 CONFIG 会报错，不影响审计写入）
	_ = c.Do(ctx, "CONFIG", "SET", "maxmemory", "2mb")

	files, err := filepath.Glob(filepath.Join(AuditDir, "audit-*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("审计文件 = %v, err = %v", files, err)
	}
	b, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("审计行数 = %d, want 2:\n%s", len(lines), b)
	}
	var e auditEntry
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatal(err)
	}
	if e.Cmd != "SET" || e.Conn != "测试连接" || !strings.Contains(e.Args, "audit-key") {
		t.Errorf("审计记录不符: %+v", e)
	}
	var e2 auditEntry
	if err := json.Unmarshal([]byte(lines[1]), &e2); err != nil {
		t.Fatal(err)
	}
	if e2.Cmd != "CONFIG" {
		t.Errorf("容器写子命令应被审计, got %s", e2.Cmd)
	}
}

func TestGuardDisabledZeroOverhead(t *testing.T) {
	ctx := context.Background()
	// 未开启任何防护：写命令正常执行
	c := guardTestClient(t, model.ConnectionProfile{})
	if err := c.Set(ctx, "k", "v", 0).Err(); err != nil {
		t.Fatalf("未防护时 SET err = %v", err)
	}
}
