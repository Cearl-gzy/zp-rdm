package service

import (
	"context"

	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
	"zp-rdm/internal/redisx"
)

// KeyService 键管理
type KeyService struct {
	ctx     context.Context
	manager *redisx.Manager
}

func NewKeyService(m *redisx.Manager) *KeyService { return &KeyService{manager: m} }

func (s *KeyService) Startup(ctx context.Context) { s.ctx = ctx }

func (s *KeyService) cli(connID string, db int) (*redis.Client, error) {
	return s.manager.Get(connID, db)
}

// ScanKeys 扫描键（游标分页）
func (s *KeyService) ScanKeys(req model.ScanRequest) (*model.ScanResult, error) {
	c, err := s.cli(req.ConnID, req.DB)
	if err != nil {
		return nil, err
	}
	if req.Count <= 0 {
		req.Count = 500
	}
	if req.Pattern == "" {
		req.Pattern = "*"
	}
	ctx := s.ctx
	var keys []string
	cursor := req.Cursor
	// 迭代扫描直到凑够一页或游标归零，最多 20 轮
	for i := 0; i < 20; i++ {
		var batch []string
		if req.Type != "" {
			batch, cursor, err = c.ScanType(ctx, cursor, req.Pattern, req.Count, req.Type).Result()
			if err != nil { // 老版本不支持 TYPE 选项，退化为扫描后过滤
				batch, cursor, err = c.Scan(ctx, cursor, req.Pattern, req.Count).Result()
			}
		} else {
			batch, cursor, err = c.Scan(ctx, cursor, req.Pattern, req.Count).Result()
		}
		if err != nil {
			return nil, err
		}
		keys = append(keys, batch...)
		if cursor == 0 || len(keys) >= int(req.Count) {
			break
		}
	}

	// 管道取 TYPE / TTL / MEMORY USAGE
	pipe := c.Pipeline()
	typeCmds := make([]*redis.StatusCmd, len(keys))
	ttlCmds := make([]*redis.DurationCmd, len(keys))
	memCmds := make([]*redis.Cmd, len(keys))
	for i, k := range keys {
		typeCmds[i] = pipe.Type(ctx, k)
		ttlCmds[i] = pipe.TTL(ctx, k)
		memCmds[i] = pipe.Do(ctx, "MEMORY", "USAGE", k)
	}
	_, _ = pipe.Exec(ctx)

	infos := make([]model.KeyInfo, 0, len(keys))
	for i, k := range keys {
		t := typeCmds[i].Val()
		if t == "" || t == "none" {
			continue
		}
		if req.Type != "" && t != req.Type {
			continue
		}
		size := int64(-1)
		if v, err := memCmds[i].Int64(); err == nil {
			size = v
		}
		infos = append(infos, model.KeyInfo{Key: k, Type: t, TTL: ttlSeconds(ttlCmds[i].Val()), Size: size})
	}

	total, _ := c.DBSize(ctx).Result()
	return &model.ScanResult{Cursor: cursor, Keys: infos, Total: total}, nil
}

// DeleteKeys 批量删除
func (s *KeyService) DeleteKeys(connID string, db int, keys []string) (int64, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return 0, err
	}
	if len(keys) == 0 {
		return 0, nil
	}
	return c.Del(s.ctx, keys...).Result()
}

// RenameKey 重命名
func (s *KeyService) RenameKey(connID string, db int, key, newKey string) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	return c.Rename(s.ctx, key, newKey).Err()
}

// CopyKey 复制键（Redis 6.2+）
func (s *KeyService) CopyKey(connID string, db int, key, newKey string) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	ok, err := c.Copy(s.ctx, key, newKey, 0, false).Result()
	if err != nil {
		return err
	}
	if ok == 0 {
		return redis.Nil
	}
	return nil
}

// SetTTL 设置过期秒数（-1 移除过期）
func (s *KeyService) SetTTL(connID string, db int, key string, seconds int64) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	if seconds < 0 {
		return c.Persist(s.ctx, key).Err()
	}
	return c.Expire(s.ctx, key, durationSeconds(seconds)).Err()
}

// MemoryUsage 键内存占用
func (s *KeyService) MemoryUsage(connID string, db int, key string) (int64, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return -1, err
	}
	v, err := c.Do(s.ctx, "MEMORY", "USAGE", key).Int64()
	if err != nil {
		return -1, nil
	}
	return v, nil
}

// KeyExists 键是否存在
func (s *KeyService) KeyExists(connID string, db int, key string) (bool, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return false, err
	}
	n, err := c.Exists(s.ctx, key).Result()
	return n > 0, err
}
