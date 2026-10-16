package redisx

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"zp-rdm/internal/model"
)

// SSH 隧道集成测试（冒烟）。
// 需要一台真实 sshd + Redis，默认走本机 WSL：
//
//	sshd  : localhost:2222（用户 zpssh / 密码 zpssh123）
//	Redis : 127.0.0.1:6379（从 sshd 所在主机的视角，即 WSL 内）
//
// 未设置 ZP_RDM_TEST_SSH_ADDR 时跳过。可用以下环境变量覆盖：
//
//	ZP_RDM_TEST_SSH_ADDR   sshd 地址（host:port）
//	ZP_RDM_TEST_SSH_USER   SSH 用户名
//	ZP_RDM_TEST_SSH_PASS   SSH 密码
//	ZP_RDM_TEST_SSH_REDIS  从 sshd 主机视角看到的 Redis 地址（host:port）
func sshTestProfile(t *testing.T) (model.ConnectionProfile, bool) {
	t.Helper()
	addr := os.Getenv("ZP_RDM_TEST_SSH_ADDR")
	if addr == "" {
		return model.ConnectionProfile{}, false
	}
	user := os.Getenv("ZP_RDM_TEST_SSH_USER")
	if user == "" {
		user = "zpssh"
	}
	pass := os.Getenv("ZP_RDM_TEST_SSH_PASS")
	if pass == "" {
		pass = "zpssh123"
	}
	redisAddr := os.Getenv("ZP_RDM_TEST_SSH_REDIS")
	if redisAddr == "" {
		redisAddr = "127.0.0.1:6379"
	}
	sshHost, sshPort := splitHostPort(t, addr)
	rHost, rPort := splitHostPort(t, redisAddr)
	return model.ConnectionProfile{
		ID:   "test-ssh-conn",
		Name: "ssh-smoke",
		Host: rHost,
		Port: rPort,
		SSH: model.SSHConfig{
			Enabled:  true,
			Host:     sshHost,
			Port:     sshPort,
			Username: user,
			AuthType: "password",
			Password: pass,
		},
	}, true
}

func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, ok := strings.Cut(addr, ":")
	if !ok || host == "" {
		t.Fatalf("地址格式错误 %q，需要 host:port", addr)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 {
		t.Fatalf("端口格式错误 %q", addr)
	}
	return host, port
}

func TestSSHTunnelSmoke(t *testing.T) {
	p, ok := sshTestProfile(t)
	if !ok {
		t.Skip("未设置 ZP_RDM_TEST_SSH_ADDR，跳过 SSH 隧道冒烟测试")
	}

	m := NewManager()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st, err := m.Open(ctx, p)
	if err != nil {
		t.Fatalf("SSH 隧道连接失败: %v", err)
	}
	defer m.Close(p.ID)

	if st.Version == "" {
		t.Error("连接成功但未取到 Redis 版本号")
	}

	c, err := m.Get(p.ID, 0)
	if err != nil {
		t.Fatalf("取 client 失败: %v", err)
	}
	key := "zp-rdm:ssh-smoke:key"
	if err := c.Set(ctx, key, "via-ssh-tunnel", 30*time.Second).Err(); err != nil {
		t.Fatalf("经隧道 SET 失败: %v", err)
	}
	defer c.Del(ctx, key) //nolint:errcheck
	if v, err := c.Get(ctx, key).Result(); err != nil || v != "via-ssh-tunnel" {
		t.Errorf("经隧道 GET 结果不符: v=%q err=%v", v, err)
	}
}

func TestSSHTunnelWrongPassword(t *testing.T) {
	p, ok := sshTestProfile(t)
	if !ok {
		t.Skip("未设置 ZP_RDM_TEST_SSH_ADDR，跳过 SSH 隧道冒烟测试")
	}
	p.SSH.Password = "definitely-wrong-password"

	m := NewManager()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := m.Open(ctx, p)
	if err == nil {
		m.Close(p.ID)
		t.Fatal("错误密码下 Open 竟然成功")
	}
	if !strings.Contains(err.Error(), "SSH 隧道连接失败") {
		t.Errorf("错误信息应包含「SSH 隧道连接失败」，实际: %v", err)
	}
}
