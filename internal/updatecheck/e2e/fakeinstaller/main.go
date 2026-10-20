// 假安装器：更新辅助进程 E2E 的替身 NSIS 安装器。
// 把收到的命令行参数写入 <自身路径>.install.log 后退出，
// 供 helpertest 断言辅助脚本确实以 "/S /D=<目录>" 调起了安装器。
package main

import (
	"os"
	"strings"
	"time"
)

func main() {
	self, err := os.Executable()
	if err != nil {
		os.Exit(2)
	}
	// 模拟真实安装器：落盘参数 + 短暂耗时
	args := strings.Join(os.Args[1:], " ")
	if err := os.WriteFile(self+".install.log", []byte(args+"\n"), 0o644); err != nil {
		os.Exit(3)
	}
	time.Sleep(300 * time.Millisecond)
}
