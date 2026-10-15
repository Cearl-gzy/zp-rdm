package service

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func newValueService(t *testing.T) (*ValueService, *redis.Client, string) {
	m, seed, connID := newTestManager(t)
	s := NewValueService(m)
	s.Startup(context.Background())
	return s, seed, connID
}

func TestIsBinaryString(t *testing.T) {
	cases := []struct {
		in     string
		binary bool
	}{
		{"hello world", false},
		{`{"a":1,"b":[1,2,3]}`, false},
		{"中文内容 UTF-8", false},
		{"with\ttab\nnewline\rcr", false},
		{"has\x00nul", true},
		{"ctrl\x01char", true},
		{"\xff\xfe invalid utf8", true},
		{"\x00", true},
		{"", false},
	}
	for _, c := range cases {
		if got := isBinaryString(c.in); got != c.binary {
			t.Errorf("isBinaryString(%q) = %v, want %v", c.in, got, c.binary)
		}
	}
}

func TestGetValueString(t *testing.T) {
	s, c, connID := newValueService(t)
	mustSet(t, c, "k1", "hello")
	d, err := s.GetValue(connID, 0, "k1")
	if err != nil {
		t.Fatal(err)
	}
	if d.Type != "string" || d.StrValue != "hello" {
		t.Fatalf("unexpected detail: %+v", d)
	}
	if d.Binary {
		t.Fatal("plain text misdetected as binary")
	}
	if d.Truncated {
		t.Fatal("small value should not be truncated")
	}
	if d.Length != 5 {
		t.Fatalf("length = %d, want 5", d.Length)
	}
}

func TestGetValueBinaryFlag(t *testing.T) {
	s, c, connID := newValueService(t)
	mustSet(t, c, "bin", "a\x00b\xffc")
	d, err := s.GetValue(connID, 0, "bin")
	if err != nil {
		t.Fatal(err)
	}
	if !d.Binary {
		t.Fatal("binary value not detected")
	}
}

func TestGetValueMissingKey(t *testing.T) {
	s, _, connID := newValueService(t)
	if _, err := s.GetValue(connID, 0, "no-such-key"); err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestStringHexRoundTrip(t *testing.T) {
	s, _, connID := newValueService(t)
	raw := []byte{0x00, 0x01, 0xff, 0xfe, 0x41, 0x42}
	if err := s.SetStringHex(connID, 0, "hexk", hex.EncodeToString(raw), 0); err != nil {
		t.Fatalf("SetStringHex: %v", err)
	}
	got, err := s.GetStringHex(connID, 0, "hexk")
	if err != nil {
		t.Fatalf("GetStringHex: %v", err)
	}
	if got != hex.EncodeToString(raw) {
		t.Fatalf("round trip = %q, want %q", got, hex.EncodeToString(raw))
	}
	// 二进制值应被标记
	d, err := s.GetValue(connID, 0, "hexk")
	if err != nil {
		t.Fatal(err)
	}
	if !d.Binary {
		t.Fatal("hex-written binary not flagged")
	}
}

func TestSetStringHexWithSpaces(t *testing.T) {
	s, _, connID := newValueService(t)
	// 编辑器传入的是空格分组的 hex 流，应能正确解析
	if err := s.SetStringHex(connID, 0, "k", "de ad be ef", 0); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetStringHex(connID, 0, "k")
	if got != "deadbeef" {
		t.Fatalf("got %q, want deadbeef", got)
	}
}

func TestSetStringHexInvalid(t *testing.T) {
	s, _, connID := newValueService(t)
	if err := s.SetStringHex(connID, 0, "k", "zz-not-hex", 0); err == nil {
		t.Fatal("expected error for invalid hex")
	}
	if err := s.SetStringHex(connID, 0, "k", "abc", 0); err == nil {
		t.Fatal("expected error for odd-length hex")
	}
}

func TestSetStringKeepTTL(t *testing.T) {
	s, c, connID := newValueService(t)
	mustSet(t, c, "k", "v1")
	mustExpire(t, c, "k", 2*time.Hour)

	// ttl<0 → 保持原 TTL
	if err := s.SetString(connID, 0, "k", "v2", -1); err != nil {
		t.Fatal(err)
	}
	if v := mustGet(t, c, "k"); v != "v2" {
		t.Fatalf("k = %q, want v2", v)
	}
	if ttl := mustTTL(t, c, "k"); ttl < 110*time.Minute {
		t.Fatalf("TTL lost after KeepTTL set: %v", ttl)
	}
}

func TestSetStringWithTTL(t *testing.T) {
	s, c, connID := newValueService(t)
	if err := s.SetString(connID, 0, "k", "v", 60); err != nil {
		t.Fatal(err)
	}
	if ttl := mustTTL(t, c, "k"); ttl < 50*time.Second || ttl > 60*time.Second {
		t.Fatalf("ttl = %v, want ~60s", ttl)
	}
}

func TestGetFullString(t *testing.T) {
	s, c, connID := newValueService(t)
	big := strings.Repeat("x", 2<<20) // 2MB，超过预览 1MB 截断
	mustSet(t, c, "big", big)

	// 预览应截断
	d, err := s.GetValue(connID, 0, "big")
	if err != nil {
		t.Fatal(err)
	}
	if !d.Truncated {
		t.Fatal("2MB value should be truncated in preview")
	}
	if len(d.StrValue) != 1<<20 {
		t.Fatalf("preview len = %d, want 1MB", len(d.StrValue))
	}

	// 完整加载
	full, err := s.GetFullString(connID, 0, "big")
	if err != nil {
		t.Fatal(err)
	}
	if len(full) != 2<<20 {
		t.Fatalf("full len = %d, want 2MB", len(full))
	}
}

func TestTTLSentinels(t *testing.T) {
	// go-redis 对 TTL -1（永久）/ -2（不存在）返回纳秒级哨兵，
	// int64(d.Seconds()) 会错误得到 0，必须经过 ttlSeconds/ttlMillis 规范化
	if got := ttlSeconds(time.Duration(-1)); got != -1 {
		t.Fatalf("ttlSeconds(-1ns sentinel) = %d, want -1", got)
	}
	if got := ttlSeconds(time.Duration(-2)); got != -1 {
		t.Fatalf("ttlSeconds(-2ns sentinel) = %d, want -1", got)
	}
	if got := ttlMillis(time.Duration(-1)); got != -1 {
		t.Fatalf("ttlMillis(-1ns sentinel) = %d, want -1", got)
	}
	if got := ttlSeconds(90 * time.Second); got != 90 {
		t.Fatalf("ttlSeconds(90s) = %d, want 90", got)
	}
	if got := ttlMillis(1500 * time.Millisecond); got != 1500 {
		t.Fatalf("ttlMillis(1500ms) = %d, want 1500", got)
	}
	// 永久键 TTL 端到端：GetValue 应报告 -1 而非 0
	s, c, connID := newValueService(t)
	mustSet(t, c, "perm", "v")
	d, err := s.GetValue(connID, 0, "perm")
	if err != nil {
		t.Fatal(err)
	}
	if d.TTL != -1 {
		t.Fatalf("permanent key TTL = %d, want -1", d.TTL)
	}
}
