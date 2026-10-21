package service

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
	"zp-rdm/internal/redisx"
)

// TsService Redis TimeSeries 模块（TS.* 命令族）
type TsService struct {
	ctx     context.Context
	manager *redisx.Manager
}

func NewTsService(m *redisx.Manager) *TsService { return &TsService{manager: m} }

func (s *TsService) Startup(ctx context.Context) { s.ctx = ctx }

func (s *TsService) cli(connID string, db int) (redis.UniversalClient, error) {
	return s.manager.Get(connID, db)
}

// TsInfo TS.INFO 概览（样本数/内存/时间范围/保留期/标签/降采样规则）
func (s *TsService) TsInfo(connID string, db int, key string) (*model.TsInfo, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	raw, err := c.Do(s.ctx, "TS.INFO", key).Result()
	if err != nil {
		return nil, err
	}
	m := toStringAnyMap(raw)
	if len(m) == 0 {
		return nil, fmt.Errorf("TS.INFO 返回为空（模块未加载？）")
	}
	info := &model.TsInfo{
		TotalSamples:   toInt64(m["totalSamples"]),
		MemoryUsage:    toInt64(m["memoryUsage"]),
		FirstTimestamp: toInt64(m["firstTimestamp"]),
		LastTimestamp:  toInt64(m["lastTimestamp"]),
		RetentionMs:    toInt64(m["retentionTime"]),
		ChunkCount:     toInt64(m["chunkCount"]),
		Labels:         map[string]string{},
	}
	// labels：RESP3 为 map，RESP2 为 [[k,v]...] 嵌套数组，按类型分派
	switch lv := m["labels"].(type) {
	case map[interface{}]interface{}, map[string]interface{}:
		for k, v := range toStringAnyMap(lv) {
			info.Labels[k] = toStr(v)
		}
	default:
		for _, kv := range toSlice(lv) {
			pair := toSlice(kv)
			if len(pair) >= 2 {
				info.Labels[toStr(pair[0])] = toStr(pair[1])
			}
		}
	}
	// rules：RESP3 为 {destKey: [bucketMs, agg, align]} map，RESP2 为 [[destKey, bucketMs, agg]...] 数组
	switch rv := m["rules"].(type) {
	case map[interface{}]interface{}, map[string]interface{}:
		for dest, v := range toStringAnyMap(rv) {
			parts := toSlice(v)
			if len(parts) >= 2 {
				info.Rules = append(info.Rules, model.TsRule{
					DestKey:    dest,
					BucketMs:   toInt64(parts[0]),
					Aggregator: toStr(parts[1]),
				})
			}
		}
	default:
		for _, elem := range toSlice(rv) {
			rule := toSlice(elem)
			if len(rule) >= 3 {
				info.Rules = append(info.Rules, model.TsRule{
					DestKey:    toStr(rule[0]),
					BucketMs:   toInt64(rule[1]),
					Aggregator: toStr(rule[2]),
				})
			}
		}
	}
	return info, nil
}

// TsRange TS.RANGE 区间查询（from<=0 → "-" 序列起点，to<=0 → "+" 序列终点）；
// agg 非空时附加 AGGREGATION agg bucketMs；count<=0 不限制（上限 5000）
func (s *TsService) TsRange(connID string, db int, key string, from, to int64, agg string, bucketMs, count int64) ([]model.TsSample, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	var fromArg, toArg interface{} = from, to
	if from <= 0 {
		fromArg = "-"
	}
	if to <= 0 {
		toArg = "+"
	}
	args := []interface{}{"TS.RANGE", key, fromArg, toArg}
	if agg != "" {
		if bucketMs <= 0 {
			return nil, fmt.Errorf("聚合查询需要正数的 bucket 毫秒数")
		}
		args = append(args, "AGGREGATION", agg, bucketMs)
	}
	if count <= 0 || count > 5000 {
		count = 5000
	}
	args = append(args, "COUNT", count)
	raw, err := c.Do(s.ctx, args...).Result()
	if err != nil {
		return nil, err
	}
	samples := make([]model.TsSample, 0, 64)
	for _, row := range toSlice(raw) {
		pair := toSlice(row)
		if len(pair) >= 2 {
			samples = append(samples, model.TsSample{Ts: toInt64(pair[0]), Value: toFloat(pair[1])})
		}
	}
	return samples, nil
}

// TsAdd TS.ADD 追加样本；ts<=0 用服务端当前时间（*）。键不存在时模块自动创建序列。
func (s *TsService) TsAdd(connID string, db int, key string, ts int64, value float64) (int64, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return 0, err
	}
	var tsArg interface{} = "*"
	if ts > 0 {
		tsArg = ts
	}
	return c.Do(s.ctx, "TS.ADD", key, tsArg, value).Int64()
}

// ---------- 解析助手（复用 cli.go 的 toStringAnyMap 风格） ----------

func toSlice(v interface{}) []interface{} {
	if s, ok := v.([]interface{}); ok {
		return s
	}
	return nil
}

func toStr(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return fmt.Sprintf("%v", v)
	}
}
