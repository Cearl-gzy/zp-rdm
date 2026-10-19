//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// startUpdateBat 以脱离进程组的方式启动更新辅助批处理（父进程退出后仍存活）
func startUpdateBat(batPath string) error {
	cmd := exec.Command("cmd", "/C", batPath, fmt.Sprintf("%d", os.Getpid()))
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000008 | 0x00000200, // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动更新程序失败: %w", err)
	}
	return nil
}
