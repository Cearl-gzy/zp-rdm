package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
	"zp-rdm/internal/redisx"
)

const maxStringRead = 1024 * 1024  // 字符串读取上限 1MB（预览）
const maxFullStringRead = 32 << 20 // 全量读取上限 32MB

// ValueService 键值读写
type ValueService struct {
	ctx     context.Context
	manager *redisx.Manager
}

func NewValueService(m *redisx.Manager) *ValueService { return &ValueService{manager: m} }

func (s *ValueService) Startup(ctx context.Context) { s.ctx = ctx }

func (s *ValueService) cli(connID string, db int) (*redis.Client, error) {
	return s.manager.Get(connID, db)
}

// GetValue 获取键概览（string 含内容，集合含长度）
func (s *ValueService) GetValue(connID string, db int, key string) (*model.ValueDetail, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	ctx := s.ctx
	t, err := c.Type(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if t == "none" {
		return nil, redis.Nil
	}
	d := &model.ValueDetail{Key: key, Type: t, Size: -1}
	ttl, _ := c.TTL(ctx, key).Result()
	d.TTL = ttlSeconds(ttl)
	if enc, err := c.ObjectEncoding(ctx, key).Result(); err == nil {
		d.Encoding = enc
	}
	if v, err := c.Do(ctx, "MEMORY", "USAGE", key).Int64(); err == nil {
		d.Size = v
	}

	switch t {
	case "string":
		v, err := c.Get(ctx, key).Result()
		if err != nil {
			return nil, err
		}
		if len(v) > maxStringRead {
			d.StrValue = v[:maxStringRead]
			d.Truncated = true
		} else {
			d.StrValue = v
		}
		d.Binary = isBinaryString(v)
		d.Length = int64(len(v))
	case "ReJSON-RL":
		// RedisJSON 文档：整体取出，前端格式化展示
		v, err := c.Do(ctx, "JSON.GET", key).Text()
		if err != nil {
			return nil, err
		}
		if len(v) > maxStringRead {
			d.StrValue = v[:maxStringRead]
			d.Truncated = true
		} else {
			d.StrValue = v
		}
		// 根节点元素数（对象取字段数，数组取长度，标量忽略）
		if n, err := c.Do(ctx, "JSON.OBJLEN", key, ".").Int64(); err == nil {
			d.Length = n
		} else if n, err := c.Do(ctx, "JSON.ARRLEN", key, ".").Int64(); err == nil {
			d.Length = n
		}
	case "hash":
		d.Length, _ = c.HLen(ctx, key).Result()
	case "list":
		d.Length, _ = c.LLen(ctx, key).Result()
	case "set":
		d.Length, _ = c.SCard(ctx, key).Result()
	case "zset":
		d.Length, _ = c.ZCard(ctx, key).Result()
	case "stream":
		if info, err := c.XInfoStream(ctx, key).Result(); err == nil {
			d.Length = info.Length
		}
	}
	return d, nil
}

// SetString 设置字符串值（ttl<0 保持原 TTL；ttl==0 不过期）
func (s *ValueService) SetString(connID string, db int, key, value string, ttl int64) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	if ttl < 0 {
		return c.SetArgs(s.ctx, key, value, redis.SetArgs{KeepTTL: true}).Err()
	}
	return c.Set(s.ctx, key, value, durationSeconds(ttl)).Err()
}

// SetJSON 设置 JSON 文档（RedisJSON；ttl<0 保持原 TTL；ttl==0 不过期）
func (s *ValueService) SetJSON(connID string, db int, key, value string, ttl int64) error {
	if !json.Valid([]byte(value)) {
		return fmt.Errorf("JSON 格式不合法")
	}
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	ctx := s.ctx
	// JSON.SET 不保留 TTL，需要保持时先读出再恢复
	var keepTTL time.Duration = -1
	if ttl < 0 {
		keepTTL, _ = c.TTL(ctx, key).Result()
	}
	if err := c.Do(ctx, "JSON.SET", key, "$", value).Err(); err != nil {
		return err
	}
	switch {
	case ttl < 0:
		if keepTTL > 0 {
			_ = c.Expire(ctx, key, keepTTL).Err()
		}
	case ttl == 0:
		_ = c.Persist(ctx, key).Err()
	default:
		_ = c.Expire(ctx, key, durationSeconds(ttl)).Err()
	}
	return nil
}

