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

func TestBuildUpdateCommand(t *testing.T) {
	name, args := BuildUpdateCommand(20948,
		`C:\Users\x\AppData\Local\Temp\zp-rdm-update-1.14.1.exe`,
		`D:\Program Files\ZP\ZP Redis Manager`, "zp-rdm.exe")
	if name != "powershell.exe" {
		t.Fatalf("unexpected program: %s", name)
	}
	if len(args) < 2 || args[len(args)-2] != "-Command" {
		t.Fatalf("missing -Command flag: %v", args)
	}
	script := args[len(args)-1]
	for _, want := range []string{
		"$appPid = 20948",                              // 等待主进程退出
		"Get-Process -Id $appPid",                      // 轮询进程存在性（替代易死锁的 tasklist|find 管道）
		"-Verb RunAs -Wait",                            // UAC 提权（安装器 requireAdministrator）并等待完成
		`'/S /D=D:\Program Files\ZP\ZP Redis Manager'`, // /D 最后参数且无引号（NSIS 硬性要求）
		`'C:\Users\x\AppData\Local\Temp\zp-rdm-update-1.14.1.exe'`,
		`'D:\Program Files\ZP\ZP Redis Manager\zp-rdm.exe'`, // 装完启动新版
		"Remove-Item",                                       // 清理安装包
		"catch {}",                                          // 拒绝 UAC 时不中断，回退启动旧版
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q:\n%s", want, script)
		}
	}
	// 安装在前、重启在后
	if strings.Index(script, "/S /D=") > strings.Index(script, "Start-Process -FilePath $exe") {
		t.Fatalf("relaunch must come after install:\n%s", script)
	}
}

func TestBuildUpdateCommandQuoting(t *testing.T) {
	// 路径含单引号时按 PowerShell 规则双写转义
	_, args := BuildUpdateCommand(1, `C:\it's here\setup.exe`, `C:\dir`, "a.exe")
	script := args[len(args)-1]
	if !strings.Contains(script, `'C:\it''s here\setup.exe'`) {
		t.Fatalf("single quote not escaped:\n%s", script)
	}
}
