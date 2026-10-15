//go:build windows

package secrets

import (
	"strings"
	"testing"
)

func TestProtectUnprotectRoundTrip(t *testing.T) {
	cases := []string{
		"simple-password",
		"带中文的密码@123",
		strings.Repeat("long", 500),
		`!@#$%^&*()_+-=[]{}|;':",./<>?` + "`~",
	}
	for _, plain := range cases {
		enc, err := Protect(plain)
		if err != nil {
			t.Fatalf("Protect(%q) error: %v", plain, err)
		}
		if !IsProtected(enc) {
			t.Fatalf("Protect(%q) result missing dpapi: prefix", plain)
		}
		if enc == plain {
			t.Fatalf("Protect(%q) did not change the value", plain)
		}
		if got := Unprotect(enc); got != plain {
			t.Fatalf("round trip failed: got %q want %q", got, plain)
		}
	}
}

func TestUnprotectLegacyPlaintext(t *testing.T) {
	// 旧版明文（无 dpapi: 前缀）应原样返回，保证向后兼容
	if got := Unprotect("plain-secret"); got != "plain-secret" {
		t.Fatalf("legacy plaintext changed: %q", got)
	}
}

func TestUnprotectGarbage(t *testing.T) {
	// 有前缀但内容非法（如来自其他机器的密文）应返回空串而非崩溃
	if got := Unprotect("dpapi:not-valid-base64!!!"); got != "" {
		t.Fatalf("expected empty for undecryptable value, got %q", got)
	}
	if got := Unprotect("dpapi:aGVsbG8="); got != "" {
		t.Fatalf("expected empty for non-DPAPI blob, got %q", got)
	}
}

func TestIsProtected(t *testing.T) {
	if IsProtected("plain") {
		t.Fatal("plain value misdetected as protected")
	}
	if !IsProtected("dpapi:AAAA") {
		t.Fatal("prefixed value not detected as protected")
	}
}

func TestProtectEmpty(t *testing.T) {
	// 空字符串不加密，直接返回（调用方约定）
	enc, err := Protect("")
	if err != nil {
		t.Fatalf("Protect(\"\") error: %v", err)
	}
	if enc != "" {
		t.Fatalf("Protect(\"\") = %q, want empty", enc)
	}
}
