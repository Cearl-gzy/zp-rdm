//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"zp-rdm/internal/updatecheck"
)

// startUpdateHelper 以隐藏控制台窗口的方式启动更新辅助进程（PowerShell，
// 父进程退出后仍存活）。辅助进程等主进程退出后弹 UAC 提权静默安装，随后启动新版。
// 必须用 CREATE_NO_WINDOW（新建不可见控制台）：DETACHED_PROCESS 下 powershell.exe
// 无任何控制台，脚本体根本不执行（v1.14.1 真机踩坑，进程创建成功但静默即死）。
func startUpdateHelper(installer, targetDir, exeName string) error {
	name, args := updatecheck.BuildUpdateCommand(os.Getpid(), installer, targetDir, exeName)
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动更新程序失败: %w", err)
	}
	return nil
}
