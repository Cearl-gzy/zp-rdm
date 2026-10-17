package service

import (
	"strings"
	"testing"
)

// FT.CREATE 向导 / FT.DROPINDEX / FT.AGGREGATE 集成测试
//（与 RediSearch 查询测试共用 ZP_RDM_TEST_JSON_ADDR fixture）

func TestSearchCreateIndexJSON(t *testing.T) {
	s, connID := newSearchServerService(t)
	c, err := s.manager.Get(connID, 0)
	if err != nil {
		t.Fatal(err)
	}

	err = s.SearchCreateIndex(SearchCreateIndexRequest{
		ConnID: connID, DB: 0,
		Name: "idx:users2", On: "JSON", Prefixes: []string{"user2:"},
		Fields: []SearchIndexField{
			{Name: "name", Path: "$.name", Type: "TEXT"},
			{Name: "age", Path: "$.age", Type: "NUMERIC", Sortable: true},
			{Name: "city", Path: "$.city", Type: "TAG"},
		},
	})
	if err != nil {
		t.Fatalf("SearchCreateIndex: %v", err)
	}

	// 索引应出现在列表且 schema 正确
	idx, err := s.SearchIndexes(connID, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, i := range idx {
		if i == "idx:users2" {
			found = true
		}
	}
	if !found {
		t.Fatalf("索引未出现在 FT._LIST: %v", idx)
	}
	fields, err := s.SearchIndexSchema(connID, 0, "idx:users2")
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 3 {
		t.Fatalf("schema fields = %+v", fields)
	}

	// 写入文档可检索
	jsonSet(t, c, "user2:1", `{"name":"dave","age":40,"city":"hangzhou"}`)
	res, err := s.SearchQuery(connID, 0, "idx:users2", "dave", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 {
		t.Fatalf("total = %d, want 1", res.Total)
	}
}

func TestSearchCreateIndexHashAndValidation(t *testing.T) {
	s, connID := newSearchServerService(t)

	// ON HASH + 多前缀 + 字段名 AS 别名
	err := s.SearchCreateIndex(SearchCreateIndexRequest{
		ConnID: connID, DB: 0,
		Name: "idx:prods2", On: "HASH", Prefixes: []string{"prod2:", "sku:"},
		Fields: []SearchIndexField{
			{Name: "title", Type: "TEXT"},
			{Name: "price", Type: "NUMERIC", Sortable: true},
		},
	})
	if err != nil {
		t.Fatalf("create HASH index: %v", err)
	}
	fields, err := s.SearchIndexSchema(connID, 0, "idx:prods2")
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 {
		t.Fatalf("schema fields = %+v", fields)
	}

	// 校验错误
	cases := []SearchCreateIndexRequest{
		{ConnID: connID, Name: " ", On: "JSON", Prefixes: []string{"a:"}, Fields: []SearchIndexField{{Name: "x", Path: "$.x", Type: "TEXT"}}},
		{ConnID: connID, Name: "x1", On: "JSON", Prefixes: nil, Fields: []SearchIndexField{{Name: "x", Path: "$.x", Type: "TEXT"}}},
		{ConnID: connID, Name: "x2", On: "JSON", Prefixes: []string{"a:"}, Fields: nil},
		{ConnID: connID, Name: "x3", On: "JSON", Prefixes: []string{"a:"}, Fields: []SearchIndexField{{Name: "x", Path: "name", Type: "TEXT"}}},
		{ConnID: connID, Name: "x4", On: "JSON", Prefixes: []string{"a:"}, Fields: []SearchIndexField{{Name: "x", Path: "$.x", Type: "VECTOR"}}},
		{ConnID: connID, Name: "x5", On: "JSON", Prefixes: []string{"a:"}, Fields: []SearchIndexField{{Name: "", Path: "$.x", Type: "TEXT"}}},
		{ConnID: connID, Name: "x6", On: "SQL", Prefixes: []string{"a:"}, Fields: []SearchIndexField{{Name: "x", Path: "$.x", Type: "TEXT"}}},
	}
	for i, req := range cases {
		if err := s.SearchCreateIndex(req); err == nil {
			t.Errorf("case %d 应校验失败", i)
		}
	}
}

func TestSearchDropIndex(t *testing.T) {
	s, connID := newSearchServerService(t)
	c, err := s.manager.Get(connID, 0)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.SearchCreateIndex(SearchCreateIndexRequest{
		ConnID: connID, DB: 0,
		Name: "idx:drop", On: "HASH", Prefixes: []string{"drop:"},
		Fields: []SearchIndexField{{Name: "v", Type: "NUMERIC"}},
	}); err != nil {
		t.Fatal(err)
	}
	c.HSet(tctx, "drop:1", "v", "1") //nolint:errcheck

	// 不带 DD：文档保留
	if err := s.SearchDropIndex(connID, 0, "idx:drop", false); err != nil {
		t.Fatal(err)
	}
	if got := c.Exists(tctx, "drop:1").Val(); got != 1 {
		t.Fatalf("不带 DD 文档应保留，exists=%d", got)
	}

	// 重建后带 DD：文档连删
	if err := s.SearchCreateIndex(SearchCreateIndexRequest{
		ConnID: connID, DB: 0,
		Name: "idx:drop", On: "HASH", Prefixes: []string{"drop:"},
		Fields: []SearchIndexField{{Name: "v", Type: "NUMERIC"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.SearchDropIndex(connID, 0, "idx:drop", true); err != nil {
		t.Fatal(err)
	}
	if got := c.Exists(tctx, "drop:1").Val(); got != 0 {
		t.Fatalf("带 DD 文档应删除，exists=%d", got)
	}
	// 索引已不在列表
	idx, _ := s.SearchIndexes(connID, 0)
	for _, i := range idx {
		if i == "idx:drop" {
			t.Fatal("索引应已从 FT._LIST 移除")
		}
	}
	// 空名校验
	if err := s.SearchDropIndex(connID, 0, " ", false); err == nil {
		t.Fatal("空索引名应报错")
	}
}

func TestSearchAggregate(t *testing.T) {
	s, connID := newSearchServerService(t)
	c, err := s.manager.Get(connID, 0)
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Do(tctx, "FT.CREATE", "idx:sales", "ON", "HASH",
		"PREFIX", "1", "sale:",
		"SCHEMA", "region", "TAG", "amount", "NUMERIC").Err(); err != nil {
		t.Fatalf("FT.CREATE: %v", err)
	}
	rows := []struct {
		region string
		amount int
	}{
		{"north", 10}, {"north", 20}, {"south", 5}, {"south", 15}, {"south", 30}, {"east", 7},
	}
	for i, r := range rows {
		c.HSet(tctx, "sale:"+string(rune('a'+i)), "region", r.region, "amount", r.amount) //nolint:errcheck
	}

	// GROUPBY region + COUNT/SUM/AVG/MAX
	res, err := s.SearchAggregate(SearchAggregateRequest{
		ConnID: connID, DB: 0,
		Index: "idx:sales", Query: "*",
		GroupBy: []string{"region"},
		Reducers: []SearchAggReducer{
			{Func: "COUNT", Alias: "n"},
			{Func: "SUM", Field: "amount", Alias: "total"},
			{Func: "AVG", Field: "amount"},
			{Func: "MAX", Field: "amount", Alias: "peak"},
		},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchAggregate: %v", err)
	}
	if res.Total != 3 || len(res.Rows) != 3 {
		t.Fatalf("rows = %+v (total %d)", res.Rows, res.Total)
	}
	byRegion := map[string]map[string]string{}
	for _, r := range res.Rows {
		byRegion[r["region"]] = r
	}
	north := byRegion["north"]
	if north == nil || north["n"] != "2" || north["total"] != "30" || north["peak"] != "20" {
		t.Fatalf("north row wrong: %+v", north)
	}
	south := byRegion["south"]
	if south == nil || south["n"] != "3" || south["total"] != "50" {
		t.Fatalf("south row wrong: %+v", south)
	}
	if south["avg_amount"] != "16.6666666667" && !strings.HasPrefix(south["avg_amount"], "16.6") {
		t.Fatalf("south avg wrong: %+v", south)
	}

	// 无 GROUPBY（GROUPBY 0）：全量聚合
	res, err = s.SearchAggregate(SearchAggregateRequest{
		ConnID: connID, DB: 0,
		Index: "idx:sales", Query: "@region:{north}",
		Reducers: []SearchAggReducer{{Func: "COUNT"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 || res.Rows[0]["count"] != "2" {
		t.Fatalf("global count wrong: %+v", res.Rows)
	}

	// 校验错误
	if _, err = s.SearchAggregate(SearchAggregateRequest{ConnID: connID, Index: " "}); err == nil {
		t.Fatal("空索引应报错")
	}
	if _, err = s.SearchAggregate(SearchAggregateRequest{
		ConnID: connID, Index: "idx:sales",
		Reducers: []SearchAggReducer{{Func: "MEDIAN", Field: "amount"}},
	}); err == nil {
		t.Fatal("不支持的聚合器应报错")
	}
	if _, err = s.SearchAggregate(SearchAggregateRequest{
		ConnID: connID, Index: "idx:sales",
		Reducers: []SearchAggReducer{{Func: "SUM"}},
	}); err == nil {
		t.Fatal("SUM 缺字段应报错")
	}
}
