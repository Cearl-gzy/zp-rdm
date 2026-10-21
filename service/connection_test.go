package service

import (
	"context"
	"path/filepath"
	"testing"

	"zp-rdm/internal/model"
	"zp-rdm/internal/store"
)

// SetConnectionGroup：仅改分组字段，不触碰连接管理器（已打开的连接保持在线）。
func TestSetConnectionGroup(t *testing.T) {
	st, err := store.NewAt(filepath.Join(t.TempDir(), "connections.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewConnectionService(st, nil)
	s.Startup(context.Background())

	p := model.ConnectionProfile{Name: "g1", Host: "127.0.0.1", Port: 6379}
	if err := st.Upsert(p); err != nil {
		t.Fatal(err)
	}
	saved := st.List()[0]

	if err := s.SetConnectionGroup(saved.ID, "生产环境"); err != nil {
		t.Fatal(err)
	}
	got, ok := st.Get(saved.ID)
	if !ok {
		t.Fatal("连接丢失")
	}
	if got.Group != "生产环境" {
		t.Fatalf("分组未更新: %q", got.Group)
	}

	// 清空分组（移回默认分组）
	if err := s.SetConnectionGroup(saved.ID, ""); err != nil {
		t.Fatal(err)
	}
	got, _ = st.Get(saved.ID)
	if got.Group != "" {
		t.Fatalf("分组未清空: %q", got.Group)
	}

	// 不存在的 ID 应报错
	if err := s.SetConnectionGroup("no-such-id", "x"); err == nil {
		t.Fatal("不存在的连接应返回错误")
	}
}
