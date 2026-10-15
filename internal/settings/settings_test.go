package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func newTemp(t *testing.T) (*Manager, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sub", "settings.json")
	m, err := NewAt(path)
	if err != nil {
		t.Fatalf("NewAt: %v", err)
	}
	return m, path
}

func TestDefaults(t *testing.T) {
	m, _ := newTemp(t)
	s := m.Get()
	if s.Window.Width != 1440 || s.Window.Height != 900 {
		t.Fatalf("unexpected default window: %+v", s.Window)
	}
	if s.Window.X != -1 || s.Window.Y != -1 {
		t.Fatalf("default position should be -1,-1 (system placement): %+v", s.Window)
	}
	if s.SidebarWidth != 236 {
		t.Fatalf("default sidebar width = %d, want 236", s.SidebarWidth)
	}
	if s.LastDb == nil || len(s.LastDb) != 0 {
		t.Fatalf("default lastDb should be empty map: %+v", s.LastDb)
	}
}

func TestSaveFrontendStateRoundTrip(t *testing.T) {
	m, path := newTemp(t)
	err := m.SaveFrontendState("light", 300, map[string]int{"conn-a": 3, "conn-b": 0})
	if err != nil {
		t.Fatalf("SaveFrontendState: %v", err)
	}

	// 重新加载验证落盘
	m2, err := NewAt(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	s := m2.Get()
	if s.Theme != "light" {
		t.Fatalf("theme = %q, want light", s.Theme)
	}
	if s.SidebarWidth != 300 {
		t.Fatalf("sidebarWidth = %d, want 300", s.SidebarWidth)
	}
	if s.LastDb["conn-a"] != 3 {
		t.Fatalf("lastDb[conn-a] = %d, want 3", s.LastDb["conn-a"])
	}
	if v, ok := s.LastDb["conn-b"]; !ok || v != 0 {
		t.Fatalf("lastDb[conn-b] = %d,%v, want 0,true", v, ok)
	}
}

func TestSidebarWidthClamp(t *testing.T) {
	m, path := newTemp(t)
	// 越界值不应写入
	if err := m.SaveFrontendState("dark", 9999, nil); err != nil {
		t.Fatal(err)
	}
	if got := m.Get().SidebarWidth; got != 236 {
		t.Fatalf("out-of-range width persisted: %d", got)
	}

	// 损坏的落盘文件：重载时应回退默认值
	bad := map[string]any{"sidebarWidth": 10, "theme": "dark"}
	b, _ := json.Marshal(bad)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	m2, err := NewAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := m2.Get().SidebarWidth; got != 236 {
		t.Fatalf("corrupt width not reset: %d", got)
	}
	if got := m2.Get().Theme; got != "dark" {
		t.Fatalf("valid theme field lost: %q", got)
	}
}

func TestSaveWindowState(t *testing.T) {
	m, path := newTemp(t)
	w := WindowState{Width: 1600, Height: 1000, X: 100, Y: 50, Maximised: false}
	if err := m.SaveWindowState(w); err != nil {
		t.Fatal(err)
	}
	m2, err := NewAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := m2.Get().Window; got != w {
		t.Fatalf("window state = %+v, want %+v", got, w)
	}

	// 零尺寸不应覆盖已保存的几何（防御性）
	if err := m2.SaveWindowState(WindowState{}); err != nil {
		t.Fatal(err)
	}
	if got := m2.Get().Window; got.Width != 1600 {
		t.Fatalf("zero-size overwrite happened: %+v", got)
	}
}

func TestGetReturnsCopy(t *testing.T) {
	m, _ := newTemp(t)
	_ = m.SaveFrontendState("dark", 236, map[string]int{"a": 1})
	s := m.Get()
	s.LastDb["a"] = 99
	s.LastDb["injected"] = 5
	if m.Get().LastDb["a"] != 1 {
		t.Fatal("Get leaked mutable map")
	}
	if _, ok := m.Get().LastDb["injected"]; ok {
		t.Fatal("injected key leaked into manager")
	}
}

func TestMissingFileIsNotError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent", "settings.json")
	if _, err := NewAt(path); err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
}

func TestCorruptFileFallsBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := NewAt(path)
	if err != nil {
		t.Fatalf("corrupt file should fall back to defaults: %v", err)
	}
	if m.Get().SidebarWidth != 236 {
		t.Fatal("defaults not applied after corrupt file")
	}
}

func TestCliHistory(t *testing.T) {
	m, path := newTemp(t)
	h := make([]string, 0, 250)
	for i := 0; i < 250; i++ {
		h = append(h, "CMD "+string(rune('A'+i%26)))
	}
	if err := m.SaveCliHistory(h); err != nil {
		t.Fatal(err)
	}
	// 截断到 200 条，保留最近
	if got := len(m.Get().CliHistory); got != 200 {
		t.Fatalf("history len = %d, want 200", got)
	}
	if got := m.Get().CliHistory[199]; got != "CMD P" { // i=249 → 'A'+249%26='P'
		t.Fatalf("last entry = %q, want CMD P", got)
	}

	// 落盘往返
	m2, err := NewAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(m2.Get().CliHistory) != 200 {
		t.Fatal("history lost after reload")
	}

	// Get 返回副本，外部修改不影响内部
	hist := m2.Get().CliHistory
	hist[0] = "MUTATED"
	if m2.Get().CliHistory[0] == "MUTATED" {
		t.Fatal("Get leaked mutable slice")
	}
}

func TestWorkspace(t *testing.T) {
	m, path := newTemp(t)
	// 默认值
	w := m.Get().Workspace
	if w.Open == nil || w.Conns == nil || w.Active != "" {
		t.Fatalf("default workspace malformed: %+v", w)
	}

	err := m.SaveWorkspace(Workspace{
		Open:   []string{"conn-a", "conn-b"},
		Active: "conn-b",
		Conns: map[string]*ConnWorkspace{
			"conn-a": {View: "data", SelectedKey: "user:1", Pattern: "user:*", ViewMode: "tree", Delimiter: ":"},
			"conn-b": {View: "cli"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 落盘往返
	m2, err := NewAt(path)
	if err != nil {
		t.Fatal(err)
	}
	got := m2.Get().Workspace
	if got.Active != "conn-b" || len(got.Open) != 2 || got.Open[0] != "conn-a" {
		t.Fatalf("workspace not persisted: %+v", got)
	}
	ca := got.Conns["conn-a"]
	if ca == nil || ca.SelectedKey != "user:1" || ca.Pattern != "user:*" || ca.Delimiter != ":" {
		t.Fatalf("conn-a workspace lost: %+v", ca)
	}

	// Get 深拷贝：外部修改不影响内部
	ca.SelectedKey = "MUTATED"
	got.Open[0] = "MUTATED"
	if m2.Get().Workspace.Conns["conn-a"].SelectedKey != "user:1" {
		t.Fatal("Get leaked mutable conn workspace")
	}
	if m2.Get().Workspace.Open[0] != "conn-a" {
		t.Fatal("Get leaked mutable open slice")
	}
}
