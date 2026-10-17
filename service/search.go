package service

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SearchField 索引 schema 字段
type SearchField struct {
	Name string `json:"name"` // 字段别名（attribute）
	Type string `json:"type"` // TEXT / TAG / NUMERIC / GEO / VECTOR
	Path string `json:"path"` // JSONPath（ON JSON 索引；ON HASH 时同 Name）
}

// SearchDoc 单条搜索结果
type SearchDoc struct {
	Key    string            `json:"key"`
	Fields map[string]string `json:"fields"`
}

// SearchResult FT.SEARCH 结果
type SearchResult struct {
	Total int64       `json:"total"`
	Docs  []SearchDoc `json:"docs"`
}

const (
	searchMaxLimit = 200
	searchTimeout  = 10 * time.Second
)

// RediSearch 回复兼容层：go-redis v9 默认 RESP3（返回 map），
// 旧服务端/显式 RESP2 时返回扁平数组，两种形态都要解析。

func asString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	}
	return ""
}

func asInt64(v interface{}) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case string:
		var n int64
		_, _ = fmt.Sscan(t, &n)
		return n
	}
	return 0
}

// mapGet 兼容 string / interface{} 键的 RESP3 map 取值
func mapGet(m map[interface{}]interface{}, key string) interface{} {
	if v, ok := m[key]; ok {
		return v
	}
	return nil
}

// SearchIndexSchema 获取索引 schema（FT.INFO，兼容 attributes/fields、RESP2/RESP3）
func (s *ServerService) SearchIndexSchema(connID string, db int, index string) ([]SearchField, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(s.ctx, searchTimeout)
	defer cancel()
	res, err := c.Do(ctx, "FT.INFO", index).Result()
	if err != nil {
		return nil, fmt.Errorf("FT.INFO 失败: %w", err)
	}
	var attrs interface{}
	switch info := res.(type) {
	case map[interface{}]interface{}: // RESP3
		if v := mapGet(info, "attributes"); v != nil {
			attrs = v
		} else {
			attrs = mapGet(info, "fields")
		}
	case []interface{}: // RESP2 扁平 kv
		for i := 0; i+1 < len(info); i += 2 {
			name := asString(info[i])
			if name == "attributes" || name == "fields" {
				attrs = info[i+1]
				break
			}
		}
	default:
		return nil, fmt.Errorf("FT.INFO 返回格式异常(%T)", res)
	}
	list, _ := attrs.([]interface{})
	fields := make([]SearchField, 0, len(list))
	for _, a := range list {
		var f SearchField
		switch attr := a.(type) {
		case map[interface{}]interface{}: // RESP3：每个字段是 kv map
			f.Name = asString(mapGet(attr, "attribute"))
			f.Path = asString(mapGet(attr, "identifier"))
			f.Type = asString(mapGet(attr, "type"))
		case []interface{}: // RESP2：扁平 kv 数组
			for i := 0; i+1 < len(attr); i += 2 {
				k, v := asString(attr[i]), asString(attr[i+1])
				switch k {
				case "attribute":
					f.Name = v
				case "identifier":
					f.Path = v
				case "type":
					f.Type = v
				}
			}
		}
		if f.Name == "" {
			f.Name = f.Path
		}
		if f.Name != "" {
			fields = append(fields, f)
		}
	}
	return fields, nil
}

// SearchIndexField 索引向导的字段定义
type SearchIndexField struct {
	Name     string `json:"name"`     // 别名（AS）
	Path     string `json:"path"`     // JSONPath（ON JSON 必填；ON HASH 留空则用 Name 作字段名）
	Type     string `json:"type"`     // TEXT / TAG / NUMERIC / GEO
	Sortable bool   `json:"sortable"` // SORTABLE
}

// SearchCreateIndexRequest FT.CREATE 向导请求
type SearchCreateIndexRequest struct {
	ConnID   string             `json:"connId"`
	DB       int                `json:"db"`
	Name     string             `json:"name"`
	On       string             `json:"on"` // HASH / JSON
	Prefixes []string           `json:"prefixes"`
	Fields   []SearchIndexField `json:"fields"`
}

var searchFieldTypes = map[string]bool{"TEXT": true, "TAG": true, "NUMERIC": true, "GEO": true}

