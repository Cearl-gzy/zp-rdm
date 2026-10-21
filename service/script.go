package service

import (
	"fmt"
	"strings"

	"zp-rdm/internal/model"
)

// Lua 脚本缓存（SCRIPT）与 Redis Functions（Redis 7+）管理。
// SCRIPT LIST 需要 Redis 8.0+，低版本调用会返回服务端错误，由前端提示。

// ScriptList 缓存中的脚本 sha1 列表（SCRIPT LIST，Redis 8+）
func (s *ServerService) ScriptList(connID string, db int) ([]string, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	res, err := c.Do(s.ctx, "SCRIPT", "LIST").StringSlice()
	if err != nil {
		return nil, err
	}
	if res == nil {
		res = []string{}
	}
	return res, nil
}

// ScriptLoad 加载脚本到缓存（SCRIPT LOAD），返回 sha1
func (s *ServerService) ScriptLoad(connID string, db int, code string) (string, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(code) == "" {
		return "", fmt.Errorf("脚本内容为空")
	}
	return c.ScriptLoad(s.ctx, code).Result()
}

// ScriptFlush 清空脚本缓存（SCRIPT FLUSH）
func (s *ServerService) ScriptFlush(connID string, db int) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	return c.Do(s.ctx, "SCRIPT", "FLUSH").Err()
}

// ScriptKill 终止正在执行的脚本（SCRIPT KILL；无脚本在执行时返回服务端错误）
func (s *ServerService) ScriptKill(connID string, db int) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	return c.Do(s.ctx, "SCRIPT", "KILL").Err()
}

// FunctionList 函数库列表（FUNCTION LIST WITHCODE；Redis 7+）
func (s *ServerService) FunctionList(connID string, db int) ([]model.FunctionLibrary, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	res, err := c.Do(s.ctx, "FUNCTION", "LIST", "WITHCODE").Result()
	if err != nil {
		return nil, err
	}
	arr, ok := res.([]interface{})
	if !ok {
		return nil, fmt.Errorf("FUNCTION LIST 返回格式异常: %T", res)
	}
	libs := make([]model.FunctionLibrary, 0, len(arr))
	for _, item := range arr {
		m := toStringAnyMap(item)
		lib := model.FunctionLibrary{
			Name:      strOf(m["library_name"]),
			Engine:    strOf(m["engine"]),
			Code:      strOf(m["library_code"]),
			Functions: []model.FunctionEntry{},
		}
		if fns, ok := m["functions"].([]interface{}); ok {
			for _, f := range fns {
				fm := toStringAnyMap(f)
				entry := model.FunctionEntry{
					Name:  strOf(fm["name"]),
					Desc:  strOf(fm["description"]),
					Flags: []string{},
				}
				if flags, ok := fm["flags"].([]interface{}); ok {
					for _, fl := range flags {
						entry.Flags = append(entry.Flags, strOf(fl))
					}
				}
				lib.Functions = append(lib.Functions, entry)
			}
		}
		libs = append(libs, lib)
	}
	return libs, nil
}

// FunctionStats 正在执行的函数/脚本（FUNCTION STATS；Redis 7+）
func (s *ServerService) FunctionStats(connID string, db int) (*model.FunctionRunStat, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	res, err := c.Do(s.ctx, "FUNCTION", "STATS").Result()
	if err != nil {
		return nil, err
	}
	out := &model.FunctionRunStat{}
	m := toStringAnyMap(res)
	if rs := toStringAnyMap(m["running_script"]); len(rs) > 0 {
		out.Running = true
		out.Name = strOf(rs["name"])
		if cmd, ok := rs["command"].([]interface{}); ok {
			parts := make([]string, 0, len(cmd))
			for _, p := range cmd {
				parts = append(parts, strOf(p))
			}
			out.Command = strings.Join(parts, " ")
		}
		out.DurationMs = toInt64(rs["duration_ms"])
	}
	return out, nil
}

// FunctionLoad 注册函数库（FUNCTION LOAD [REPLACE]），返回库名
func (s *ServerService) FunctionLoad(connID string, db int, code string, replace bool) (string, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(code) == "" {
		return "", fmt.Errorf("函数库代码为空")
	}
	args := []interface{}{"FUNCTION", "LOAD"}
	if replace {
		args = append(args, "REPLACE")
	}
	args = append(args, code)
	return c.Do(s.ctx, args...).Text()
}

// FunctionDelete 删除函数库（FUNCTION DELETE）
func (s *ServerService) FunctionDelete(connID string, db int, lib string) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	return c.Do(s.ctx, "FUNCTION", "DELETE", lib).Err()
}

// FunctionKill 终止正在执行的函数（FUNCTION KILL）
func (s *ServerService) FunctionKill(connID string, db int) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	return c.Do(s.ctx, "FUNCTION", "KILL").Err()
}
