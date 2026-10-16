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
