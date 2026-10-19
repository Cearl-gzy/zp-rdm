package service

import (
	"strings"
	"testing"
)

func TestRespToStringMap(t *testing.T) {
	// RESP2 扁平数组
	m, err := respToStringMap([]interface{}{"flags", []interface{}{"on", "nopass"}, "commands", "+@all"})
	if err != nil {
		t.Fatal(err)
	}
	if m["commands"] != "+@all" {
		t.Fatalf("commands = %v", m["commands"])
	}
	// RESP3 map
	m, err = respToStringMap(map[interface{}]interface{}{"keys": "~*", "channels": "&*"})
	if err != nil {
		t.Fatal(err)
	}
	if m["keys"] != "~*" || m["channels"] != "&*" {
		t.Fatalf("got %v", m)
	}
	// 未知类型
	if _, err = respToStringMap("nope"); err == nil {
		t.Fatal("未知类型应报错")
	}
}

func TestRespStringList(t *testing.T) {
	got := respStringList([]interface{}{"a", "b", 1})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("got %v", got)
	}
	if got := respStringList("not-a-list"); got != nil {
		t.Fatalf("got %v", got)
	}
}

func TestAclSetUserValidation(t *testing.T) {
	s, _, connID := newServerService(t)
	// 空用户名
	if err := s.AclSetUser(connID, 0, " ", []string{"on"}); err == nil {
		t.Fatal("空用户名应报错")
	}
	// 空规则
	if err := s.AclSetUser(connID, 0, "u1", []string{" ", ""}); err == nil {
		t.Fatal("空规则应报错")
	}
	// 未填用户名的 GetUser
	if _, err := s.AclGetUser(connID, 0, ""); err == nil {
		t.Log("ACL GETUSER 空名由服务端报错（或桩不支持）:", err)
	}
}

// requireAclServer 探测服务器是否支持 ACL / CONFIG（miniredis 桩可能不支持则跳过）
func requireAclServer(t *testing.T, s *ServerService, connID string) {
	t.Helper()
	if _, err := s.AclUsers(connID, 0); err != nil {
		t.Skipf("服务器不支持 ACL（%v），跳过", err)
	}
}

func TestConfigGetSetRoundtrip(t *testing.T) {
	s, _, connID := newServerService(t)
	if _, err := s.GetConfig(connID, 0, "timeout"); err != nil {
		t.Skipf("服务器不支持 CONFIG（%v），跳过", err)
	}
	if err := s.SetConfig(connID, 0, "timeout", "0"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	cfg, err := s.GetConfig(connID, 0, "timeout")
	if err != nil {
		t.Fatal(err)
	}
	if cfg["timeout"] != "0" {
		t.Fatalf("timeout = %q, want 0", cfg["timeout"])
	}
	// 空参数名
	if err := s.SetConfig(connID, 0, " ", "1"); err == nil {
		t.Fatal("空参数名应报错")
	}
}

func TestAclUserLifecycle(t *testing.T) {
	s, _, connID := newServerService(t)
	requireAclServer(t, s, connID)

	const user = "zp-acl-test-user"
	// 清理残留
	_ = s.AclDelUser(connID, 0, user)
	t.Cleanup(func() { _ = s.AclDelUser(connID, 0, user) })

	// 创建：启用 + 密码 + 键模式 + 只读 + 频道
	err := s.AclSetUser(connID, 0, user, []string{"on", ">testpass123", "~zp:acl:*", "+@read", "&*"})
	if err != nil {
		t.Fatalf("AclSetUser: %v", err)
	}

	names, err := s.AclUsers(connID, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range names {
		if n == user {
			found = true
		}
	}
	if !found {
		t.Fatalf("ACL USERS 中未找到 %s: %v", user, names)
	}

	info, err := s.AclGetUser(connID, 0, user)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Enabled {
		t.Error("Enabled 应为 true")
	}
	if info.NoPass {
		t.Error("NoPass 应为 false（设置了密码）")
	}
	if info.Passwords != 1 {
		t.Errorf("Passwords = %d, want 1", info.Passwords)
	}
	if info.Keys != "~zp:acl:*" {
		t.Errorf("Keys = %q, want ~zp:acl:*", info.Keys)
	}
	if !strings.Contains(info.Commands, "+@read") {
		t.Errorf("Commands = %q, want 含 +@read", info.Commands)
	}

	// 更新：关闭 + 清空密码（resetpass）
	if err := s.AclSetUser(connID, 0, user, []string{"off", "resetpass"}); err != nil {
		t.Fatalf("AclSetUser 更新: %v", err)
	}
	info, err = s.AclGetUser(connID, 0, user)
	if err != nil {
		t.Fatal(err)
	}
	if info.Enabled {
		t.Error("off 后 Enabled 应为 false")
	}
	if info.Passwords != 0 {
		t.Errorf("resetpass 后 Passwords = %d, want 0", info.Passwords)
	}

	// 删除
	if err := s.AclDelUser(connID, 0, user); err != nil {
		t.Fatalf("AclDelUser: %v", err)
	}
	if _, err := s.AclGetUser(connID, 0, user); err == nil {
		t.Error("删除后 GETUSER 应报错")
	}
}
