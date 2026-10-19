//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"zp-rdm/internal/updatecheck"
)

// startUpdateHelper 以脱离进程组的方式启动更新辅助进程（PowerShell 隐藏窗口，
// 父进程退出后仍存活）。辅助进程等主进程退出后弹 UAC 提权静默安装，随后启动新版。
func startUpdateHelper(installer, targetDir, exeName string) error {
	name, args := updatecheck.BuildUpdateCommand(os.Getpid(), installer, targetDir, exeName)
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000008 | 0x00000200, // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动更新程序失败: %w", err)
	}
	return nil
}
