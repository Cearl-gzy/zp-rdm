package updatecheck

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.12.0", "1.12.0", 0},
		{"1.12.0", "1.13.0", -1},
		{"1.13.0", "1.12.0", 1},
		{"1.12.0", "1.12.1", -1},
		{"2.0.0", "1.99.9", 1},
		{"v1.12.0", "1.12.0", 0},     // v 前缀容忍
		{"1.12", "1.12.0", 0},        // 段数不足补 0
		{"1.12.0", "1.12.0.1", 0},    // 第四段忽略
		{"1.13.0-beta", "1.13.0", 0}, // 预发布标识按 0 截断比较
	}
	for _, c := range cases {
		if got := CompareSemver(c.a, c.b); got != c.want {
			t.Errorf("CompareSemver(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestVerifyFileSHA256(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.bin")
	content := []byte("hello zp-rdm")
	if err := os.WriteFile(p, content, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	good := hex.EncodeToString(sum[:])

	if err := VerifyFileSHA256(p, good); err != nil {
		t.Fatalf("good hash rejected: %v", err)
	}
	if err := VerifyFileSHA256(p, "sha256:"+good); err != nil {
		t.Fatalf("prefixed hash rejected: %v", err)
	}
	if err := VerifyFileSHA256(p, strings.ToUpper(good)); err != nil {
		t.Fatalf("upper hash rejected: %v", err)
	}
	if err := VerifyFileSHA256(p, ""); err != nil {
		t.Fatalf("empty want should skip verify: %v", err)
	}
	bad := strings.Repeat("0", 64)
	if err := VerifyFileSHA256(p, bad); err == nil {
		t.Fatal("bad hash accepted")
	}
	if err := VerifyFileSHA256(filepath.Join(dir, "nope"), good); err == nil {
		t.Fatal("missing file accepted")
	}
}

func TestBuildUpdateBat(t *testing.T) {
	bat := BuildUpdateBat(`C:\Users\x\AppData\Local\Temp\zp-rdm-update-1.13.0.exe`, `C:\Users\x\AppData\Local\zp-rdm`, "zp-rdm.exe")
	for _, want := range []string{
		"tasklist /FI \"PID eq %PID%\"",
		"/S /D=C:\\Users\\x\\AppData\\Local\\zp-rdm\r\n", // /D 必须是最后参数且无引号
		"\\zp-rdm.exe\"\r\n",
		"del /q \"%~f0\"",
	} {
		if !strings.Contains(bat, want) {
			t.Fatalf("bat missing %q:\n%s", want, bat)
		}
	}
	// 静默参数存在
	if !strings.Contains(bat, " /S ") {
		t.Fatal("missing NSIS silent switch")
	}
}
