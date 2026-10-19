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

// CompareSemver 比较两个 "x.y.z" 版本号：a<b 返回 -1，相等 0，a>b 返回 1。
// 段数不足按 0 补齐，非数字段按 0 处理（预发布标识不参与，本项目不发预发布版）。
func CompareSemver(a, b string) int {
	pa, pb := semverParts(a), semverParts(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			if pa[i] < pb[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func semverParts(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	for i, seg := range strings.SplitN(v, ".", 4) {
		if i >= 3 {
			break
		}
		var n int
		for _, c := range seg {
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
		}
		out[i] = n
	}
	return out
}
