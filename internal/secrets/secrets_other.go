//go:build !windows && !darwin && !linux

// Package secrets 非主流桌面平台的降级实现：不做加密，原样透传。
package secrets

// Protect 原样返回（非 Windows 平台不加密）
func Protect(plain string) (string, error) { return plain, nil }

// Unprotect 原样返回
func Unprotect(s string) string { return s }

// IsProtected 恒为 false
func IsProtected(s string) bool { return false }