// GetFullJSON 获取完整 JSON 文档（"加载完整值"，突破预览截断）
func (s *ValueService) GetFullJSON(connID string, db int, key string) (string, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return "", err
	}
	return c.Do(s.ctx, "JSON.GET", key).Text()
}

// GetFullString 获取完整字符串值（"加载完整值"，突破预览 1MB 截断）
func (s *ValueService) GetFullString(connID string, db int, key string) (string, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return "", err
	}
	v, err := c.Get(s.ctx, key).Result()
	if err != nil {
		return "", err
	}
	if len(v) > maxFullStringRead {
		return "", fmt.Errorf("值过大（%.1f MB），超过 32MB 读取上限", float64(len(v))/1024/1024)
	}
	return v, nil
}

// GetStringHex 获取字符串值的完整十六进制表示（二进制安全查看）
func (s *ValueService) GetStringHex(connID string, db int, key string) (string, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return "", err
	}
	v, err := c.Get(s.ctx, key).Result()
	if err != nil {
		return "", err
	}
	if len(v) > maxFullStringRead {
		return "", fmt.Errorf("值过大（%.1f MB），超过 32MB 读取上限", float64(len(v))/1024/1024)
	}
	return hex.EncodeToString([]byte(v)), nil
}

// SetStringHex 以十六进制形式写入字符串值（二进制安全写入），空白字符自动忽略
func (s *ValueService) SetStringHex(connID string, db int, key, hexValue string, ttl int64) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r':
			return -1
		}
		return r
	}, hexValue)
	raw, err := hex.DecodeString(cleaned)
	if err != nil {
		return fmt.Errorf("hex 格式错误：%w", err)
	}
	if ttl < 0 {
		return c.SetArgs(s.ctx, key, raw, redis.SetArgs{KeepTTL: true}).Err()
	}
	return c.Set(s.ctx, key, raw, durationSeconds(ttl)).Err()
}

// HashGet 分页获取 hash 字段
func (s *ValueService) HashGet(connID string, db int, key string, cursor uint64, pattern string, count int64) (*model.ScanItems, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	if pattern == "" {
		pattern = "*"
	}
	if count <= 0 {
		count = 200
	}
	vals, next, err := c.HScan(s.ctx, key, cursor, pattern, count).Result()
	if err != nil {
		return nil, err
	}
	items := make([]model.HashItem, 0, len(vals)/2)
	for i := 0; i+1 < len(vals); i += 2 {
		items = append(items, model.HashItem{Field: vals[i], Value: vals[i+1]})
	}
	total, _ := c.HLen(s.ctx, key).Result()
	return &model.ScanItems{Cursor: next, Total: total, Items: items}, nil
}

// HashSet 设置 hash 字段
func (s *ValueService) HashSet(connID string, db int, key, field, value string) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	return c.HSet(s.ctx, key, field, value).Err()
}

// HashDel 删除 hash 字段
func (s *ValueService) HashDel(connID string, db int, key string, fields []string) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	return c.HDel(s.ctx, key, fields...).Err()
}

// HashRenameField 重命名 hash 字段
func (s *ValueService) HashRenameField(connID string, db int, key, field, newField string) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	v, err := c.HGet(s.ctx, key, field).Result()
	if err != nil {
		return err
	}
	pipe := c.TxPipeline()
	pipe.HSet(s.ctx, key, newField, v)
	pipe.HDel(s.ctx, key, field)
	_, err = pipe.Exec(s.ctx)
	return err
}

