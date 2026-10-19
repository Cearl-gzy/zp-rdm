package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUpdateDownloadURLs(t *testing.T) {
	// 无资产 ID：只有直链
	urls := updateDownloadURLs(UpdateInfo{URL: "https://example.com/a.exe"})
	if len(urls) != 1 || urls[0] != "https://example.com/a.exe" {
		t.Fatalf("unexpected urls: %v", urls)
	}
	// 有资产 ID：追加 API 回退地址
	urls = updateDownloadURLs(UpdateInfo{URL: "https://example.com/a.exe", assetID: 12345})
	if len(urls) != 2 {
		t.Fatalf("expected 2 urls, got %v", urls)
	}
	want := "https://api.github.com/repos/Cearl-gzy/zp-rdm/releases/assets/12345"
	if urls[1] != want {
		t.Fatalf("fallback url = %q, want %q", urls[1], want)
	}
}

func TestFetchFirstOK_Fallback(t *testing.T) {
	// 第一个地址 500，第二个 200，且第二个应带 octet-stream Accept
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer bad.Close()
	var gotAccept string
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		_, _ = w.Write([]byte("installer-bytes"))
	}))
	defer good.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := fetchFirstOK(client, []string{bad.URL, good.URL})
	if err != nil {
		t.Fatalf("fetchFirstOK: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "installer-bytes" {
		t.Fatalf("body = %q", body)
	}
	if gotAccept != "application/octet-stream" {
		t.Fatalf("fallback Accept = %q, want application/octet-stream", gotAccept)
	}
}

func TestFetchFirstOK_AllFail(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer bad.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	// 不可达地址 + 404 地址 + 空地址
	_, err := fetchFirstOK(client, []string{"http://127.0.0.1:1/none", bad.URL, ""})
	if err == nil || !strings.Contains(err.Error(), "下载失败") {
		t.Fatalf("err = %v, want 下载失败", err)
	}
}

func TestFetchFirstOK_FirstWorks(t *testing.T) {
	var acceptFirst string
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acceptFirst = r.Header.Get("Accept")
		_, _ = w.Write([]byte("ok"))
	}))
	defer good.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := fetchFirstOK(client, []string{good.URL, "http://127.0.0.1:1/unused"})
	if err != nil {
		t.Fatalf("fetchFirstOK: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if acceptFirst == "application/octet-stream" {
		t.Fatalf("首个地址不应带 octet-stream Accept，got %q", acceptFirst)
	}
}
