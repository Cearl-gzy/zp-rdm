package service

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
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

	opt := c.Options()
	dialer := opt.Dialer
	if dialer == nil {
		d := &net.Dialer{Timeout: 8 * time.Second}
		dialer = d.DialContext
	}
	raw, err := dialer(s.ctx, opt.Network, opt.Addr)
	if err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}
	if opt.TLSConfig != nil {
		tc := tls.Client(raw, opt.TLSConfig)
		if err := tc.Handshake(); err != nil {
			_ = raw.Close()
			return fmt.Errorf("TLS 握手失败: %w", err)
		}
		raw = tc
	}

	r := bufio.NewReader(raw)
	w := bufio.NewWriter(raw)
	fail := func(err error) error {
		_ = raw.Close()
		return err
	}
	// 认证 + 选库 + MONITOR
	if opt.Password != "" {
		if opt.Username != "" {
			err = writeRESP(w, "AUTH", opt.Username, opt.Password)
		} else {
			err = writeRESP(w, "AUTH", opt.Password)
		}
		if err != nil {
			return fail(err)
		}
		if _, err := readRESPLine(r); err != nil {
			return fail(fmt.Errorf("认证失败: %w", err))
		}
	}
	if opt.DB != 0 {
		if err := writeRESP(w, "SELECT", fmt.Sprintf("%d", opt.DB)); err != nil {
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
			wailsRuntime.EventsEmit(s.ctx, eventName, line)
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
