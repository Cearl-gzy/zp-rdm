package service

import (
	"fmt"
)

// 内存医生：MEMORY STATS / DOCTOR / PURGE（Redis 4.0+）

// MemoryStats 服务器内存统计（MEMORY STATS，键值原样透传，数值为 float64/字符串混合）
func (s *ServerService) MemoryStats(connID string, db int) (map[string]interface{}, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	res, err := c.Do(s.ctx, "MEMORY", "STATS").Result()
	if err != nil {
		return nil, err
	}
	m := toStringAnyMap(res)
	if len(m) == 0 {
		return nil, fmt.Errorf("MEMORY STATS 返回为空")
	}
	return m, nil
}

// MemoryDoctor 内存诊断文本（MEMORY DOCTOR；健康时返回提示语，异常时返回问题清单）
func (s *ServerService) MemoryDoctor(connID string, db int) (string, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return "", err
	}
	return c.Do(s.ctx, "MEMORY", "DOCTOR").Text()
}

// MemoryPurge 尝试归还 allocator 持有的脏页（MEMORY PURGE；非 jemalloc 平台可能报错）
func (s *ServerService) MemoryPurge(connID string, db int) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	return c.Do(s.ctx, "MEMORY", "PURGE").Err()
}

// MemoryInfoSection INFO memory 段常用字段（供内存面板卡片与诊断报告）
func (s *ServerService) MemoryInfoSection(connID string, db int) (map[string]string, error) {
	info, err := s.GetInfo(connID, db, []string{"memory"})
	if err != nil {
		return nil, err
	}
	sec := info["memory"]
	if sec == nil {
		return map[string]string{}, nil
	}
	return sec, nil
}
