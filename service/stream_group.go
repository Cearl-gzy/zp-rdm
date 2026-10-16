package service

import (
	"zp-rdm/internal/model"

	"github.com/redis/go-redis/v9"
)

// Stream 消费者组管理（XINFO GROUPS / CONSUMERS、XPENDING、XACK、XGROUP）。

// StreamGroups 列出 stream 的消费者组
func (s *ValueService) StreamGroups(connID string, db int, key string) ([]model.StreamGroup, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	gs, err := c.XInfoGroups(s.ctx, key).Result()
	if err != nil {
		return nil, err
	}
	out := make([]model.StreamGroup, 0, len(gs))
	for _, g := range gs {
		out = append(out, model.StreamGroup{
			Name:            g.Name,
			Consumers:       g.Consumers,
			Pending:         g.Pending,
			LastDeliveredID: g.LastDeliveredID,
			EntriesRead:     g.EntriesRead,
			Lag:             g.Lag,
		})
	}
	return out, nil
}

// StreamConsumers 列出组内消费者
func (s *ValueService) StreamConsumers(connID string, db int, key, group string) ([]model.StreamConsumer, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	cs, err := c.XInfoConsumers(s.ctx, key, group).Result()
	if err != nil {
		return nil, err
	}
	out := make([]model.StreamConsumer, 0, len(cs))
	for _, x := range cs {
		out = append(out, model.StreamConsumer{
			Name:    x.Name,
			Pending: x.Pending,
			IdleMs:  x.Idle.Milliseconds(),
		})
	}
	return out, nil
}

// StreamPending 列出组的待确认条目（XPENDING 扩展形式，上限 200 条）
func (s *ValueService) StreamPending(connID string, db int, key, group string, count int64) ([]model.StreamPendingEntry, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	if count <= 0 || count > 200 {
		count = 200
	}
	ps, err := c.XPendingExt(s.ctx, &redis.XPendingExtArgs{
		Stream: key,
		Group:  group,
		Start:  "-",
		End:    "+",
		Count:  count,
	}).Result()
	if err != nil {
		return nil, err
	}
	out := make([]model.StreamPendingEntry, 0, len(ps))
	for _, p := range ps {
		out = append(out, model.StreamPendingEntry{
			ID:            p.ID,
			Consumer:      p.Consumer,
			IdleMs:        p.Idle.Milliseconds(),
			DeliveryCount: p.RetryCount,
		})
	}
	return out, nil
}

// StreamAck 确认条目（XACK），返回确认条数
func (s *ValueService) StreamAck(connID string, db int, key, group string, ids []string) (int64, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return 0, err
	}
	return c.XAck(s.ctx, key, group, ids...).Result()
}

// StreamGroupCreate 创建消费者组；startID 为空按 "$"（只接收新消息），传 "0" 从头消费。
// mkStream 为 true 时 stream 不存在则自动创建空 stream。
func (s *ValueService) StreamGroupCreate(connID string, db int, key, group, startID string, mkStream bool) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	if startID == "" {
		startID = "$"
	}
	if mkStream {
		return c.XGroupCreateMkStream(s.ctx, key, group, startID).Err()
	}
	return c.XGroupCreate(s.ctx, key, group, startID).Err()
}

// StreamGroupDestroy 删除消费者组
func (s *ValueService) StreamGroupDestroy(connID string, db int, key, group string) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	return c.XGroupDestroy(s.ctx, key, group).Err()
}

// StreamConsumerDelete 删除组内消费者（同时清除其 pending 记录），返回被清除的 pending 条数
func (s *ValueService) StreamConsumerDelete(connID string, db int, key, group, consumer string) (int64, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return 0, err
	}
	return c.XGroupDelConsumer(s.ctx, key, group, consumer).Result()
}
