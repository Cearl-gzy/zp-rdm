package main

import (
	"testing"

	"zp-rdm/internal/updatecheck"
)

func TestUpdateAPIFallbackURL(t *testing.T) {
	// 无资产 ID：无回退地址
	if got := updateAPIFallbackURL(UpdateInfo{URL: "https://example.com/a.exe"}); got != "" {
		t.Fatalf("无资产 ID 应为空, got %q", got)
	}
	// 有资产 ID：API octet-stream 回退地址
	got := updateAPIFallbackURL(UpdateInfo{assetID: 12345})
	want := "https://api.github.com/repos/Cearl-gzy/zp-rdm/releases/assets/12345"
	if got != want {
		t.Fatalf("fallback url = %q, want %q", got, want)
	}
}

func TestDownloadCandidates(t *testing.T) {
	// github.com 直链：官方 + 镜像 + API 回退全量候选，官方直链在前
	info := UpdateInfo{
		URL:     "https://github.com/Cearl-gzy/zp-rdm/releases/download/v1/x-installer.exe",
		assetID: 7,
	}
	urls := updatecheck.RaceCandidates(info.URL, updateAPIFallbackURL(info))
	want := 1 + len(updatecheck.MirrorPrefixes) + 1
	if len(urls) != want {
		t.Fatalf("候选数 = %d, want %d: %v", len(urls), want, urls)
	}
	if urls[0] != info.URL {
		t.Fatalf("首个候选应为官方直链, got %s", urls[0])
	}
	if last := urls[len(urls)-1]; last != "https://api.github.com/repos/Cearl-gzy/zp-rdm/releases/assets/7" {
		t.Fatalf("末位应为 API 回退, got %s", last)
	}
}