// SearchCreateIndex 建索引向导（FT.CREATE；VECTOR 等高级选项走 CLI）
func (s *ServerService) SearchCreateIndex(req SearchCreateIndexRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	req.On = strings.ToUpper(strings.TrimSpace(req.On))
	if req.Name == "" {
		return fmt.Errorf("索引名不能为空")
	}
	if req.On != "HASH" && req.On != "JSON" {
		return fmt.Errorf("ON 仅支持 HASH / JSON")
	}
	prefixes := make([]string, 0, len(req.Prefixes))
	for _, p := range req.Prefixes {
		if p = strings.TrimSpace(p); p != "" {
			prefixes = append(prefixes, p)
		}
	}
	if len(prefixes) == 0 {
		return fmt.Errorf("至少需要一个键前缀")
	}
	if len(req.Fields) == 0 {
		return fmt.Errorf("至少需要一个字段")
	}

	args := []interface{}{"FT.CREATE", req.Name, "ON", req.On, "PREFIX", len(prefixes)}
	for _, p := range prefixes {
		args = append(args, p)
	}
	args = append(args, "SCHEMA")
	for i, f := range req.Fields {
		f.Name = strings.TrimSpace(f.Name)
		f.Path = strings.TrimSpace(f.Path)
		f.Type = strings.ToUpper(strings.TrimSpace(f.Type))
		if f.Name == "" {
			return fmt.Errorf("字段 %d 缺少别名", i+1)
		}
		if !searchFieldTypes[f.Type] {
			return fmt.Errorf("字段 %s 类型 %q 不支持（TEXT/TAG/NUMERIC/GEO）", f.Name, f.Type)
		}
		if req.On == "JSON" {
			if !strings.HasPrefix(f.Path, "$") {
				return fmt.Errorf("字段 %s 需要 JSONPath（以 $ 开头）", f.Name)
			}
			args = append(args, f.Path, "AS", f.Name, f.Type)
		} else {
			field := f.Name
			if f.Path != "" {
				field = f.Path // ON HASH 也允许「字段名 AS 别名」
			}
			args = append(args, field, "AS", f.Name, f.Type)
		}
		if f.Sortable {
			args = append(args, "SORTABLE")
		}
	}

	c, err := s.cli(req.ConnID, req.DB)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(s.ctx, searchTimeout)
	defer cancel()
	if err := c.Do(ctx, args...).Err(); err != nil {
		return fmt.Errorf("FT.CREATE 失败: %w", err)
	}
	return nil
}

// SearchDropIndex 删除索引（FT.DROPINDEX；deleteDocs 为 true 时带 DD 连同文档一起删）
func (s *ServerService) SearchDropIndex(connID string, db int, index string, deleteDocs bool) error {
	index = strings.TrimSpace(index)
	if index == "" {
		return fmt.Errorf("索引名不能为空")
	}
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	args := []interface{}{"FT.DROPINDEX", index}
	if deleteDocs {
		args = append(args, "DD")
	}
	ctx, cancel := context.WithTimeout(s.ctx, searchTimeout)
	defer cancel()
	if err := c.Do(ctx, args...).Err(); err != nil {
		return fmt.Errorf("FT.DROPINDEX 失败: %w", err)
	}
	return nil
}

// SearchAggReducer FT.AGGREGATE 聚合器
type SearchAggReducer struct {
	Func  string `json:"func"`  // COUNT / COUNT_DISTINCT / SUM / AVG / MIN / MAX
	Field string `json:"field"` // COUNT 无需字段
	Alias string `json:"alias"` // 留空自动生成
}

// SearchAggregateRequest 聚合查询请求（GROUPBY + REDUCE + LIMIT 子集）
type SearchAggregateRequest struct {
	ConnID   string             `json:"connId"`
	DB       int                `json:"db"`
	Index    string             `json:"index"`
	Query    string             `json:"query"` // 空按 * 处理
	GroupBy  []string           `json:"groupBy"`
	Reducers []SearchAggReducer `json:"reducers"`
	Limit    int                `json:"limit"`
}

// SearchAggregateResult FT.AGGREGATE 结果（行为动态列）
type SearchAggregateResult struct {
	Total int64               `json:"total"`
	Rows  []map[string]string `json:"rows"`
}

const aggMaxLimit = 1000

// reducer 需要的字段参数个数；-1 表示不支持
var aggReducerNargs = map[string]int{
	"COUNT": 0, "COUNT_DISTINCT": 1, "SUM": 1, "AVG": 1, "MIN": 1, "MAX": 1,
}

