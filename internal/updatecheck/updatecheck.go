// Package updatecheck 更新检查的平台资产匹配逻辑（纯函数，便于测试）。
// 资产命名约定见 .github/workflows/release.yml。
package updatecheck

import "strings"

// MatchAsset 按平台匹配 Release 资产名
func MatchAsset(goos, name string) bool {
	switch goos {
	case "windows":
		return strings.HasSuffix(name, "-installer.exe")
	case "darwin":
		return strings.Contains(name, "darwin") && strings.HasSuffix(name, ".zip")
	case "linux":
		return strings.Contains(name, "linux") && strings.HasSuffix(name, ".tar.gz")
	}
	return false
}

// PickManifestURL 从更新清单选择当前平台的下载地址；
// 顶层 fallback 仅作为 Windows 老清单（无 platforms 字段）的向后兼容回退
func PickManifestURL(goos, goarch, fallback string, platforms map[string]string) string {
	for _, k := range []string{goos + "/" + goarch, goos + "/universal"} {
		if u := platforms[k]; strings.HasPrefix(u, "http") {
			return u
		}
	}
	if goos == "windows" && strings.HasPrefix(fallback, "http") {
		return fallback
	}
	return ""
}
