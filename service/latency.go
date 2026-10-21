package service

import (
	"fmt"
	"sort"
	"strconv"

	"zp-rdm/internal/model"
)

// LATENCY 延迟事件监控（Redis 2.8.13+）。事件由 latency-monitor-threshold-ms 触发
// 采样（默认 0 = 关闭，此时 LATEST/HISTORY 均为空）。与 SLOWLOG 互补：慢日志记录
// 单条命令耗时，延迟事件记录命令/IO/fork 等系统级毛刺。go-redis 无封装，走裸 Do()。

// LatencyLatest LATENCY LATEST：全部事件的最近快照（按发生时间倒序）
func (s *ServerService) LatencyLatest(connID string, db int) ([]model.LatencyEvent, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	res, err := c.Do(s.ctx, "LATENCY", "LATEST").Result()
	if err != nil {
		return nil, err
	}
	arr, ok := res.([]interface{})
	if !ok {
		return nil, fmt.Errorf("LATENCY LATEST 返回格式异常: %T", res)
	}
	out := make([]model.LatencyEvent, 0, len(arr))
	for _, item := range arr {
		row, ok := item.([]interface{})
		if !ok || len(row) < 4 {
			continue
		}
		out = append(out, model.LatencyEvent{
			Name:      fmt.Sprint(row[0]),
			Timestamp: toInt64(row[1]),
			LatestMs:  toInt64(row[2]),
			MaxMs:     toInt64(row[3]),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp > out[j].Timestamp })
	return out, nil
}

// LatencyHistory LATENCY HISTORY event：某事件的延迟采样（最多 160 条，时间升序）
func (s *ServerService) LatencyHistory(connID string, db int, event string) ([]model.LatencySample, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	res, err := c.Do(s.ctx, "LATENCY", "HISTORY", event).Result()
	if err != nil {
		return nil, err
	}
	arr, ok := res.([]interface{})
	if !ok {
		return nil, fmt.Errorf("LATENCY HISTORY 返回格式异常: %T", res)
	}
	out := make([]model.LatencySample, 0, len(arr))
	for _, item := range arr {
		row, ok := item.([]interface{})
		if !ok || len(row) < 2 {
			continue
		}
		out = append(out, model.LatencySample{Timestamp: toInt64(row[0]), Ms: toInt64(row[1])})
	}
	return out, nil
}

// toInt64 RESP 整数宽松转换（int64 / float64 / string）
func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	default:
		return 0
	}
}
