package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"zp-rdm/internal/model"
)

func seedKeys(t *testing.T, mr *miniredis.Miniredis) {
	t.Helper()
	if err := mr.Set("user:1", "alice"); err != nil {
		t.Fatal(err)
	}
	if err := mr.Set("user:2", "bob"); err != nil {
		t.Fatal(err)
	}
	mr.HSet("hash:1", "f1", "v1", "f2", "v2")
	mr.SetTTL("user:1", time.Hour)
}

func TestExportImportRoundTrip(t *testing.T) {
	src, mrSrc, srcID := newTransferService(t)
	requireDump(t, src, srcID)
	seedKeys(t, mrSrc)

	// 目标实例
	dst, mrDst, dstID := newTransferService(t)
	_ = dstID

	path := filepath.Join(t.TempDir(), "export.json")
	n, err := src.ExportKeys(srcID, 0, []string{"user:1", "user:2", "hash:1"}, "json", path)
	if err != nil {
		t.Fatalf("ExportKeys: %v", err)
	}
	if n != 3 {
		t.Fatalf("exported %d keys, want 3", n)
	}

	imported, err := dst.ImportKeys(dstID, 0, path)
	if err != nil {
		t.Fatalf("ImportKeys: %v", err)
	}
	if imported != 3 {
		t.Fatalf("imported %d, want 3", imported)
	}

	// 值一致
	if v, _ := mrDst.Get("user:1"); v != "alice" {
		t.Fatalf("user:1 = %q, want alice", v)
	}
	if v := mrDst.HGet("hash:1", "f2"); v != "v2" {
		t.Fatalf("hash:1.f2 = %q, want v2", v)
	}
	// TTL 保留（允许少量误差）
	ttl := mrDst.TTL("user:1")
	if ttl < 55*time.Minute || ttl > time.Hour {
		t.Fatalf("user:1 ttl = %v, want ~1h", ttl)
	}
}

func TestExportKeysJSONStructure(t *testing.T) {
	s, mr, connID := newTransferService(t)
	requireDump(t, s, connID)
	seedKeys(t, mr)

	path := filepath.Join(t.TempDir(), "out.json")
	if _, err := s.ExportKeys(connID, 0, []string{"user:1"}, "json", path); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	var keys []model.DumpedKey
	if err := json.Unmarshal(b, &keys); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(keys) != 1 || keys[0].Key != "user:1" || keys[0].Type != "string" {
		t.Fatalf("unexpected structure: %+v", keys)
	}
	if keys[0].TTL <= 0 {
		t.Fatalf("TTL not exported: %d", keys[0].TTL)
	}
	if keys[0].Dump == "" {
		t.Fatal("empty DUMP payload")
	}
}

func TestExportKeysCSV(t *testing.T) {
	s, mr, connID := newTransferService(t)
	requireDump(t, s, connID)
	seedKeys(t, mr)

	path := filepath.Join(t.TempDir(), "out.csv")
	n, err := s.ExportKeys(connID, 0, []string{"user:1", "hash:1"}, "csv", path)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("exported %d, want 2", n)
	}
	b, _ := os.ReadFile(path)
	content := string(b)
	if !strings.Contains(content, "key,type,ttl_seconds,value") {
		t.Fatal("missing CSV header")
	}
	if !strings.Contains(content, "user:1,string,") || !strings.Contains(content, "alice") {
		t.Fatalf("string value row wrong:\n%s", content)
	}
	if !strings.Contains(content, "hash:1,hash,") || !strings.Contains(content, "<hash>") {
		t.Fatalf("non-string type should use placeholder:\n%s", content)
	}
}

func TestExportDatabaseAndImportJSONL(t *testing.T) {
	src, mrSrc, srcID := newTransferService(t)
	requireDump(t, src, srcID)
	seedKeys(t, mrSrc)

	path := filepath.Join(t.TempDir(), "db.jsonl")
	total, err := src.ExportDatabase(srcID, 0, "*", path)
	if err != nil {
		t.Fatalf("ExportDatabase: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}

	// JSONL：每行一个对象
	b, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 3 {
		t.Fatalf("jsonl lines = %d, want 3", len(lines))
	}

	dst, mrDst, dstID := newTransferService(t)
	imported, err := dst.ImportKeys(dstID, 0, path)
	if err != nil {
		t.Fatalf("ImportKeys(jsonl): %v", err)
	}
	if imported != 3 {
		t.Fatalf("imported %d, want 3", imported)
	}
	if v, _ := mrDst.Get("user:2"); v != "bob" {
		t.Fatalf("user:2 = %q", v)
	}
}

func TestExportDatabasePattern(t *testing.T) {
	src, mrSrc, srcID := newTransferService(t)
	requireDump(t, src, srcID)
	seedKeys(t, mrSrc)

	path := filepath.Join(t.TempDir(), "users.jsonl")
	total, err := src.ExportDatabase(srcID, 0, "user:*", path)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("pattern export total = %d, want 2", total)
	}
}

func TestImportOverwrite(t *testing.T) {
	src, mrSrc, srcID := newTransferService(t)
	requireDump(t, src, srcID)
	if err := mrSrc.Set("k", "v1"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "k.json")
	if _, err := src.ExportKeys(srcID, 0, []string{"k"}, "json", path); err != nil {
		t.Fatal(err)
	}

	// 改值后导入应覆盖（RESTORE REPLACE）
	if err := mrSrc.Set("k", "v2"); err != nil {
		t.Fatal(err)
	}
	if _, err := src.ImportKeys(srcID, 0, path); err != nil {
		t.Fatalf("import overwrite: %v", err)
	}
	if v, _ := mrSrc.Get("k"); v != "v1" {
		t.Fatalf("overwrite failed: k = %q, want v1", v)
	}
}

func TestCopyKeyAcross(t *testing.T) {
	src, mrSrc, srcID := newTransferService(t)
	requireDump(t, src, srcID)
	dst, mrDst, dstID := newTransferService(t)

	if err := mrSrc.Set("src-key", "payload"); err != nil {
		t.Fatal(err)
	}
	mrSrc.SetTTL("src-key", 2*time.Hour)

	// 跨连接复制 + 改名
	if err := src.CopyKeyAcross(srcID, 0, dstID, 0, "src-key", "dst-key", false); err != nil {
		t.Fatalf("CopyKeyAcross: %v", err)
	}
	if v, _ := mrDst.Get("dst-key"); v != "payload" {
		t.Fatalf("dst-key = %q, want payload", v)
	}
	if ttl := mrDst.TTL("dst-key"); ttl < 110*time.Minute || ttl > 2*time.Hour {
		t.Fatalf("ttl = %v, want ~2h", ttl)
	}

	// 目标已存在且不允许覆盖 → 报错
	if err := dst.CopyKeyAcross(srcID, 0, dstID, 0, "src-key", "dst-key", false); err == nil {
		t.Fatal("expected BUSYKEY error without replace")
	}
	// 允许覆盖 → 成功
	if err := dst.CopyKeyAcross(srcID, 0, dstID, 0, "src-key", "dst-key", true); err != nil {
		t.Fatalf("replace copy: %v", err)
	}
}

func TestCopyKeyAcrossMissing(t *testing.T) {
	src, _, srcID := newTransferService(t)
	requireDump(t, src, srcID)
	_, _, dstID := newTransferService(t)
	if err := src.CopyKeyAcross(srcID, 0, dstID, 0, "no-such-key", "x", false); err == nil {
		t.Fatal("expected error for missing key")
	}
}
