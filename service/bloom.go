package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
	"zp-rdm/internal/redisx"
)

// BfService RedisBloom 模块（BF.* 命令族；Cuckoo 见 CLI 原生支持）
type BfService struct {
	ctx     context.Context
	manager *redisx.Manager
}

func NewBfService(m *redisx.Manager) *BfService { return &BfService{manager: m} }

func (s *BfService) Startup(ctx context.Context) { s.ctx = ctx }

func (s *BfService) cli(connID string, db int) (redis.UniversalClient, error) {
	return s.manager.Get(connID, db)
}

// BfInfo BF.INFO 概览（容量/占用/子过滤器数/已插入数/扩容倍率）
func (s *BfService) BfInfo(connID string, db int, key string) (*model.BfInfo, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	raw, err := c.Do(s.ctx, "BF.INFO", key).Result()
	if err != nil {
		return nil, err
	}
	m := toStringAnyMap(raw)
	if len(m) == 0 {
		return nil, fmt.Errorf("BF.INFO 返回为空（模块未加载？）")
	}
	info := &model.BfInfo{}
	for k, v := range m {
		lk := strings.ToLower(k)
		switch {
		case lk == "capacity":
			info.Capacity = toInt64(v)
		case lk == "size":
			info.Size = toInt64(v)
		case strings.Contains(lk, "filter"):
			info.Filters = toInt64(v)
		case strings.Contains(lk, "inserted"):
			info.Inserted = toInt64(v)
		case strings.Contains(lk, "expansion"):
			info.ExpansionRate = toFloat(v)
		}
	}
	return info, nil
}

// BfAdd BF.ADD 添加元素（键不存在时按默认参数自动创建过滤器），返回是否为新增
func (s *BfService) BfAdd(connID string, db int, key, item string) (bool, error) {
	if item == "" {
		return false, fmt.Errorf("元素不能为空")
	}
	c, err := s.cli(connID, db)
	if err != nil {
		return false, err
	}
	return c.Do(s.ctx, "BF.ADD", key, item).Bool()
}

// BfMExists BF.MEXISTS 批量存在性检查（布隆语义：false 一定不存在，true 可能误判）
func (s *BfService) BfMExists(connID string, db int, key string, items []string) ([]bool, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("元素列表为空")
	}
	if len(items) > 100 {
		return nil, fmt.Errorf("单次最多检查 100 个元素")
	}
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	args := make([]interface{}, 0, len(items)+2)
	args = append(args, "BF.MEXISTS", key)
	for _, it := range items {
		args = append(args, it)
	}
	raw, err := c.Do(s.ctx, args...).Result()
	if err != nil {
		return nil, err
	}
	// RESP3 返回布尔数组，RESP2 返回整数数组，双协议兼容
	rows := toSlice(raw)
	res := make([]bool, 0, len(rows))
	for _, r := range rows {
		if b, ok := r.(bool); ok {
			res = append(res, b)
		} else {
			res = append(res, toInt64(r) == 1)
		}
	}
	return res, nil
}
