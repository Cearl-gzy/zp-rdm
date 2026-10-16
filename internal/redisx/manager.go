package redisx

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/ssh"

	"zp-rdm/internal/model"
)

type entry struct {
	profile model.ConnectionProfile
	ssh     *ssh.Client
	state   *model.ConnState
}

// Manager 管理所有打开的 Redis 连接（按 connID|db 缓存 client；集群连接固定 db 0）
type Manager struct {
	mu      sync.Mutex
	clients map[string]redis.UniversalClient
	entries map[string]*entry
}

func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]redis.UniversalClient),
		entries: make(map[string]*entry),
	}
}

func clientKey(connID string, db int) string { return fmt.Sprintf("%s|%d", connID, db) }

// Open 打开连接（测试连通性并缓存）
func (m *Manager) Open(ctx context.Context, p model.ConnectionProfile) (*model.ConnState, error) {
	m.Close(p.ID)

	var sshCli *ssh.Client
	if p.SSH.Enabled {
		var err error
		sshCli, err = dialSSH(p.SSH)
		if err != nil {
			return nil, fmt.Errorf("SSH 隧道连接失败: %w", err)
		}
	}

	c, err := m.newClient(p, p.DB, sshCli)
	if err != nil {
		if sshCli != nil {
			_ = sshCli.Close()
		}
		return nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := c.Ping(pingCtx).Err(); err != nil {
		_ = c.Close()
		if sshCli != nil {
			_ = sshCli.Close()
		}
		return nil, fmt.Errorf("连接失败: %w", err)
	}

	dbCount := 16
	if p.Cluster {
		dbCount = 1 // 集群模式只有 db 0
	} else if res, err := c.ConfigGet(ctx, "databases").Result(); err == nil {
		if v, ok := res["databases"]; ok {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				dbCount = n
			}
		}
	}
	version, mode := "", "standalone"
	if info, err := c.Info(ctx, "server").Result(); err == nil {
		version = parseInfoField(info, "redis_version")
		mode = parseInfoField(info, "redis_mode")
		if mode == "" {
			mode = "standalone"
		}
	}

	st := &model.ConnState{
		ID: p.ID, Name: p.Name, Group: p.Group, Color: p.Color,
		DBCount: dbCount, Version: version, Mode: mode,
		Modules: parseModuleNames(ctx, c),
	}

	m.mu.Lock()
	m.clients[clientKey(p.ID, p.DB)] = c
	m.entries[p.ID] = &entry{profile: p, ssh: sshCli, state: st}
	m.mu.Unlock()
	return st, nil
}

// Test 测试连接（不缓存）
func (m *Manager) Test(ctx context.Context, p model.ConnectionProfile) error {
	var sshCli *ssh.Client
	if p.SSH.Enabled {
		var err error
		sshCli, err = dialSSH(p.SSH)
		if err != nil {
			return fmt.Errorf("SSH 隧道连接失败: %w", err)
		}
		defer sshCli.Close()
	}
	c, err := m.newClient(p, p.DB, sshCli)
	if err != nil {
		return err
	}
	defer c.Close()
	tctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return c.Ping(tctx).Err()
}

func (m *Manager) newClient(p model.ConnectionProfile, db int, sshCli *ssh.Client) (redis.UniversalClient, error) {
	var tlsCfg *tls.Config
	if p.UseTLS {
		tc, err := buildTLSConfig(p)
		if err != nil {
			return nil, err
		}
		tlsCfg = tc
	}
	if p.Cluster {
		opts := &redis.ClusterOptions{
			Addrs:        []string{fmt.Sprintf("%s:%d", p.Host, p.Port)},
			Username:     p.Username,
			Password:     p.Password,
			DialTimeout:  8 * time.Second,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			TLSConfig:    tlsCfg,
		}
		if sshCli != nil {
			opts.Dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialThroughSSH(sshCli, network, addr)
			}
		}
		return redis.NewClusterClient(opts), nil
	}
	opts := &redis.Options{
		Addr:         fmt.Sprintf("%s:%d", p.Host, p.Port),
		Username:     p.Username,
		Password:     p.Password,
		DB:           db,
		DialTimeout:  8 * time.Second,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		TLSConfig:    tlsCfg,
	}
	if sshCli != nil {
		opts.Dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialThroughSSH(sshCli, network, addr)
		}
	}
	return redis.NewClient(opts), nil
}

// buildTLSConfig 按连接配置构建 tls.Config。
// 默认跳过证书校验（兼容旧行为）；提供自定义 CA 时可关闭跳过做严格校验；
// 客户端证书/私钥同时提供时启用 mTLS。
func buildTLSConfig(p model.ConnectionProfile) (*tls.Config, error) {
	// TLSSkipVerify 为 nil 的旧配置视为跳过（历史行为就是 InsecureSkipVerify）
	skip := p.TLSSkipVerify == nil || *p.TLSSkipVerify
	tc := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: skip, //nolint:gosec 用户显式选择
	}
	if ca := strings.TrimSpace(p.TLSCaCert); ca != "" {
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM([]byte(ca)) {
			return nil, errors.New("CA 证书 PEM 解析失败（未包含有效证书）")
		}
		tc.RootCAs = pool
	}
	cert, key := strings.TrimSpace(p.TLSClientCert), strings.TrimSpace(p.TLSClientKey)
	if (cert == "") != (key == "") {
		return nil, errors.New("mTLS 需要同时提供客户端证书与私钥")
	}
	if cert != "" {
		pair, err := tls.X509KeyPair([]byte(cert), []byte(key))
		if err != nil {
			return nil, fmt.Errorf("客户端证书/私钥解析失败: %w", err)
		}
		tc.Certificates = []tls.Certificate{pair}
	}
	return tc, nil
}