// SearchAggregate 执行 FT.AGGREGATE（结构化构造 GROUPBY/REDUCE，原生命令可走 CLI）
func (s *ServerService) SearchAggregate(req SearchAggregateRequest) (*SearchAggregateResult, error) {
	req.Index = strings.TrimSpace(req.Index)
	if req.Index == "" {
		return nil, fmt.Errorf("索引不能为空")
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		query = "*"
	}
	if req.Limit <= 0 || req.Limit > aggMaxLimit {
		req.Limit = 100
	}

	args := []interface{}{"FT.AGGREGATE", req.Index, query}
	if len(req.GroupBy) > 0 || len(req.Reducers) > 0 {
		groupBy := make([]string, 0, len(req.GroupBy))
		for _, g := range req.GroupBy {
			if g = strings.TrimSpace(g); g != "" {
				groupBy = append(groupBy, g)
			}
		}
		args = append(args, "GROUPBY", len(groupBy))
		for _, g := range groupBy {
			args = append(args, "@"+strings.TrimPrefix(g, "@"))
		}
		for i, r := range req.Reducers {
			r.Func = strings.ToUpper(strings.TrimSpace(r.Func))
			r.Field = strings.TrimSpace(r.Field)
			nargs, ok := aggReducerNargs[r.Func]
			if !ok {
				return nil, fmt.Errorf("聚合器 %d 不支持 %q", i+1, r.Func)
			}
			if nargs == 1 && r.Field == "" {
				return nil, fmt.Errorf("聚合器 %s 需要选择字段", r.Func)
			}
			alias := strings.TrimSpace(r.Alias)
			if alias == "" {
				alias = strings.ToLower(r.Func)
				if r.Field != "" {
					alias += "_" + strings.TrimPrefix(r.Field, "@")
				}
			}
			args = append(args, "REDUCE", r.Func, nargs)
			if nargs == 1 {
				args = append(args, "@"+strings.TrimPrefix(r.Field, "@"))
			}
			args = append(args, "AS", alias)
		}
	}
	args = append(args, "LIMIT", 0, req.Limit)

	c, err := s.cli(req.ConnID, req.DB)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(s.ctx, searchTimeout)
	defer cancel()
	res, err := c.Do(ctx, args...).Result()
	if err != nil {
		return nil, fmt.Errorf("FT.AGGREGATE 失败: %w", err)
	}

	out := &SearchAggregateResult{Rows: []map[string]string{}}
	switch r := res.(type) {
	case map[interface{}]interface{}: // RESP3：行在 results[].extra_attributes
		out.Total = asInt64(mapGet(r, "total_results"))
		results, _ := mapGet(r, "results").([]interface{})
		for _, item := range results {
			m, ok := item.(map[interface{}]interface{})
			if !ok {
				continue
			}
			row := map[string]string{}
			if attrs, ok := mapGet(m, "extra_attributes").(map[interface{}]interface{}); ok {
				for k, v := range attrs {
					row[asString(k)] = asString(v)
				}
			}
			out.Rows = append(out.Rows, row)
		}
	case []interface{}: // RESP2：[total, [k,v,...], ...]
		if len(r) == 0 {
			return nil, fmt.Errorf("FT.AGGREGATE 返回格式异常")
		}
		out.Total = asInt64(r[0])
		for _, item := range r[1:] {
			pairs, ok := item.([]interface{})
			if !ok {
				continue
			}
			row := map[string]string{}
			for j := 0; j+1 < len(pairs); j += 2 {
				row[asString(pairs[j])] = asString(pairs[j+1])
			}
			out.Rows = append(out.Rows, row)
		}
	default:
		return nil, fmt.Errorf("FT.AGGREGATE 返回格式异常(%T)", res)
	}
	return out, nil
}

// SearchQuery 执行 FT.SEARCH（query 为 RediSearch 查询语法，原样透传）
func (s *ServerService) SearchQuery(connID string, db int, index, query string, offset, limit int) (*SearchResult, error) {
	index = strings.TrimSpace(index)
	query = strings.TrimSpace(query)
	if index == "" || query == "" {
		return nil, fmt.Errorf("索引与查询语句不能为空")
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > searchMaxLimit {
		limit = 20
	}
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(s.ctx, searchTimeout)
	defer cancel()
	res, err := c.Do(ctx, "FT.SEARCH", index, query, "LIMIT", offset, limit).Result()
	if err != nil {
		return nil, fmt.Errorf("FT.SEARCH 失败: %w", err)
	}
	out := &SearchResult{Docs: []SearchDoc{}}
	switch r := res.(type) {
	case map[interface{}]interface{}: // RESP3
		out.Total = asInt64(mapGet(r, "total_results"))
		results, _ := mapGet(r, "results").([]interface{})
		for _, item := range results {
			m, ok := item.(map[interface{}]interface{})
			if !ok {
				continue
			}
			doc := SearchDoc{Key: asString(mapGet(m, "id")), Fields: map[string]string{}}
			if attrs, ok := mapGet(m, "extra_attributes").(map[interface{}]interface{}); ok {
				for k, v := range attrs {
					doc.Fields[asString(k)] = asString(v)
				}
			}
			out.Docs = append(out.Docs, doc)
		}
	case []interface{}: // RESP2：[total, key1, [f,v,...], key2, ...]
		if len(r) == 0 {
			return nil, fmt.Errorf("FT.SEARCH 返回格式异常")
		}
		out.Total = asInt64(r[0])
		for i := 1; i+1 < len(r); i += 2 {
			doc := SearchDoc{Key: asString(r[i]), Fields: map[string]string{}}
			if pairs, ok := r[i+1].([]interface{}); ok {
				for j := 0; j+1 < len(pairs); j += 2 {
					doc.Fields[asString(pairs[j])] = asString(pairs[j+1])
				}
			}
			out.Docs = append(out.Docs, doc)
		}
	default:
		return nil, fmt.Errorf("FT.SEARCH 返回格式异常(%T)", res)
	}
	return out, nil
}
