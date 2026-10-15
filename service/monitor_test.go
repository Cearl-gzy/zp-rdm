package service

import (
	"bufio"
	"strings"
	"testing"
)

func TestWriteRESP(t *testing.T) {
	var sb strings.Builder
	w := bufio.NewWriter(&sb)
	if err := writeRESP(w, "SET", "key", "value"); err != nil {
		t.Fatal(err)
	}
	want := "*3\r\n$3\r\nSET\r\n$3\r\nkey\r\n$5\r\nvalue\r\n"
	if sb.String() != want {
		t.Fatalf("RESP = %q, want %q", sb.String(), want)
	}
}

func TestWriteRESPUTF8(t *testing.T) {
	var sb strings.Builder
	w := bufio.NewWriter(&sb)
	// $ 长度必须是字节数而非字符数
	if err := writeRESP(w, "GET", "中文键"); err != nil {
		t.Fatal(err)
	}
	want := "*2\r\n$3\r\nGET\r\n$9\r\n中文键\r\n"
	if sb.String() != want {
		t.Fatalf("RESP = %q, want %q", sb.String(), want)
	}
}

func TestReadRESPLine(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"+OK\r\n", "OK", false},
		{"+1697360000.123 [0 127.0.0.1] \"GET\" \"k\"\r\n", "1697360000.123 [0 127.0.0.1] \"GET\" \"k\"", false},
		{":12345\r\n", "12345", false},
		{"-WRONGPASS invalid password\r\n", "", true},
		{"-ERR unknown command\r\n", "", true},
	}
	for _, c := range cases {
		got, err := readRESPLine(bufio.NewReader(strings.NewReader(c.in)))
		if c.wantErr {
			if err == nil {
				t.Fatalf("readRESPLine(%q) expected error, got %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("readRESPLine(%q) error: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("readRESPLine(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestReadRESPLineErrorMessage(t *testing.T) {
	_, err := readRESPLine(bufio.NewReader(strings.NewReader("-WRONGPASS invalid username-password pair\r\n")))
	if err == nil || !strings.Contains(err.Error(), "WRONGPASS") {
		t.Fatalf("error should carry server message, got %v", err)
	}
}

func TestStartMonitorUnknownConn(t *testing.T) {
	m, _, _ := newTestManager(t)
	s := NewServerService(m)
	// 未打开的连接 ID 应返回错误而非 panic
	if err := s.StartMonitor("no-such-conn", 0); err == nil {
		t.Fatal("expected error for unknown connection")
	}
}
