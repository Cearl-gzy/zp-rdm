//go:build !windows

package secrets

import "testing"

// 非 Windows 平台为透传桩（DPAPI 不可用），验证透传语义
func TestPassthroughStub(t *testing.T) {
	enc, err := Protect("plain-secret")
	if err != nil {
		t.Fatalf("Protect: %v", err)
	}
	if enc != "plain-secret" {
		t.Fatalf("stub should pass through unchanged, got %q", enc)
	}
	if IsProtected(enc) {
		t.Fatal("passthrough value should not be marked protected")
	}
	if got := Unprotect("plain-secret"); got != "plain-secret" {
		t.Fatalf("Unprotect passthrough = %q", got)
	}
}
