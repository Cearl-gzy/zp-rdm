package service

import (
	"crypto/rand"
	"encoding/hex"
	"time"
	"unicode/utf8"
)

func durationSeconds(s int64) time.Duration { return time.Duration(s) * time.Second }

// ttlSeconds 将 go-redis 的 TTL 结果规范化为秒。
// go-redis 对 -1（永久）/ -2（不存在）返回纳秒级哨兵值，
// 直接 .Seconds() 再取整会得到 0，必须显式判断负数。
func ttlSeconds(d time.Duration) int64 {
	if d < 0 {
		return -1
	}
	return int64(d.Seconds())
}

// ttlMillis 同上，规范化为毫秒
func ttlMillis(d time.Duration) int64 {
	if d < 0 {
		return -1
	}
	return d.Milliseconds()
}

// isBinaryString 判断是否为二进制内容：无效 UTF-8，或前 8KB 内含 NUL / 非常用控制字符
func isBinaryString(s string) bool {
	if s == "" {
		return false
	}
	if !utf8.ValidString(s) {
		return true
	}
	n := len(s)
	if n > 8192 {
		n = 8192
	}
	for i := 0; i < n; i++ {
		b := s[i]
		if b == 0 {
			return true
		}
		if b < 0x20 && b != '\t' && b != '\n' && b != '\r' {
			return true
		}
	}
	return false
}

func nowMillis() int64 { return time.Now().UnixMilli() }

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