// ListGet 分页获取 list
func (s *ValueService) ListGet(connID string, db int, key string, page, pageSize int64) (*model.ScanItems, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 100
	}
	total, _ := c.LLen(s.ctx, key).Result()
	start := (page - 1) * pageSize
	stop := start + pageSize - 1
	vals, err := c.LRange(s.ctx, key, start, stop).Result()
	if err != nil {
		return nil, err
	}
	items := make([]model.ListItem, 0, len(vals))
	for i, v := range vals {
		items = append(items, model.ListItem{Index: start + int64(i), Value: v})
	}
	return &model.ScanItems{Total: total, Items: items}, nil
}

// ListSet 按索引更新
func (s *ValueService) ListSet(connID string, db int, key string, index int64, value string) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	return c.LSet(s.ctx, key, index, value).Err()
}

// ListPush 推入元素（side: left|right）
func (s *ValueService) ListPush(connID string, db int, key, side, value string) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	if side == "left" {
		return c.LPush(s.ctx, key, value).Err()
	}
	return c.RPush(s.ctx, key, value).Err()
}

// ListRemoveIndex 按索引删除（哨兵值技巧）
func (s *ValueService) ListRemoveIndex(connID string, db int, key string, index int64) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	sentinel := "__zp_rdm_deleted_" + randomHex(8)
	_, err = c.TxPipelined(s.ctx, func(pipe redis.Pipeliner) error {
		pipe.LSet(s.ctx, key, index, sentinel)
		pipe.LRem(s.ctx, key, 1, sentinel)
		return nil
	})
	return err
}

// SetGet 分页获取 set
func (s *ValueService) SetGet(connID string, db int, key string, cursor uint64, pattern string, count int64) (*model.ScanItems, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	if pattern == "" {
		pattern = "*"
	}
	if count <= 0 {
		count = 200
	}
	vals, next, err := c.SScan(s.ctx, key, cursor, pattern, count).Result()
	if err != nil {
		return nil, err
	}
	total, _ := c.SCard(s.ctx, key).Result()
	return &model.ScanItems{Cursor: next, Total: total, Items: vals}, nil
}

// SetAdd 添加成员
func (s *ValueService) SetAdd(connID string, db int, key, member string) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	return c.SAdd(s.ctx, key, member).Err()
}

// SetRemove 删除成员
func (s *ValueService) SetRemove(connID string, db int, key string, members []string) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	ifaces := make([]interface{}, len(members))
	for i, m := range members {
		ifaces[i] = m
	}
	return c.SRem(s.ctx, key, ifaces...).Err()
}

// SetUpdate 更新成员（删除旧值添加新值）
func (s *ValueService) SetUpdate(connID string, db int, key, oldMember, newMember string) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	_, err = c.TxPipelined(s.ctx, func(pipe redis.Pipeliner) error {
		pipe.SRem(s.ctx, key, oldMember)
		pipe.SAdd(s.ctx, key, newMember)
		return nil
	})
	return err
}

// ZSetGet 分页获取 zset（rev 按分数倒序）
func (s *ValueService) ZSetGet(connID string, db int, key string, page, pageSize int64, rev bool) (*model.ScanItems, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 100
	}
	total, _ := c.ZCard(s.ctx, key).Result()
	start := (page - 1) * pageSize
	stop := start + pageSize - 1
	var zs []redis.Z
	if rev {
		zs, err = c.ZRevRangeWithScores(s.ctx, key, start, stop).Result()
	} else {
		zs, err = c.ZRangeWithScores(s.ctx, key, start, stop).Result()
	}
	if err != nil {
		return nil, err
	}
	items := make([]model.ZSetItem, 0, len(zs))
	for _, z := range zs {
		m, _ := z.Member.(string)
		items = append(items, model.ZSetItem{Member: m, Score: z.Score})
	}
	return &model.ScanItems{Total: total, Items: items}, nil
}

// ZSetAdd 添加/更新成员分数
func (s *ValueService) ZSetAdd(connID string, db int, key, member string, score float64) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	return c.ZAdd(s.ctx, key, redis.Z{Score: score, Member: member}).Err()
}

