package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
	"zp-rdm/internal/redisx"
)

// CliService 命令行执行
type CliService struct {
	ctx     context.Context
	manager *redisx.Manager

	docsMu    sync.Mutex
	docsCache map[string]map[string]*model.CommandDoc // connID → 命令名 → 文档（nil = 未找到）
}

func NewCliService(m *redisx.Manager) *CliService { return &CliService{manager: m} }

func (s *CliService) Startup(ctx context.Context) { s.ctx = ctx }

// ExecCommand 执行任意 Redis 命令
func (s *CliService) ExecCommand(connID string, db int, args []string) (*model.CliResult, error) {
	if len(args) == 0 {
		return &model.CliResult{OK: false, Type: "error", Value: "空命令"}, nil
	}
	c, err := s.manager.Get(connID, db)
	if err != nil {
		return nil, err
	}
	ifaces := make([]interface{}, len(args))
	for i, a := range args {
		ifaces[i] = a
	}
	start := time.Now()
	val, err := c.Do(s.ctx, ifaces...).Result()
	res := &model.CliResult{CostMs: time.Since(start).Milliseconds()}
	if err != nil {
		if err == redis.Nil {
			res.OK, res.Type, res.Value = true, "nil", nil
			return res, nil
		}
		res.OK, res.Type, res.Value = false, "error", err.Error()
		return res, nil
	}
	res.OK = true
	res.Type, res.Value = normalize(val)
	return res, nil
}

// normalize 将 go-redis 返回值转为 JSON 友好结构
func normalize(v interface{}) (string, interface{}) {
	switch t := v.(type) {
	case nil:
		return "nil", nil
	case string:
		return "bulk", t
	case int64:
		return "integer", t
	case float64:
		return "bulk", strconvFloat(t)
	case bool:
		return "status", t
	case []interface{}:
		arr := make([]interface{}, len(t))
		for i, item := range t {
			_, arr[i] = normalize(item)
		}
		return "array", arr
	case map[interface{}]interface{}:
		arr := make([]interface{}, 0, len(t)*2)
		for k, val := range t {
			_, nv := normalize(val)
			arr = append(arr, fmt.Sprintf("%v", k), nv)
		}
		return "array", arr
	default:
		return "bulk", fmt.Sprintf("%v", t)
	}
}

func strconvFloat(f float64) string {
	return fmt.Sprintf("%v", f)
}

// ---------- COMMAND DOCS 命令文档（Redis 6.2+） ----------

// CommandDocs 查询命令文档（COMMAND DOCS）。name 形如 "set" 或 "config|get"（容器命令
// 子命令）。未找到 / 无摘要（容器命令本身）时返回 (nil, nil)。结果按连接缓存——文档
// 来自服务器自身，版本始终匹配。
func (s *CliService) CommandDocs(connID string, db int, name string) (*model.CommandDoc, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return nil, nil
	}
	s.docsMu.Lock()
	if s.docsCache == nil {
		s.docsCache = make(map[string]map[string]*model.CommandDoc)
	}
	cache := s.docsCache[connID]
	if cache == nil {
		cache = make(map[string]*model.CommandDoc)
		s.docsCache[connID] = cache
	}
	if doc, ok := cache[name]; ok {
		s.docsMu.Unlock()
		return doc, nil
	}
	s.docsMu.Unlock()

	c, err := s.manager.Get(connID, db)
	if err != nil {
		return nil, err
	}
	doc, err := s.fetchCommandDocs(c, name)
	if err == nil && doc == nil {
		if i := strings.IndexByte(name, '|'); i > 0 {
			// 子命令未命中（半截子命令名），退回容器命令——容器无摘要时同样返回 nil
			doc, err = s.fetchCommandDocs(c, name[:i])
		}
	}
	if err != nil {
		return nil, err
	}
	s.docsMu.Lock()
	cache[name] = doc
	s.docsMu.Unlock()
	return doc, nil
}

// fetchCommandDocs 拉取并解析单条命令文档；命令不存在或无摘要返回 (nil, nil)
func (s *CliService) fetchCommandDocs(c redis.UniversalClient, name string) (*model.CommandDoc, error) {
	res, err := c.Do(s.ctx, "COMMAND", "DOCS", name).Result()
	if err != nil {
		return nil, err
	}
	entry, ok := toStringAnyMap(res)[name]
	if !ok {
		return nil, nil
	}
	m := toStringAnyMap(entry)
	summary, _ := m["summary"].(string)
	if summary == "" {
		return nil, nil // 容器命令（CONFIG/CLIENT…）无摘要，提示无意义
	}
	doc := &model.CommandDoc{
		Name:       name,
		Summary:    summary,
		Since:      strOf(m["since"]),
		Complexity: strOf(m["complexity"]),
		Syntax:     strings.ToUpper(strings.Replace(name, "|", " ", 1)),
	}
	args, _ := m["arguments"].([]interface{})
	parts := make([]string, 0, len(args))
	for _, a := range args {
		if seg := renderDocArg(toStringAnyMap(a)); seg != "" {
			parts = append(parts, seg)
		}
	}
	if len(parts) > 0 {
		doc.Syntax += " " + strings.Join(parts, " ")
	}
	return doc, nil
}

// toStringAnyMap RESP 回复宽松转 string 键 map（RESP3 map / RESP2 扁平 kv 数组）
func toStringAnyMap(v interface{}) map[string]interface{} {
	out := make(map[string]interface{})
	switch m := v.(type) {
	case map[string]interface{}:
		for k, val := range m {
			out[k] = val
		}
	case map[interface{}]interface{}:
		for k, val := range m {
			out[fmt.Sprint(k)] = val
		}
	case []interface{}:
		for i := 0; i+1 < len(m); i += 2 {
			out[fmt.Sprint(m[i])] = m[i+1]
		}
	}
	return out
}

// renderDocArg 把 COMMAND DOCS 的 argument 描述渲染成 redis-cli 风格用法片段
func renderDocArg(arg map[string]interface{}) string {
	name := strOf(arg["name"])
	token := strOf(arg["token"])

	var core string
	switch typ := strOf(arg["type"]); typ {
	case "oneof", "block":
		sub, _ := arg["arguments"].([]interface{})
		parts := make([]string, 0, len(sub))
		for _, sa := range sub {
			if seg := renderDocArg(toStringAnyMap(sa)); seg != "" {
				parts = append(parts, seg)
			}
		}
		sep := " "
		if typ == "oneof" {
			sep = " | "
		}
		core = strings.Join(parts, sep)
	case "pure-token":
		core = token
	default: // string / integer / double / key / pattern / unix-time / token
		if token != "" {
			core = token
			if name != "" {
				core += " " + name
			}
		} else {
			core = name
		}
	}
	if core == "" {
		return ""
	}
	if hasDocFlag(arg, "multiple") {
		core += " [" + core + " ...]"
	}
	if hasDocFlag(arg, "optional") {
		core = "[" + core + "]"
	}
	return core
}

func hasDocFlag(arg map[string]interface{}, flag string) bool {
	flags, _ := arg["flags"].([]interface{})
	for _, f := range flags {
		if strOf(f) == flag {
			return true
		}
	}
	return false
}

func strOf(v interface{}) string {
	s, _ := v.(string)
	return s
}
