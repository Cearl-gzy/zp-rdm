//go:build windows

// Package secrets 使用 Windows DPAPI 保护敏感字段（连接密码、SSH 密钥等）。
// 密文绑定当前 Windows 用户，无需主密码；换机/换用户后无法解密。
package secrets

import (
	"encoding/base64"
	"unsafe"

	"golang.org/x/sys/windows"
)

const prefix = "dpapi:"

type dataBlob struct {
	cbData uint32
	pbData *byte
}

var (
	crypt32       = windows.NewLazySystemDLL("crypt32.dll")
	procProtect   = crypt32.NewProc("CryptProtectData")
	procUnprotect = crypt32.NewProc("CryptUnprotectData")
)

func blobToBytes(b *dataBlob) []byte {
	out := make([]byte, b.cbData)
	copy(out, unsafe.Slice(b.pbData, b.cbData))
	return out
}

// Protect 加密字符串，返回带前缀的 base64 密文；空串原样返回
func Protect(plain string) (string, error) {
	if plain == "" {
		return plain, nil
	}
	raw := []byte(plain)
	in := dataBlob{cbData: uint32(len(raw)), pbData: &raw[0]}
	var out dataBlob
	r, _, err := procProtect.Call(
		uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return "", err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.pbData))) //nolint:errcheck
	return prefix + base64.StdEncoding.EncodeToString(blobToBytes(&out)), nil
}

// Unprotect 解密；非密文（旧版明文/他机导出）原样返回，解密失败返回空串
func Unprotect(s string) string {
	if s == "" || len(s) <= len(prefix) || s[:len(prefix)] != prefix {
		return s
	}
	raw, err := base64.StdEncoding.DecodeString(s[len(prefix):])
	if err != nil || len(raw) == 0 {
		return ""
	}
	in := dataBlob{cbData: uint32(len(raw)), pbData: &raw[0]}
	var out dataBlob
	r, _, _ := procUnprotect.Call(
		uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return ""
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.pbData))) //nolint:errcheck
	return string(blobToBytes(&out))
}

// IsProtected 判断是否为已加密密文
func IsProtected(s string) bool {
	return len(s) > len(prefix) && s[:len(prefix)] == prefix
}