// ZSetRemove 删除成员
func (s *ValueService) ZSetRemove(connID string, db int, key string, members []string) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	ifaces := make([]interface{}, len(members))
	for i, m := range members {
		ifaces[i] = m
	}
	return c.ZRem(s.ctx, key, ifaces...).Err()
}

// ZSetUpdate 更新成员（换名保留/更新分数）
func (s *ValueService) ZSetUpdate(connID string, db int, key, oldMember, newMember string, score float64) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	_, err = c.TxPipelined(s.ctx, func(pipe redis.Pipeliner) error {
		if oldMember != newMember {
			pipe.ZRem(s.ctx, key, oldMember)
		}
		pipe.ZAdd(s.ctx, key, redis.Z{Score: score, Member: newMember})
		return nil
	})
	return err
}

// StreamGet 分页获取 stream（rev 倒序）
func (s *ValueService) StreamGet(connID string, db int, key string, count int64, rev bool) (*model.ScanItems, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	if count <= 0 {
		count = 100
	}
	var msgs []redis.XMessage
	if rev {
		msgs, err = c.XRevRangeN(s.ctx, key, "+", "-", count).Result()
	} else {
		msgs, err = c.XRangeN(s.ctx, key, "-", "+", count).Result()
	}
	if err != nil {
		return nil, err
	}
	items := make([]model.StreamItem, 0, len(msgs))
	for _, m := range msgs {
		fields := make(map[string]string, len(m.Values))
		for k, v := range m.Values {
			if sv, ok := v.(string); ok {
				fields[k] = sv
			}
		}
		items = append(items, model.StreamItem{ID: m.ID, Fields: fields})
	}
	var total int64
	if info, err := c.XInfoStream(s.ctx, key).Result(); err == nil {
		total = info.Length
	}
	return &model.ScanItems{Total: total, Items: items}, nil
}

// StreamAdd 添加条目（fields 为扁平 "k=v" 列表的 JSON 友好形式：map）
func (s *ValueService) StreamAdd(connID string, db int, key string, fields map[string]string) (string, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return "", err
	}
	vals := make([]interface{}, 0, len(fields)*2)
	for k, v := range fields {
		vals = append(vals, k, v)
	}
	return c.XAdd(s.ctx, &redis.XAddArgs{Stream: key, Values: vals}).Result()
}

// StreamDel 删除条目
func (s *ValueService) StreamDel(connID string, db int, key string, ids []string) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	return c.XDel(s.ctx, key, ids...).Err()
}

// CreateKey 新建键（按类型放入初始成员）
func (s *ValueService) CreateKey(connID string, db int, key, typ, value, field string, score float64, ttl int64) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	ctx := s.ctx
	switch strings.ToLower(typ) {
	case "string":
		if err := c.Set(ctx, key, value, 0).Err(); err != nil {
			return err
		}
	case "hash":
		if field == "" {
			field = "field"
		}
		if err := c.HSet(ctx, key, field, value).Err(); err != nil {
			return err
		}
	case "list":
		if err := c.RPush(ctx, key, value).Err(); err != nil {
			return err
		}
	case "set":
		if err := c.SAdd(ctx, key, value).Err(); err != nil {
			return err
		}
	case "zset":
		if err := c.ZAdd(ctx, key, redis.Z{Score: score, Member: value}).Err(); err != nil {
			return err
		}
	case "stream":
		if field == "" {
			field = "field"
		}
		if _, err := c.XAdd(ctx, &redis.XAddArgs{Stream: key, Values: []string{field, value}}).Result(); err != nil {
			return err
		}
	case "json":
		if !json.Valid([]byte(value)) {
			return fmt.Errorf("JSON 格式不合法")
		}
		if err := c.Do(ctx, "JSON.SET", key, "$", value).Err(); err != nil {
			return err
		}
	default:
		return redis.Nil
	}
	if ttl > 0 {
		return c.Expire(ctx, key, durationSeconds(ttl)).Err()
	}
	return nil
}
