package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
)

// seedKeys 通过种子客户端写入测试数据（miniredis 与真实 Redis 一致）
func seedKeys(t *testing.T, c *redis.Client) {
	t.Helper()
	mustSet(t, c, "user:1", "alice")
	mustSet(t, c, "user:2", "bob")
	if err := c.HSet(tctx, "hash:1", "f1", "v1", "f2", "v2").Err(); err != nil {
		t.Fatal(err)
	}
	mustExpire(t, c, "user:1", time.Hour)
}

func TestExportImportRoundTrip(t *testing.T) {
	src, cSrc, srcID := newTransferService(t)
	requireDump(t, src, srcID)
	// 目标实例（真实 Redis 为共享实例，fixture 创建时会清库，须在种数前建好）
	dst, cDst, dstID := newTransferService(t)
	_ = dstID
	seedKeys(t, cSrc)

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
	if v := mustGet(t, cDst, "user:1"); v != "alice" {
		t.Fatalf("user:1 = %q, want alice", v)
	}
	if v, err := cDst.HGet(tctx, "hash:1", "f2").Result(); err != nil || v != "v2" {
		t.Fatalf("hash:1.f2 = %q, want v2 (err=%v)", v, err)
	}
	// TTL 保留（允许少量误差）
	if ttl := mustTTL(t, cDst, "user:1"); ttl < 55*time.Minute || ttl > time.Hour {
		t.Fatalf("user:1 ttl = %v, want ~1h", ttl)
	}
}

func TestExportKeysJSONStructure(t *testing.T) {
	s, c, connID := newTransferService(t)
	requireDump(t, s, connID)
	seedKeys(t, c)

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
	s, c, connID := newTransferService(t)
	requireDump(t, s, connID)
	seedKeys(t, c)

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
	src, cSrc, srcID := newTransferService(t)
	requireDump(t, src, srcID)
	// 目标实例（真实 Redis 为共享实例，fixture 创建时会清库，须在种数前建好）
	dst, cDst, dstID := newTransferService(t)
	seedKeys(t, cSrc)

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

	imported, err := dst.ImportKeys(dstID, 0, path)
	if err != nil {
		t.Fatalf("ImportKeys(jsonl): %v", err)
	}
	if imported != 3 {
		t.Fatalf("imported %d, want 3", imported)
	}
	if v := mustGet(t, cDst, "user:2"); v != "bob" {
		t.Fatalf("user:2 = %q", v)
	}
}

func TestExportDatabasePattern(t *testing.T) {
	src, cSrc, srcID := newTransferService(t)
	requireDump(t, src, srcID)
	seedKeys(t, cSrc)

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
	src, c, srcID := newTransferService(t)
	requireDump(t, src, srcID)
	mustSet(t, c, "k", "v1")
	path := filepath.Join(t.TempDir(), "k.json")
	if _, err := src.ExportKeys(srcID, 0, []string{"k"}, "json", path); err != nil {
		t.Fatal(err)
	}

	// 改值后导入应覆盖（RESTORE REPLACE）
	mustSet(t, c, "k", "v2")
	if _, err := src.ImportKeys(srcID, 0, path); err != nil {
		t.Fatalf("import overwrite: %v", err)
	}
	if v := mustGet(t, c, "k"); v != "v1" {
		t.Fatalf("overwrite failed: k = %q, want v1", v)
	}
}

func TestCopyKeyAcross(t *testing.T) {
	src, cSrc, srcID := newTransferService(t)
	requireDump(t, src, srcID)
	dst, cDst, dstID := newTransferService(t)

	mustSet(t, cSrc, "src-key", "payload")
	mustExpire(t, cSrc, "src-key", 2*time.Hour)

	// 跨连接复制 + 改名
	if err := src.CopyKeyAcross(srcID, 0, dstID, 0, "src-key", "dst-key", false); err != nil {
		t.Fatalf("CopyKeyAcross: %v", err)
	}
	if v := mustGet(t, cDst, "dst-key"); v != "payload" {
		t.Fatalf("dst-key = %q, want payload", v)
	}
	if ttl := mustTTL(t, cDst, "dst-key"); ttl < 110*time.Minute || ttl > 2*time.Hour {
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
