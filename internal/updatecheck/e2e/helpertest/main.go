//go:build windows

// 更新辅助进程 E2E 验证（Windows CI / 真机手动）：
// 以生产同款 CREATE_NO_WINDOW 标志拉起 BuildUpdateCommand 生成的 PowerShell 辅助进程，
// 轮询辅助日志与假安装器日志，断言：
//  1. 脚本体真的执行了（helper started / launching installer / installer exited）
//     —— v1.14.1 的 DETACHED_PROCESS 事故就是进程创建成功但脚本体从未执行
//  2. 安装器确实收到了 /S /D= 参数（假安装器落盘）
//
// 用法：helpertest.exe <fakeinstaller.exe 路径>
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"zp-rdm/internal/updatecheck"
)

func fail(format string, args ...interface{}) {
	fmt.Printf("E2E FAIL: "+format+"\n", args...)
	os.Exit(1)
}

func main() {
	if len(os.Args) < 2 {
		fail("usage: helpertest <fakeinstaller.exe>")
	}
	installer, err := filepath.Abs(os.Args[1])
	if err != nil {
		fail("abs path: %v", err)
	}
	if _, err := os.Stat(installer); err != nil {
		fail("fake installer not found: %v", err)
	}

	// 目标目录：临时目录下放一个假的应用 exe 路径（不要求存在，
	// 辅助脚本最后的 Start-Process 有 SilentlyContinue 兜底）
	targetDir := filepath.Join(os.TempDir(), "zp-e2e-target")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		fail("mkdir: %v", err)
	}
	helperLog := installer + ".log"
	installLog := installer + ".install.log"
	_ = os.Remove(helperLog)
	_ = os.Remove(installLog)

	// 主进程 pid 用一个"即起即死"的进程：辅助脚本检测到 pid 不存在
	// （或路径不符，PID 复用护栏）后立即进入安装阶段，不用真等本进程退出
	dead := exec.Command("cmd.exe", "/c", "exit")
	if err := dead.Run(); err != nil {
		fail("spawn dead pid: %v", err)
	}

	name, args := updatecheck.BuildUpdateCommand(dead.Process.Pid, installer, targetDir, "zp-rdm.exe")
	cmd := exec.Command(name, args...)
	// 与 update_windows.go 生产路径一致：CREATE_NO_WINDOW。
	// 若有人回退到 DETACHED_PROCESS，脚本体不执行，本测试等不到日志而失败——这正是它的职责。
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	if err := cmd.Start(); err != nil {
		fail("start helper: %v", err)
	}

	// 轮询辅助日志（最多 90s；含 UAC 静默提权 + 假安装器 300ms 耗时）
	wantSteps := []string{"helper started", "launching installer", "installer exited"}
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(helperLog) //nolint:gosec 测试自产日志
		text := string(b)
		ok := true
		for _, s := range wantSteps {
			if !strings.Contains(text, s) {
				ok = false
				break
			}
		}
		if ok {
			// 再验证安装器侧收到的参数
			ib, _ := os.ReadFile(installLog) //nolint:gosec 测试自产日志
			if !strings.Contains(string(ib), "/S /D=") {
				fail("installer args missing /S /D=, got: %q", string(ib))
			}
			fmt.Printf("E2E PASS\n--- helper log ---\n%s--- installer args ---\n%s", text, string(ib))
			return
		}
		// 安装失败（如 UAC 被拒）直接报错，不等超时
		if strings.Contains(text, "install failed or cancelled") {
			fail("helper reported install failure:\n%s", text)
		}
		time.Sleep(500 * time.Millisecond)
	}
	b, _ := os.ReadFile(helperLog) //nolint:gosec 测试自产日志
	fail("timeout waiting for helper steps; helper log so far:\n%s", string(b))
}