// parseModuleNames 解析 MODULE LIST 返回的模块名列表；命令失败（老版本/受限）时返回空切片
func parseModuleNames(ctx context.Context, c redis.UniversalClient) []string {
	names := []string{}
	res, err := c.Do(ctx, "MODULE", "LIST").Result()
	if err != nil {
		return names
	}
	list, ok := res.([]interface{})
	if !ok {
		return names
	}
	for _, item := range list {
		m, ok := item.(map[interface{}]interface{})
		if !ok {
			continue
		}
		if name, ok := m["name"].(string); ok && name != "" {
			names = append(names, name)
		}
	}
	return names
}

// dialThroughSSH 通过 SSH 通道拨号，并用 net.Pipe 桥接后返回。
// SSH 通道不支持 SetDeadline（go-redis 依赖它实现读/写超时），
// 因此把支持 deadline 的 net.Pipe 一端交给 go-redis，后台做双向转发。
func dialThroughSSH(sshCli *ssh.Client, network, addr string) (net.Conn, error) {
	rc, err := sshCli.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	cli, pipe := net.Pipe()
	go proxyAndClose(rc, pipe) // 客户端写入 → ssh
	go proxyAndClose(pipe, rc) // ssh 返回 → 客户端读取
	return cli, nil
}

// proxyAndClose 把 src 的数据拷到 dst；任一端结束后两端一起关闭，
// 释放另一侧阻塞中的读写，避免 goroutine 泄漏。
func proxyAndClose(dst, src net.Conn) {
	_, _ = io.Copy(dst, src)
	_ = dst.Close()
	_ = src.Close()
}

// Get 获取指定 db 的 client（懒创建；集群连接忽略 db，固定返回 db 0 的集群 client）
func (m *Manager) Get(connID string, db int) (redis.UniversalClient, error) {
	m.mu.Lock()
	e, ok := m.entries[connID]
	if ok && e.profile.Cluster {
		db = 0
	}
	k := clientKey(connID, db)
	if c, hit := m.clients[k]; hit {
		m.mu.Unlock()
		return c, nil
	}
	m.mu.Unlock()
	if !ok {
		return nil, errors.New("连接未打开")
	}
	c, err := m.newClient(e.profile, db, e.ssh)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.clients[k] = c
	m.mu.Unlock()
	return c, nil
}

// IsCluster 连接是否为集群模式（未打开返回 false）
func (m *Manager) IsCluster(connID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[connID]; ok {
		return e.profile.Cluster
	}
	return false
}

// State 返回连接状态（未打开返回 nil）
func (m *Manager) State(connID string) *model.ConnState {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[connID]; ok {
		return e.state
	}
	return nil
}

// OpenStates 返回所有已打开连接状态
func (m *Manager) OpenStates() []model.ConnState {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.ConnState, 0, len(m.entries))
	for _, e := range m.entries {
		out = append(out, *e.state)
	}
	return out
}

// IsOpen 是否已打开
func (m *Manager) IsOpen(connID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.entries[connID]
	return ok
}

// Close 关闭连接（含全部 db 的 client 与 SSH 隧道）
func (m *Manager) Close(connID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, c := range m.clients {
		if strings.HasPrefix(k, connID+"|") {
			_ = c.Close()
			delete(m.clients, k)
		}
	}
	if e, ok := m.entries[connID]; ok {
		if e.ssh != nil {
			_ = e.ssh.Close()
		}
		delete(m.entries, connID)
	}
}

func dialSSH(cfg model.SSHConfig) (*ssh.Client, error) {
	var auth []ssh.AuthMethod
	if cfg.AuthType == "key" {
		keyData := []byte(cfg.PrivateKey)
		if fi, err := os.Stat(cfg.PrivateKey); err == nil && !fi.IsDir() {
			if b, err := os.ReadFile(cfg.PrivateKey); err == nil {
				keyData = b
			}
		}
		var signer ssh.Signer
		var err error
		if cfg.Passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(cfg.Passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(keyData)
		}
		if err != nil {
			return nil, fmt.Errorf("私钥解析失败: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	} else {
		auth = append(auth, ssh.Password(cfg.Password))
	}
	port := cfg.Port
	if port <= 0 {
		port = 22
	}
	config := &ssh.ClientConfig{
		User:            cfg.Username,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec 桌面工具场景
		Timeout:         10 * time.Second,
	}
	return ssh.Dial("tcp", fmt.Sprintf("%s:%d", cfg.Host, port), config)
}

func parseInfoField(info, field string) string {
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, field+":") {
			return strings.TrimPrefix(line, field+":")
		}
	}
	return ""
}
