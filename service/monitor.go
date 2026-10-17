package service

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// MONITOR 命令流（go-redis 无公开流式 API，用裸连接 + 手写 RESP 实现）

// writeRESP 以 RESP 数组协议写入一条命令
func writeRESP(w *bufio.Writer, args ...string) error {
	if _, err := fmt.Fprintf(w, "*%d\r\n", len(args)); err != nil {
		return err
	}
	for _, a := range args {
		if _, err := fmt.Fprintf(w, "$%d\r\n%s\r\n", len(a), a); err != nil {
			return err
		}
	}
	return w.Flush()
}

// readRESPLine 读取一行简单回复（+OK / -ERR / :1 / $-1 等），返回内容
func readRESPLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return "", fmt.Errorf("空回复")
	}
	if line[0] == '-' {
		return "", fmt.Errorf("%s", line[1:])
	}
	return line[1:], nil
}

// openMonitorConn 建立一条已认证、已选库、已进入 MONITOR 模式的裸连接。
// 单机取 client 配置；集群取种子节点地址
// （MONITOR 只能看到单个节点的命令流，集群模式的已知限制）。
func openMonitorConn(ctx context.Context, c redis.UniversalClient, db int) (net.Conn, *bufio.Reader, error) {
	var addr, network, username, password string
	var dbSel int
	var dialer func(ctx context.Context, network, addr string) (net.Conn, error)
	var tlsCfg *tls.Config
	switch cli := c.(type) {
	case *redis.Client:
		opt := cli.Options()
		addr, network = opt.Addr, opt.Network
		username, password, dbSel = opt.Username, opt.Password, opt.DB
		dialer, tlsCfg = opt.Dialer, opt.TLSConfig
	case *redis.ClusterClient:
		opt := cli.Options()
		addr, network = opt.Addrs[0], "tcp"
		username, password = opt.Username, opt.Password
		dialer, tlsCfg = opt.Dialer, opt.TLSConfig
	default:
		return nil, nil, fmt.Errorf("不支持的客户端类型 %T", c)
	}
	if dbSel == 0 {
		dbSel = db // 集群模式无 SELECT（仅 db 0）；单机按调用方要求选库
	}
	if dialer == nil {
		d := &net.Dialer{Timeout: 8 * time.Second}
		dialer = d.DialContext
	}
	raw, err := dialer(ctx, network, addr)
	if err != nil {
		return nil, nil, fmt.Errorf("连接失败: %w", err)
	}
	if tlsCfg != nil {
		tc := tls.Client(raw, tlsCfg)
		if err := tc.Handshake(); err != nil {
			_ = raw.Close()
			return nil, nil, fmt.Errorf("TLS 握手失败: %w", err)
		}
		raw = tc
	}

	r := bufio.NewReader(raw)
	w := bufio.NewWriter(raw)
	fail := func(err error) (net.Conn, *bufio.Reader, error) {
		_ = raw.Close()
		return nil, nil, err
	}
	// 认证 + 选库 + MONITOR
	if password != "" {
		if username != "" {
			err = writeRESP(w, "AUTH", username, password)
		} else {
			err = writeRESP(w, "AUTH", password)
		}
		if err != nil {
			return fail(err)
		}
		if _, err := readRESPLine(r); err != nil {
			return fail(fmt.Errorf("认证失败: %w", err))
		}
	}
	if dbSel != 0 {
		if err := writeRESP(w, "SELECT", fmt.Sprintf("%d", dbSel)); err != nil {
			return fail(err)
		}
		if _, err := readRESPLine(r); err != nil {
			return fail(fmt.Errorf("SELECT 失败: %w", err))
		}
	}
	if err := writeRESP(w, "MONITOR"); err != nil {
		return fail(err)
	}
	if _, err := readRESPLine(r); err != nil { // MONITOR 的 +OK
		return fail(fmt.Errorf("MONITOR 启动失败: %w", err))
	}
	return raw, r, nil
}

// StartMonitor 启动 MONITOR，命令流经事件 monitor:connID:db 逐行推送
func (s *ServerService) StartMonitor(connID string, db int) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	monKey := fmt.Sprintf("%s|%d", connID, db)
	s.mu.Lock()
	if _, ok := s.monitors[monKey]; ok {
		s.mu.Unlock()
		return fmt.Errorf("MONITOR 已在运行")
	}
	s.mu.Unlock()

	raw, r, err := openMonitorConn(s.ctx, c, db)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.monitors[monKey] = raw
	s.mu.Unlock()

	eventName := fmt.Sprintf("monitor:%s:%d", connID, db)
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.monitors, monKey)
			s.mu.Unlock()
			_ = raw.Close()
		}()
		for {
			line, err := readRESPLine(r)
			if err != nil {
				return // 连接被 StopMonitor 关闭或服务端断开
			}
			emit(s.ctx, eventName, line)
		}
	}()
	return nil
}

// StopMonitor 停止 MONITOR
func (s *ServerService) StopMonitor(connID string, db int) {
	monKey := fmt.Sprintf("%s|%d", connID, db)
	s.mu.Lock()
	raw, ok := s.monitors[monKey]
	delete(s.monitors, monKey)
	s.mu.Unlock()
	if ok {
		_ = raw.Close() // 读循环因连接关闭退出
	}
}
