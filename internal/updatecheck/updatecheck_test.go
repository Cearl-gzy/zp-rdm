package updatecheck

import "testing"

func TestMatchAsset(t *testing.T) {
	cases := []struct {
		goos, name string
		want       bool
	}{
		{"windows", "zp-rdm-amd64-installer.exe", true},
		{"windows", "zp-rdm.exe", false}, // 便携版不作为更新下载目标
		{"windows", "zp-rdm-darwin-universal.zip", false},
		{"darwin", "zp-rdm-darwin-universal.zip", true},
		{"darwin", "zp-rdm-darwin-universal.zip.sha256", false},
		{"darwin", "zp-rdm-amd64-installer.exe", false},
		{"linux", "zp-rdm-linux-amd64.tar.gz", true},
		{"linux", "zp-rdm-linux-amd64.tar.gz.sha256", false},
		{"linux", "zp-rdm-amd64-installer.exe", false},
		{"freebsd", "zp-rdm-linux-amd64.tar.gz", false}, // 未支持平台不匹配
	}
	for _, c := range cases {
		if got := MatchAsset(c.goos, c.name); got != c.want {
			t.Errorf("MatchAsset(%q, %q) = %v, want %v", c.goos, c.name, got, c.want)
		}
	}
}

func TestPickManifestURL(t *testing.T) {
	platforms := map[string]string{
		"windows/amd64":    "https://x/win.exe",
		"darwin/universal": "https://x/mac.zip",
		"linux/amd64":      "https://x/linux.tar.gz",
	}

	// 精确匹配 GOOS/GOARCH
	if got := PickManifestURL("linux", "amd64", "", platforms); got != "https://x/linux.tar.gz" {
		t.Fatalf("linux/amd64 = %q", got)
	}
	// darwin/arm64 命中 universal 回退
	if got := PickManifestURL("darwin", "arm64", "", platforms); got != "https://x/mac.zip" {
		t.Fatalf("darwin/arm64 universal fallback = %q", got)
	}
	// windows 老清单：无 platforms 时回退顶层 url
	if got := PickManifestURL("windows", "amd64", "https://x/old.exe", nil); got != "https://x/old.exe" {
		t.Fatalf("windows legacy fallback = %q", got)
	}
	// 非 windows 不回退顶层 url（那是 windows 安装包）
	if got := PickManifestURL("darwin", "arm64", "https://x/old.exe", nil); got != "" {
		t.Fatalf("darwin should not use legacy url, got %q", got)
	}
	// 平台缺失 → 空
	if got := PickManifestURL("linux", "arm64", "", platforms); got != "" {
		t.Fatalf("linux/arm64 missing = %q", got)
	}
	// 非法 url 不采用
	if got := PickManifestURL("windows", "amd64", "not-a-url", map[string]string{"windows/amd64": "ftp://x"}); got != "" {
		t.Fatalf("invalid urls = %q", got)
	}
}
