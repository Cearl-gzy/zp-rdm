package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
	"zp-rdm/internal/redisx"
)

// ServerService 服务器信息 / 慢日志 / 发布订阅
type ServerService struct {
	ctx     context.Context
	manager *redisx.Manager

	mu       sync.Mutex
	subs     map[string]*redis.PubSub  // key: connID|db|channel
	monitors map[string]net.Conn       // key: connID|db
	hotkeys  map[string]*hotKeySampler // key: connID|db
}

func NewServerService(m *redisx.Manager) *ServerService {
	return &ServerService{
		manager:  m,
		subs:     make(map[string]*redis.PubSub),
		monitors: make(map[string]net.Conn),
		hotkeys:  make(map[string]*hotKeySampler),
	}
}

func (s *ServerService) Startup(ctx context.Context) { s.ctx = ctx }

func (s *ServerService) cli(connID string, db int) (redis.UniversalClient, error) {
	return s.manager.Get(connID, db)
}

// GetInfo 获取 INFO（按 section 分组）
func (s *ServerService) GetInfo(connID string, db int, sections ...string) (map[string]map[string]string, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	section := "all"
	if len(sections) > 0 && sections[0] != "" {
		section = strings.Join(sections, " ")
	}
	raw, err := c.Info(s.ctx, strings.Split(section, " ")...).Result()
	if err != nil {
		return nil, err
	}
	out := make(map[string]map[string]string)
	cur := "default"
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			cur = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "#")))
			if _, ok := out[cur]; !ok {
				out[cur] = make(map[string]string)
			}
			continue
		}
		if i := strings.Index(line, ":"); i > 0 {
			out[cur][line[:i]] = line[i+1:]
		}
	}
	return out, nil
}

// SearchIndexes RediSearch 索引列表（FT._LIST）。
// 未加载 search 模块时返回空切片而非报错——调用方按"无索引"展示。
func (s *ServerService) SearchIndexes(connID string, db int) ([]string, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	res, err := c.Do(s.ctx, "FT._LIST").StringSlice()
	if err != nil {
		return []string{}, nil
	}
	return res, nil
}

// DbSize 键总数
func (s *ServerService) DbSize(connID string, db int) (int64, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return 0, err
	}
	return c.DBSize(s.ctx).Result()
}

// SlowLog 慢日志
func (s *ServerService) SlowLog(connID string, db int, count int64) ([]model.SlowLogEntry, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	if count <= 0 {
		count = 128
	}
	logs, err := c.SlowLogGet(s.ctx, count).Result()
	if err != nil {
		return nil, err
	}
	out := make([]model.SlowLogEntry, 0, len(logs))
	for _, l := range logs {
		args := make([]string, 0, len(l.Args))
		for _, a := range l.Args {
			args = append(args, fmt.Sprintf("%v", a))
		}
		out = append(out, model.SlowLogEntry{
			ID:         l.ID,
			Timestamp:  l.Time.Unix(),
			DurationUs: l.Duration.Microseconds(),
			Args:       args,
			ClientAddr: l.ClientAddr,
			ClientName: l.ClientName,
		})
	}
	return out, nil
}

// SlowLogReset 清空慢日志
func (s *ServerService) SlowLogReset(connID string, db int) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	return c.Do(s.ctx, "SLOWLOG", "RESET").Err()
}

// ClientList 客户端列表
func (s *ServerService) ClientList(connID string, db int) ([]model.ClientConnInfo, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	raw, err := c.ClientList(s.ctx).Result()
	if err != nil {
		return nil, err
	}
	var out []model.ClientConnInfo
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		fields := make(map[string]string)
		for _, kv := range strings.Split(line, " ") {
			if i := strings.Index(kv, "="); i > 0 {
				fields[kv[:i]] = kv[i+1:]
			}
		}
		if len(fields) == 0 {
			continue
		}
		ci := model.ClientConnInfo{
			Addr: fields["addr"], Name: fields["name"], Cmd: fields["cmd"], Flags: fields["flags"],
		}
		ci.ID, _ = strconv.ParseInt(fields["id"], 10, 64)
		ci.Age, _ = strconv.ParseInt(fields["age"], 10, 64)
		ci.Idle, _ = strconv.ParseInt(fields["idle"], 10, 64)
		ci.DB, _ = strconv.Atoi(fields["db"])
		out = append(out, ci)
	}
	return out, nil
}

