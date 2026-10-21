package service

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"zp-rdm/internal/model"
	"zp-rdm/internal/redisx"
	"zp-rdm/internal/store"
)

// ConnectionService 连接管理
type ConnectionService struct {
	ctx     context.Context
	store   *store.Store
	manager *redisx.Manager
}

func NewConnectionService(s *store.Store, m *redisx.Manager) *ConnectionService {
	return &ConnectionService{store: s, manager: m}
}

func (s *ConnectionService) Startup(ctx context.Context) { s.ctx = ctx }

// ListConnections 全部连接配置
func (s *ConnectionService) ListConnections() []model.ConnectionProfile {
	return s.store.List()
}

// SaveConnection 新建/更新（无 ID 自动生成）
func (s *ConnectionService) SaveConnection(p model.ConnectionProfile) (model.ConnectionProfile, error) {
	if p.Name == "" {
		p.Name = fmt.Sprintf("%s:%d", p.Host, p.Port)
	}
	if p.Port <= 0 {
		p.Port = 6379
	}
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if err := s.store.Upsert(p); err != nil {
		return p, err
	}
	// 已打开的连接配置变更后关闭，下次重新打开
	if s.manager.IsOpen(p.ID) {
		s.manager.Close(p.ID)
	}
	return p, nil
}

// SetConnectionGroup 仅更新连接的分组（不关闭已打开的连接，供分组重命名/移动使用）
func (s *ConnectionService) SetConnectionGroup(id, group string) error {
	p, ok := s.store.Get(id)
	if !ok {
		return fmt.Errorf("连接不存在")
	}
	p.Group = group
	return s.store.Upsert(p)
}

// DeleteConnection 删除连接
func (s *ConnectionService) DeleteConnection(id string) error {
	s.manager.Close(id)
	return s.store.Delete(id)
}

// DuplicateConnection 复制连接配置
func (s *ConnectionService) DuplicateConnection(id string) (model.ConnectionProfile, error) {
	p, ok := s.store.Get(id)
	if !ok {
		return p, fmt.Errorf("连接不存在")
	}
	p.ID = uuid.NewString()
	p.Name = p.Name + " 副本"
	return p, s.store.Upsert(p)
}

// TestConnection 测试连接
func (s *ConnectionService) TestConnection(p model.ConnectionProfile) error {
	if p.Port <= 0 {
		p.Port = 6379
	}
	return s.manager.Test(s.ctx, p)
}

// OpenConnection 打开连接
func (s *ConnectionService) OpenConnection(id string) (*model.ConnState, error) {
	p, ok := s.store.Get(id)
	if !ok {
		return nil, fmt.Errorf("连接不存在")
	}
	if st := s.manager.State(id); st != nil {
		return st, nil
	}
	return s.manager.Open(s.ctx, p)
}

// CloseConnection 关闭连接
func (s *ConnectionService) CloseConnection(id string) {
	s.manager.Close(id)
}

// OpenStates 已打开的连接
func (s *ConnectionService) OpenStates() []model.ConnState {
	return s.manager.OpenStates()
}

// PingRTT 测量 PING 往返时延（毫秒），供侧栏连接状态展示
func (s *ConnectionService) PingRTT(connID string) (int64, error) {
	c, err := s.manager.Get(connID, 0)
	if err != nil {
		return 0, err
	}
	start := time.Now()
	if err := c.Ping(s.ctx).Err(); err != nil {
		return 0, err
	}
	return time.Since(start).Milliseconds(), nil
}

// ExportConnections 导出连接配置
func (s *ConnectionService) ExportConnections(path string) error {
	return s.store.Export(path)
}

// ImportConnections 导入连接配置
func (s *ConnectionService) ImportConnections(path string) (int, error) {
	if fi, err := os.Stat(path); err != nil || fi.IsDir() {
		return 0, fmt.Errorf("文件不存在")
	}
	return s.store.Import(path)
}
