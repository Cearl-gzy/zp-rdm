package service

import (
	"context"
	"strings"
	"testing"
)

// RediSearch 集成测试（与 RedisJSON 共用 ZP_RDM_TEST_JSON_ADDR 实例/fixture）

func newSearchServerService(t *testing.T) (*ServerService, string) {
	t.Helper()
	m, seed, connID := newJSONFixture(t)
	s := NewServerService(m)
	s.Startup(context.Background())
	// 确认 search 模块可用（fixture 只探了 ReJSON）
	if err := seed.Do(tctx, "FT._LIST").Err(); err != nil {
		t.Skipf("实例不支持 RediSearch: %v", err)
	}
	return s, connID
}

func TestSearchQueryJSON(t *testing.T) {
	s, connID := newSearchServerService(t)
	m := s.manager
	c, err := m.Get(connID, 0)
	if err != nil {
		t.Fatal(err)
	}

	// ON JSON 索引
	if err := c.Do(tctx, "FT.CREATE", "idx:users", "ON", "JSON",
		"PREFIX", "1", "user:",
		"SCHEMA", "$.name", "AS", "name", "TEXT",
		"$.age", "AS", "age", "NUMERIC",
		"$.city", "AS", "city", "TAG").Err(); err != nil {
		t.Fatalf("FT.CREATE: %v", err)
	}
	jsonSet(t, c, "user:1", `{"name":"alice","age":30,"city":"beijing"}`)
	jsonSet(t, c, "user:2", `{"name":"bob","age":25,"city":"shanghai"}`)
	jsonSet(t, c, "user:3", `{"name":"carol","age":35,"city":"beijing"}`)

	// schema
	fields, err := s.SearchIndexSchema(connID, 0, "idx:users")
	if err != nil {
		t.Fatalf("SearchIndexSchema: %v", err)
	}
	if len(fields) != 3 {
		t.Fatalf("schema fields = %+v", fields)
	}
	byName := map[string]SearchField{}
	for _, f := range fields {
		byName[f.Name] = f
	}
	if byName["name"].Type != "TEXT" || byName["age"].Type != "NUMERIC" || byName["city"].Type != "TAG" {
		t.Fatalf("schema types wrong: %+v", fields)
	}
	if byName["name"].Path != "$.name" {
		t.Fatalf("schema path wrong: %+v", byName["name"])
	}

	// 全文查询
	res, err := s.SearchQuery(connID, 0, "idx:users", "alice", 0, 20)
	if err != nil {
		t.Fatalf("SearchQuery: %v", err)
	}
	if res.Total != 1 || len(res.Docs) != 1 || res.Docs[0].Key != "user:1" {
		t.Fatalf("unexpected result: %+v", res)
	}
	// ON JSON 返回 $ 字段为完整文档
	if !strings.Contains(res.Docs[0].Fields["$"], "alice") {
		t.Fatalf("doc fields missing $ document: %+v", res.Docs[0].Fields)
	}

	// TAG 过滤 + 分页
	res, err = s.SearchQuery(connID, 0, "idx:users", "@city:{beijing}", 0, 1)
	if err != nil {
		t.Fatalf("SearchQuery tag: %v", err)
	}
	if res.Total != 2 || len(res.Docs) != 1 {
		t.Fatalf("tag query paging: total=%d docs=%d", res.Total, len(res.Docs))
	}
	res2, err := s.SearchQuery(connID, 0, "idx:users", "@city:{beijing}", 1, 1)
	if err != nil {
		t.Fatalf("SearchQuery page2: %v", err)
	}
	if res2.Total != 2 || len(res2.Docs) != 1 || res2.Docs[0].Key == res.Docs[0].Key {
		t.Fatalf("page2 wrong: %+v vs %+v", res2.Docs, res.Docs)
	}

	// 数值范围
	res, err = s.SearchQuery(connID, 0, "idx:users", "@age:[26 40]", 0, 20)
	if err != nil {
		t.Fatalf("SearchQuery range: %v", err)
	}
	if res.Total != 2 {
		t.Fatalf("range total = %d, want 2", res.Total)
	}

	// 语法错误应返回错误
	if _, err = s.SearchQuery(connID, 0, "idx:users", "@city:{", 0, 20); err == nil {
		t.Fatal("bad syntax should error")
	}
	// 空查询校验
	if _, err = s.SearchQuery(connID, 0, "idx:users", "  ", 0, 20); err == nil {
		t.Fatal("empty query should error")
	}
}

func TestSearchQueryHash(t *testing.T) {
	s, connID := newSearchServerService(t)
	c, err := s.manager.Get(connID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Do(tctx, "FT.CREATE", "idx:prods", "ON", "HASH",
		"PREFIX", "1", "prod:",
		"SCHEMA", "title", "TEXT", "price", "NUMERIC").Err(); err != nil {
		t.Fatalf("FT.CREATE: %v", err)
	}
	c.HSet(tctx, "prod:1", "title", "redis cookbook", "price", "59") //nolint:errcheck
	c.HSet(tctx, "prod:2", "title", "go cookbook", "price", "49")    //nolint:errcheck

	res, err := s.SearchQuery(connID, 0, "idx:prods", "cookbook", 0, 20)
	if err != nil {
		t.Fatalf("SearchQuery: %v", err)
	}
	if res.Total != 2 {
		t.Fatalf("total = %d, want 2", res.Total)
	}
	// ON HASH 返回逐字段
	found := false
	for _, d := range res.Docs {
		if d.Key == "prod:1" {
			found = true
			if d.Fields["title"] != "redis cookbook" || d.Fields["price"] != "59" {
				t.Fatalf("hash fields wrong: %+v", d.Fields)
			}
		}
	}
	if !found {
		t.Fatalf("prod:1 not in results: %+v", res.Docs)
	}
}