// KillClient 断开客户端
func (s *ServerService) KillClient(connID string, db int, addr string) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	return c.Do(s.ctx, "CLIENT", "KILL", "ADDR", addr).Err()
}

// GetConfig 配置查询
func (s *ServerService) GetConfig(connID string, db int, pattern string) (map[string]string, error) {
	if s.manager.IsCluster(connID) {
		return nil, errClusterUnsupported
	}
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	if pattern == "" {
		pattern = "*"
	}
	return c.ConfigGet(s.ctx, pattern).Result()
}

// SetConfig 修改运行参数（CONFIG SET，不落盘，重启失效——持久化需 CONFIG REWRITE）
func (s *ServerService) SetConfig(connID string, db int, param, value string) error {
	if s.manager.IsCluster(connID) {
		return errClusterUnsupported
	}
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	param = strings.TrimSpace(param)
	if param == "" {
		return errors.New("参数名不能为空")
	}
	return c.ConfigSet(s.ctx, param, value).Err()
}

// ConfigRewrite 把当前运行配置写回配置文件（CONFIG REWRITE）
func (s *ServerService) ConfigRewrite(connID string, db int) error {
	if s.manager.IsCluster(connID) {
		return errClusterUnsupported
	}
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	return c.Do(s.ctx, "CONFIG", "REWRITE").Err()
}

// Publish 发布消息
func (s *ServerService) Publish(connID string, db int, channel, message string) (int64, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return 0, err
	}
	return c.Publish(s.ctx, channel, message).Result()
}

// PubSubChannels 当前活跃的频道
func (s *ServerService) PubSubChannels(connID string, db int, pattern string) ([]string, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	return c.PubSubChannels(s.ctx, pattern).Result()
}

// Subscribe 订阅频道（pattern=true 使用 PSUBSCRIBE），消息通过事件 pubsub:connID:db 推送
func (s *ServerService) Subscribe(connID string, db int, channel string, pattern bool) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	subKey := fmt.Sprintf("%s|%d|%s", connID, db, channel)
	s.mu.Lock()
	if _, ok := s.subs[subKey]; ok {
		s.mu.Unlock()
		return fmt.Errorf("已订阅该频道")
	}
	var ps *redis.PubSub
	if pattern {
		ps = c.PSubscribe(s.ctx, channel)
	} else {
		ps = c.Subscribe(s.ctx, channel)
	}
	s.subs[subKey] = ps
	s.mu.Unlock()

	eventName := fmt.Sprintf("pubsub:%s:%d", connID, db)
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.subs, subKey)
			s.mu.Unlock()
			_ = ps.Close()
		}()
		ch := ps.Channel()
		for msg := range ch {
			kind := "message"
			if msg.Pattern != "" {
				kind = "pmessage"
			}
			emit(s.ctx, eventName, model.PubSubMessage{
				Channel: msg.Channel,
				Pattern: msg.Pattern,
				Payload: msg.Payload,
				Time:    nowMillis(),
				Kind:    kind,
			})
		}
	}()
	return nil
}

// Unsubscribe 取消订阅
func (s *ServerService) Unsubscribe(connID string, db int, channel string) error {
	subKey := fmt.Sprintf("%s|%d|%s", connID, db, channel)
	s.mu.Lock()
	ps, ok := s.subs[subKey]
	delete(s.subs, subKey)
	s.mu.Unlock()
	if !ok {
		return nil
	}
	return ps.Close()
}

// UnsubscribeAll 取消该连接全部订阅
func (s *ServerService) UnsubscribeAll(connID string, db int) {
	prefix := fmt.Sprintf("%s|%d|", connID, db)
	s.mu.Lock()
	var toClose []*redis.PubSub
	for k, ps := range s.subs {
		if strings.HasPrefix(k, prefix) {
			toClose = append(toClose, ps)
			delete(s.subs, k)
		}
	}
	s.mu.Unlock()
	for _, ps := range toClose {
		_ = ps.Close()
	}
}
