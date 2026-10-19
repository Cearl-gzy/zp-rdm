package updatecheck

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
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

// BuildUpdateBat 生成 Windows 自动更新辅助批处理：
// 等主进程退出 → 静默安装到原目录（/D 保留用户自定义安装位置）→ 启动新版 → 自删。
// installer/targetDir 由调用方保证来自应用内部（非用户输入），bat 中不转义。
func BuildUpdateBat(installer, targetDir, exeName string) string {
	// NSIS /D 必须是最后一个参数且不带引号
	return "@echo off\r\n" +
		"set PID=%1\r\n" +
		":wait\r\n" +
		"tasklist /FI \"PID eq %PID%\" | find \"%PID%\" >nul 2>&1\r\n" +
		"if not errorlevel 1 (\r\n" +
		"  timeout /t 1 /nobreak >nul\r\n" +
		"  goto wait\r\n" +
		")\r\n" +
		"start \"\" /wait \"" + installer + "\" /S /D=" + targetDir + "\r\n" +
		"start \"\" \"" + targetDir + "\\" + exeName + "\"\r\n" +
		"del /q \"%~f0\"\r\n"
}
