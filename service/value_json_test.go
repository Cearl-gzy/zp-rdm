package service

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
	"zp-rdm/internal/redisx"
)

// RedisJSON / RediSearch 集成测试。
// 需要一个加载了 ReJSON + search 模块的实例（如 redis-stack），
// 通过 ZP_RDM_TEST_JSON_ADDR=host:port 指定；未设置或实例不支持模块时跳过。
// 本机环境：WSL redis-stack-server 运行在 127.0.0.1:6380。
func newJSONFixture(t *testing.T) (*redisx.Manager, *redis.Client, string) {
	t.Helper()
	addr := os.Getenv("ZP_RDM_TEST_JSON_ADDR")
	if addr == "" {
		t.Skip("未设置 ZP_RDM_TEST_JSON_ADDR，跳过 RedisJSON 集成测试")
	}
	host, portStr, _ := strings.Cut(addr, ":")
	port, _ := strconv.Atoi(portStr)

	seed := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = seed.Close() })
	if err := seed.FlushDB(tctx).Err(); err != nil {
		t.Fatalf("flush test db: %v", err)
	}
	probe := "zp-rdm-json-probe"
	if err := seed.Do(tctx, "JSON.SET", probe, "$", `{"a":1}`).Err(); err != nil {
		t.Skipf("实例不支持 RedisJSON: %v", err)
	}
	seed.Del(tctx, probe) //nolint:errcheck

	m := redisx.NewManager()
	connID := "test-json-conn"
	p := model.ConnectionProfile{ID: connID, Name: "json-test", Host: host, Port: port, DB: 0}
	if _, err := m.Open(context.Background(), p); err != nil {
		t.Fatalf("open json test connection: %v", err)
	}
	t.Cleanup(func() { m.Close(connID) })
	return m, seed, connID
}

func newJSONValueService(t *testing.T) (*ValueService, *redis.Client, string) {
	t.Helper()
	m, seed, connID := newJSONFixture(t)
	s := NewValueService(m)
	s.Startup(context.Background())
	return s, seed, connID
}

func jsonSet(t *testing.T, c redis.UniversalClient, key, doc string) {
	t.Helper()
	if err := c.Do(tctx, "JSON.SET", key, "$", doc).Err(); err != nil {
		t.Fatal(err)
	}
}

func TestJSONGetValue(t *testing.T) {
	s, seed, connID := newJSONValueService(t)

	jsonSet(t, seed, "doc:obj", `{"a":1,"b":[1,2,3]}`)
	jsonSet(t, seed, "doc:arr", `[1,2,3,4]`)
	jsonSet(t, seed, "doc:scalar", `"hello"`)

	d, err := s.GetValue(connID, 0, "doc:obj")
	if err != nil {
		t.Fatal(err)
	}
	if d.Type != "ReJSON-RL" {
		t.Fatalf("type 应为 ReJSON-RL，实际 %q", d.Type)
	}
	if !strings.Contains(d.StrValue, `"a":1`) {
		t.Errorf("StrValue 应包含文档内容，实际 %q", d.StrValue)
	}
	if d.Length != 2 {
		t.Errorf("对象根字段数应为 2，实际 %d", d.Length)
	}

	d, err = s.GetValue(connID, 0, "doc:arr")
	if err != nil {
		t.Fatal(err)
	}
	if d.Length != 4 {
		t.Errorf("数组长度应为 4，实际 %d", d.Length)
	}

	if _, err = s.GetValue(connID, 0, "doc:scalar"); err != nil {
		t.Errorf("标量 JSON 应能读取: %v", err)
	}
}

func TestJSONSetAndTTL(t *testing.T) {
	s, seed, connID := newJSONValueService(t)

	jsonSet(t, seed, "doc:1", `{"v":1}`)
	mustExpire(t, seed, "doc:1", 300*time.Second)

	// ttl<0 保持原 TTL（JSON.SET 本身不保留 TTL，由服务层恢复）
	if err := s.SetJSON(connID, 0, "doc:1", `{"v":2}`, -1); err != nil {
		t.Fatal(err)
	}
	if ttl := mustTTL(t, seed, "doc:1"); ttl <= 0 || ttl > 300*time.Second {
		t.Errorf("保持 TTL 语义下 TTL 应在 (0,300s]，实际 %v", ttl)
	}
	if got, _ := seed.Do(tctx, "JSON.GET", "doc:1", "$.v").Text(); !strings.Contains(got, "2") {
		t.Errorf("更新后 $.v 应为 2，实际 %q", got)
	}

	// ttl>0 覆盖
	if err := s.SetJSON(connID, 0, "doc:1", `{"v":3}`, 100); err != nil {
		t.Fatal(err)
	}
	if ttl := mustTTL(t, seed, "doc:1"); ttl < 90*time.Second || ttl > 100*time.Second {
		t.Errorf("覆盖 TTL 应约为 100s，实际 %v", ttl)
	}

	// ttl==0 清除过期
	if err := s.SetJSON(connID, 0, "doc:1", `{"v":4}`, 0); err != nil {
		t.Fatal(err)
	}
	if ttl := mustTTL(t, seed, "doc:1"); ttl != -1 {
		t.Errorf("ttl==0 应变为永久，实际 %v", ttl)
	}

	// 非法 JSON 报错
	if err := s.SetJSON(connID, 0, "doc:1", `{bad json`, -1); err == nil {
		t.Error("非法 JSON 应返回错误")
	}
}

