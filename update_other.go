//go:build !windows

package main

import "fmt"

// startUpdateBat 非 Windows 不支持自动安装（macOS 用 brew / Linux 用包管理器）
func startUpdateBat(string) error {
	return fmt.Errorf("当前平台不支持自动安装")
}
