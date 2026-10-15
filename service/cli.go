package service

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
	"zp-rdm/internal/redisx"
)

// CliService 命令行执行
type CliService struct {
	ctx     context.Context
	manager *redisx.Manager
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
