package service

import (
	"context"
	"strings"
	"testing"
)

// COMMAND DOCS 命令文档测试（Redis 6.2+；miniredis 不支持则探测跳过，真实 Redis 双验证）

func newCliServiceForTest(t *testing.T) (*CliService, string) {
	t.Helper()
	m, _, connID := newTestManager(t)
	s := NewCliService(m)
	s.Startup(context.Background())
	return s, connID
}

// requireCommandDocs 探测 COMMAND DOCS 可用性（Redis < 6.2 报错、miniredis 返回空文档，均跳过）
func requireCommandDocs(t *testing.T, s *CliService, connID string) {
	t.Helper()
	doc, err := s.CommandDocs(connID, 0, "get")
	if err != nil || doc == nil {
		t.Skipf("server does not support COMMAND DOCS: doc=%v err=%v", doc, err)
	}
}

func TestCommandDocs(t *testing.T) {
	s, connID := newCliServiceForTest(t)
	requireCommandDocs(t, s, connID)

	// 简单命令：摘要 + 引入版本 + 用法行
	doc, err := s.CommandDocs(connID, 0, "GET")
	if err != nil {
		t.Fatal(err)
	}
	if doc == nil {
		t.Fatal("GET docs = nil")
	}
	if doc.Summary == "" || doc.Since == "" {
		t.Fatalf("GET doc 缺摘要/版本: %+v", doc)
	}
	if doc.Syntax != "GET key" {
		t.Fatalf("GET syntax = %q, want %q", doc.Syntax, "GET key")
	}

	// 可选参数渲染：SET 应带 [NX | XX] / [EX seconds | ...] 之类的可选块
	setDoc, err := s.CommandDocs(connID, 0, "set")
	if err != nil {
		t.Fatal(err)
	}
	if setDoc == nil || !strings.HasPrefix(setDoc.Syntax, "SET key value") || !strings.Contains(setDoc.Syntax, "[") {
		t.Fatalf("SET syntax 渲染异常: %+v", setDoc)
	}

	// 容器子命令
	subDoc, err := s.CommandDocs(connID, 0, "config|get")
	if err != nil {
		t.Fatal(err)
	}
	if subDoc == nil || !strings.HasPrefix(subDoc.Syntax, "CONFIG GET") {
		t.Fatalf("config|get doc = %+v", subDoc)
	}

	// 半截子命令名：退回容器命令文档（Redis 7+ 容器命令也有摘要），不得报错
	miss, err := s.CommandDocs(connID, 0, "config|gex")
	if err != nil {
		t.Fatal(err)
	}
	if miss != nil && miss.Name != "config" {
		t.Fatalf("config|gex 应退回容器文档 config, got %+v", miss)
	}

	// 未知命令：(nil, nil)
	none, err := s.CommandDocs(connID, 0, "nosuchcmd")
	if err != nil {
		t.Fatal(err)
	}
	if none != nil {
		t.Fatalf("nosuchcmd = %+v, want nil", none)
	}

	// 缓存：第二次同名查询返回同一指针
	doc2, err := s.CommandDocs(connID, 0, "get")
	if err != nil {
		t.Fatal(err)
	}
	if doc2 != doc {
		t.Fatal("第二次查询未命中缓存")
	}
}
