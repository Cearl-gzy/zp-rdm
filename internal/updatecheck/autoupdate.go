package updatecheck

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// VerifyFileSHA256 校验文件 sha256（wantHex 为空视为无校验要求，直接通过）
func VerifyFileSHA256(path, wantHex string) error {
	wantHex = strings.ToLower(strings.TrimSpace(wantHex))
	wantHex = strings.TrimPrefix(wantHex, "sha256:")
	if wantHex == "" {
		return nil
	}
	f, err := os.Open(path) //nolint:gosec 路径来自应用内部临时目录
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != wantHex {
		return fmt.Errorf("sha256 不匹配：期望 %s，实际 %s", wantHex, got)
	}
	return nil
}

// psQuote PowerShell 单引号字符串字面量（内嵌单引号双写转义）
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// BuildUpdateCommand 生成 Windows 自动更新辅助命令（PowerShell，返回程序名 + 参数）：
// 等主进程退出（上限 5 分钟，且校验 PID 未被复用）→ UAC 提权静默安装到原目录
// （/S /D= 保留自定义安装位置）→ 清理安装包 → 启动新版。
// 用户拒绝 UAC 时回退启动旧版，不留死窗口。每步写日志到 <installer>.log，失败可溯源。
// 不用 bat：tasklist|find 管道可能死锁（写端句柄泄漏导致 find 永远等不到 EOF），
// 且 start 无法为 requireAdministrator 的安装器提权（ERROR_ELEVATION_REQUIRED）。
// installer/targetDir 由调用方保证来自应用内部（非用户输入），按 PS 字面量转义。
func BuildUpdateCommand(pid int, installer, targetDir, exeName string) (string, []string) {
	exe := targetDir + `\` + exeName
	script := "$appPid = " + strconv.Itoa(pid) + "; " +
		"$installer = " + psQuote(installer) + "; " +
		"$exe = " + psQuote(exe) + "; " +
		"$log = " + psQuote(installer+".log") + "; " +
		"function Log($m) { (Get-Date -Format 'HH:mm:ss') + ' ' + $m | Out-File -FilePath $log -Append -Encoding utf8 }; " +
		"Log 'helper started'; " +
		// 等主进程退出：上限 600×500ms=5 分钟；PID 被复用成其他进程（路径不符）时视为已退出。
		// $p.Path 读不到（权限）时保守继续等，上限兜底。
		"$n = 0; " +
		"while ($n -lt 600) { " +
		"$p = Get-Process -Id $appPid -ErrorAction SilentlyContinue; " +
		"if (-not $p) { break }; " +
		"if ($p.Path -and $p.Path -ne $exe) { break }; " +
		"Start-Sleep -Milliseconds 500; $n++ " +
		"}; " +
		"Log ('wait done n=' + $n); " +
		"try { " +
		"Log 'launching installer'; " +
		// NSIS /D 必须是最后一个参数且不带引号；ArgumentList 传单字符串时 PowerShell 原样拼到命令行
		"Start-Process -FilePath $installer -ArgumentList " + psQuote("/S /D="+targetDir) + " -Verb RunAs -Wait; " +
		"Log 'installer exited'; " +
		"Remove-Item -LiteralPath $installer -Force -ErrorAction SilentlyContinue " +
		"} catch { Log ('install failed or cancelled: ' + $_.Exception.Message) }; " +
		"Log 'relaunching app'; " +
		"Start-Process -FilePath $exe -ErrorAction SilentlyContinue"
	return "powershell.exe", []string{"-NoProfile", "-WindowStyle", "Hidden", "-Command", script}
}
