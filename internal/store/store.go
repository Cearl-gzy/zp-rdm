package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"zp-rdm/internal/model"
	"zp-rdm/internal/secrets"
)

// Store 连接配置持久化（JSON 文件）
type Store struct {
	mu   sync.Mutex
	path string
	data struct {
		Connections []model.ConnectionProfile `json:"connections"`
	}
}

func New() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return NewAt(filepath.Join(dir, "zp-rdm", "connections.json"))
}

// NewAt 使用指定文件路径（便于测试与自定义配置目录）
func NewAt(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	s := &Store{path: path}
	if migrated := s.load(); migrated {
		// 旧版明文文件 → 立即以密文重写
		s.mu.Lock()
		_ = s.saveLocked()
		s.mu.Unlock()
	}
	return s, nil
}

// protectProfile 敏感字段加密（返回副本，不改原对象）
func protectProfile(p model.ConnectionProfile) model.ConnectionProfile {
	enc := func(v string) string {
		if v == "" || secrets.IsProtected(v) {
			return v
		}
		if out, err := secrets.Protect(v); err == nil {
			return out
		}
		return v
	}
	p.Password = enc(p.Password)
	p.SSH.Password = enc(p.SSH.Password)
	p.SSH.PrivateKey = enc(p.SSH.PrivateKey)
	p.SSH.Passphrase = enc(p.SSH.Passphrase)
	return p
}

// unprotectProfile 敏感字段解密（就地），返回是否存在旧版明文（需迁移）
func unprotectProfile(p *model.ConnectionProfile) bool {
	plain := false
	dec := func(v string) string {
		if v == "" {
			return v
		}
		if !secrets.IsProtected(v) {
			plain = true
			return v
		}
		return secrets.Unprotect(v)
	}
	p.Password = dec(p.Password)
	p.SSH.Password = dec(p.SSH.Password)
	p.SSH.PrivateKey = dec(p.SSH.PrivateKey)
	p.SSH.Passphrase = dec(p.SSH.Passphrase)
	return plain
}

// load 返回是否存在需要迁移的明文字段
func (s *Store) load() bool {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return false
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return false
	}
	migrated := false
	for i := range s.data.Connections {
		if unprotectProfile(&s.data.Connections[i]) {
			migrated = true
		}
	}
	return migrated
}

// saveLocked 落盘时敏感字段加密（内存中保持明文）
func (s *Store) saveLocked() error {
	disk := struct {
		Connections []model.ConnectionProfile `json:"connections"`
	}{Connections: make([]model.ConnectionProfile, len(s.data.Connections))}
	for i, c := range s.data.Connections {
		disk.Connections[i] = protectProfile(c)
	}
	b, err := json.MarshalIndent(&disk, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o600)
}

func (s *Store) List() []model.ConnectionProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.ConnectionProfile, len(s.data.Connections))
	copy(out, s.data.Connections)
	return out
}

func (s *Store) Get(id string) (model.ConnectionProfile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.data.Connections {
		if c.ID == id {
			return c, true
		}
	}
	return model.ConnectionProfile{}, false
}

func (s *Store) Upsert(p model.ConnectionProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.data.Connections {
		if c.ID == p.ID {
			s.data.Connections[i] = p
			return s.saveLocked()
		}
	}
	s.data.Connections = append(s.data.Connections, p)
	return s.saveLocked()
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.data.Connections {
		if c.ID == id {
			s.data.Connections = append(s.data.Connections[:i], s.data.Connections[i+1:]...)
			return s.saveLocked()
		}
	}
	return nil
}

// Export 导出到指定路径
func (s *Store) Export(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.MarshalIndent(&s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// Import 从指定路径导入（合并，按 ID 覆盖）
func (s *Store) Import(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var incoming struct {
		Connections []model.ConnectionProfile `json:"connections"`
	}
	if err := json.Unmarshal(b, &incoming); err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := make(map[string]int)
	for i, c := range s.data.Connections {
		idx[c.ID] = i
	}
	n := 0
	for _, c := range incoming.Connections {
		if i, ok := idx[c.ID]; ok {
			s.data.Connections[i] = c
		} else {
			s.data.Connections = append(s.data.Connections, c)
		}
		n++
	}
	return n, s.saveLocked()
}
