// Package settings 应用设置持久化（%AppData%/zp-rdm/settings.json）
package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// WindowState 窗口几何状态
type WindowState struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Maximised bool `json:"maximised"`
}

// Settings 应用设置
type Settings struct {
	Window       WindowState    `json:"window"`
	SidebarWidth int            `json:"sidebarWidth"`
	Theme        string         `json:"theme"` // dark | light | ""（跟随默认深色）
	LastDb       map[string]int `json:"lastDb"`
	CliHistory   []string       `json:"cliHistory"` // CLI 命令历史（全局，上限 200 条）
}

// Manager 设置读写（带锁）
type Manager struct {
	mu   sync.Mutex
	path string
	data Settings
}

func defaults() Settings {
	return Settings{
		Window:       WindowState{Width: 1440, Height: 900, X: -1, Y: -1},
		SidebarWidth: 236,
		Theme:        "",
		LastDb:       map[string]int{},
		CliHistory:   []string{},
	}
}

func New() (*Manager, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return NewAt(filepath.Join(dir, "zp-rdm", "settings.json"))
}

// NewAt 使用指定文件路径（便于测试与自定义配置目录）
func NewAt(path string) (*Manager, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	m := &Manager{path: path, data: defaults()}
	if b, err := os.ReadFile(m.path); err == nil {
		_ = json.Unmarshal(b, &m.data)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if m.data.LastDb == nil {
		m.data.LastDb = map[string]int{}
	}
	if m.data.CliHistory == nil {
		m.data.CliHistory = []string{}
	}
	if m.data.SidebarWidth < 160 || m.data.SidebarWidth > 480 {
		m.data.SidebarWidth = 236
	}
	return m, nil
}

// Get 读取当前设置（副本）
func (m *Manager) Get() Settings {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.data
	lastDb := make(map[string]int, len(m.data.LastDb))
	for k, v := range m.data.LastDb {
		lastDb[k] = v
	}
	s.LastDb = lastDb
	s.CliHistory = append([]string(nil), m.data.CliHistory...)
	return s
}

// SaveFrontendState 更新前端相关字段并落盘
func (m *Manager) SaveFrontendState(theme string, sidebarWidth int, lastDb map[string]int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data.Theme = theme
	if sidebarWidth >= 160 && sidebarWidth <= 480 {
		m.data.SidebarWidth = sidebarWidth
	}
	if lastDb != nil {
		m.data.LastDb = lastDb
	}
	return m.saveLocked()
}

// maxCliHistory CLI 历史上限
const maxCliHistory = 200

// SaveCliHistory 保存 CLI 命令历史（超出上限截断，保留最近）
func (m *Manager) SaveCliHistory(history []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(history) > maxCliHistory {
		history = history[len(history)-maxCliHistory:]
	}
	m.data.CliHistory = append([]string(nil), history...)
	return m.saveLocked()
}

// SaveWindowState 更新窗口几何并落盘（退出时调用）
func (m *Manager) SaveWindowState(w WindowState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w.Width > 0 {
		m.data.Window = w
	}
	return m.saveLocked()
}

func (m *Manager) saveLocked() error {
	b, err := json.MarshalIndent(&m.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, b, 0o600)
}
