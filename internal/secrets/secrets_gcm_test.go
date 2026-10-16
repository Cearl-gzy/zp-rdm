//go:build darwin || linux

package secrets

import (
	"strings"
	"testing"
)

// 完整往返：密钥环（或文件回退）+ AES-GCM 加解密
func TestGCMRoundTrip(t *testing.T) {
	enc, err := Protect("my-secret-密码")
	if err != nil {
		t.Fatalf("Protect: %v", err)
	}
	if !IsProtected(enc) {
		t.Fatalf("ciphertext should have gcm prefix, got %q", enc)
	}
	if strings.Contains(enc, "my-secret") {
		t.Fatal("ciphertext leaks plaintext")
	}
	if got := Unprotect(enc); got != "my-secret-密码" {
		t.Fatalf("Unprotect = %q", got)
	}
}

func TestGCMEmptyPassthrough(t *testing.T) {
	enc, err := Protect("")
	if err != nil || enc != "" {
		t.Fatalf("empty Protect = %q, %v", enc, err)
	}
}

func TestGCMLegacyPlaintextPassthrough(t *testing.T) {
	if got := Unprotect("old-plain-password"); got != "old-plain-password" {
		t.Fatalf("legacy plaintext = %q", got)
	}
	if IsProtected("old-plain-password") {
		t.Fatal("plaintext marked protected")
	}
}

func TestGCMTampered(t *testing.T) {
	enc, err := Protect("data")
	if err != nil {
		t.Fatalf("Protect: %v", err)
	}
	// 篡改最后一个字符（base64 内）
	tampered := enc[:len(enc)-2] + "XX"
	if got := Unprotect(tampered); got != "" {
		t.Fatalf("tampered ciphertext should decrypt to empty, got %q", got)
	}
}

// 两次加密同一明文，密文应不同（随机 nonce）
func TestGCMRandomNonce(t *testing.T) {
	a, err1 := Protect("same")
	b, err2 := Protect("same")
	if err1 != nil || err2 != nil {
		t.Fatalf("Protect: %v %v", err1, err2)
	}
	if a == b {
		t.Fatal("identical ciphertexts for same plaintext (nonce reuse?)")
	}
	if Unprotect(a) != "same" || Unprotect(b) != "same" {
		t.Fatal("roundtrip failed")
	}
}
