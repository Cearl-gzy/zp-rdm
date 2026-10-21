package service

import (
	"context"
	"path/filepath"
	"testing"

	"zp-rdm/internal/store"
)

// SCRIPT / FUNCTION 管理与 PingRTT 测试。
// SCRIPT LIST 需要 Redis 8+、FUNCTION 需要 Redis 7+，miniredis 探测失败整体跳过。

func requireScriptList(t *testing.T, s *ServerService, connID string) {
	t.Helper()
	if _, err := s.ScriptList(connID, 0); err != nil {
		t.Skipf("server does not support SCRIPT LIST (need Redis 8): %v", err)
	}
}

func requireFunction(t *testing.T, s *ServerService, connID string) {
	t.Helper()
	if _, err := s.FunctionList(connID, 0); err != nil {
		t.Skipf("server does not support FUNCTION (need Redis 7): %v", err)
	}
}

func TestScriptCache(t *testing.T) {
	s, _, connID := newServerService(t)
	requireScriptList(t, s, connID)

	if _, err := s.ScriptLoad(connID, 0, "  "); err == nil {
		t.Fatal("空脚本应报错")
	}

	sha, err := s.ScriptLoad(connID, 0, "return 1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(sha) != 40 {
		t.Fatalf("sha1 = %q, want 40 位", sha)
	}

	list, err := s.ScriptList(connID, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, x := range list {
		if x == sha {
			found = true
		}
	}
	if !found {
		t.Fatalf("SCRIPT LIST 不含刚加载的 %s: %v", sha, list)
	}

	// 无脚本在执行时 KILL 应返回服务端错误（NOTBUSY）
	if err := s.ScriptKill(connID, 0); err == nil {
		t.Fatal("无脚本执行时 SCRIPT KILL 应报错")
	}

	if err := s.ScriptFlush(connID, 0); err != nil {
		t.Fatalf("flush: %v", err)
	}
	list, err = s.ScriptList(connID, 0)
	if err != nil {
		t.Fatalf("list after flush: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("flush 后仍有脚本: %v", list)
	}
}

func TestFunctionManage(t *testing.T) {
	s, _, connID := newServerService(t)
	requireFunction(t, s, connID)

	if _, err := s.FunctionLoad(connID, 0, "", false); err == nil {
		t.Fatal("空代码应报错")
	}

	code := "#!lua name=zplib\nredis.register_function('zpfn', function(keys, args) return 42 end)"
	name, err := s.FunctionLoad(connID, 0, code, false)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if name != "zplib" {
		t.Fatalf("库名 = %q, want zplib", name)
	}

	libs, err := s.FunctionList(connID, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found, hasFn, hasCode := false, false, false
	for _, l := range libs {
		if l.Name == "zplib" {
			found = true
			hasCode = l.Code != ""
			for _, f := range l.Functions {
				if f.Name == "zpfn" {
					hasFn = true
				}
			}
		}
	}
	if !found || !hasFn || !hasCode {
		t.Fatalf("FUNCTION LIST 校验失败 found=%v hasFn=%v hasCode=%v: %+v", found, hasFn, hasCode, libs)
	}

	stat, err := s.FunctionStats(connID, 0)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stat.Running {
		t.Fatalf("不应有运行中的函数: %+v", stat)
	}
	if err := s.FunctionKill(connID, 0); err == nil {
		t.Fatal("无函数执行时 FUNCTION KILL 应报错")
	}

	// 重复注册不 REPLACE 应报错；REPLACE 应成功
	if _, err := s.FunctionLoad(connID, 0, code, false); err == nil {
		t.Fatal("重复注册不带 REPLACE 应报错")
	}
	if _, err := s.FunctionLoad(connID, 0, code, true); err != nil {
		t.Fatalf("REPLACE 注册失败: %v", err)
	}

	if err := s.FunctionDelete(connID, 0, "zplib"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	libs, err = s.FunctionList(connID, 0)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	for _, l := range libs {
		if l.Name == "zplib" {
			t.Fatal("删除后库仍存在")
		}
	}
}

func TestPingRTT(t *testing.T) {
	m, _, connID := newTestManager(t)
	st, err := store.NewAt(filepath.Join(t.TempDir(), "connections.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewConnectionService(st, m)
	s.Startup(context.Background())

	ms, err := s.PingRTT(connID)
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	if ms < 0 {
		t.Fatalf("rtt = %d", ms)
	}

	if _, err := s.PingRTT("no-such-conn"); err == nil {
		t.Fatal("未知连接应报错")
	}
}