func TestJSONCreateKeyAndFull(t *testing.T) {
	s, seed, connID := newJSONValueService(t)

	if err := s.CreateKey(connID, 0, "doc:new", "json", `{"x":[1]}`, "", 0, 0); err != nil {
		t.Fatal(err)
	}
	if typ := seed.Type(tctx, "doc:new").Val(); typ != "ReJSON-RL" {
		t.Errorf("CreateKey json 应创建 ReJSON-RL 键，实际 %q", typ)
	}

	full, err := s.GetFullJSON(connID, 0, "doc:new")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(full, `"x"`) {
		t.Errorf("GetFullJSON 内容不符: %q", full)
	}

	if err := s.CreateKey(connID, 0, "doc:bad", "json", `not-json`, "", 0, 0); err == nil {
		t.Error("CreateKey 非法 JSON 应返回错误")
	}
}

func TestJSONPathOps(t *testing.T) {
	s, seed, connID := newJSONValueService(t)

	jsonSet(t, seed, "doc:path", `{"user":{"name":"alice","tags":["a","b"]},"count":3}`)

	// 读路径
	v, err := s.GetJSONPath(connID, 0, "doc:path", "$.user.name")
	if err != nil {
		t.Fatal(err)
	}
	if v != `"alice"` {
		t.Errorf("$.user.name = %q", v)
	}
	v, err = s.GetJSONPath(connID, 0, "doc:path", "$.user.tags[1]")
	if err != nil {
		t.Fatal(err)
	}
	if v != `"b"` {
		t.Errorf("$.user.tags[1] = %q", v)
	}

	// 路径级写入（对象字段）
	if err := s.SetJSONPath(connID, 0, "doc:path", "$.user.name", `"bob"`); err != nil {
		t.Fatal(err)
	}
	v, _ = s.GetJSONPath(connID, 0, "doc:path", "$.user.name")
	if v != `"bob"` {
		t.Errorf("after set $.user.name = %q", v)
	}

	// 路径级写入（子树替换 + 新字段）
	if err := s.SetJSONPath(connID, 0, "doc:path", "$.user.tags", `["x",1,true]`); err != nil {
		t.Fatal(err)
	}
	if err := s.SetJSONPath(connID, 0, "doc:path", "$.user.age", `30`); err != nil {
		t.Fatal(err)
	}
	full, err := s.GetFullJSON(connID, 0, "doc:path")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"bob"`, `"x"`, `"age"`, `"count"`} {
		if !strings.Contains(full, want) {
			t.Errorf("full doc missing %s: %s", want, full)
		}
	}

	// 非法 JSON 值应报错
	if err := s.SetJSONPath(connID, 0, "doc:path", "$.x", `not-json`); err == nil {
		t.Error("SetJSONPath 非法 JSON 应返回错误")
	}
	// 不存在路径读应报错
	if _, err := s.GetJSONPath(connID, 0, "doc:path", "$.nope.deep"); err == nil {
		t.Error("不存在路径应返回错误")
	}

	// JSON.SET 原地修改不影响 TTL
	if err := seed.Do(tctx, "JSON.SET", "doc:ttl", "$", `{"a":1}`).Err(); err != nil {
		t.Fatal(err)
	}
	seed.Expire(tctx, "doc:ttl", 100*time.Second) //nolint:errcheck
	if err := s.SetJSONPath(connID, 0, "doc:ttl", "$.a", `2`); err != nil {
		t.Fatal(err)
	}
	if ttl := seed.TTL(tctx, "doc:ttl").Val(); ttl <= 0 {
		t.Errorf("SetJSONPath 后 TTL 丢失: %v", ttl)
	}
}

func TestModulesDetected(t *testing.T) {
	m, _, connID := newJSONFixture(t)

	st := m.State(connID)
	if st == nil {
		t.Fatal("连接状态应为已打开")
	}
	joined := strings.Join(st.Modules, ",")
	if !strings.Contains(joined, "ReJSON") {
		t.Errorf("模块列表应包含 ReJSON，实际 %v", st.Modules)
	}
	if !strings.Contains(joined, "search") {
		t.Errorf("模块列表应包含 search，实际 %v", st.Modules)
	}
}

func TestSearchIndexes(t *testing.T) {
	m, seed, connID := newJSONFixture(t)
	ss := NewServerService(m)
	ss.Startup(context.Background())

	// 初始为空
	idx, err := ss.SearchIndexes(connID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx) != 0 {
		t.Fatalf("初始索引应为空，实际 %v", idx)
	}

	// 建一个 JSON 索引后应列出
	jsonSet(t, seed, "doc:idx", `{"a":5}`)
	if err := seed.Do(tctx, "FT.CREATE", "idx:a", "ON", "JSON", "PREFIX", "1", "doc:", "SCHEMA", "$.a", "AS", "a", "NUMERIC").Err(); err != nil {
		t.Fatalf("FT.CREATE: %v", err)
	}
	idx, err = ss.SearchIndexes(connID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx) != 1 || idx[0] != "idx:a" {
		t.Fatalf("应列出 idx:a，实际 %v", idx)
	}
}
