package redisx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditReadAndDates(t *testing.T) {
	dir := t.TempDir()
	old := AuditDir
	AuditDir = dir
	defer func() { AuditDir = old }()

	// 空目录：无日期
	if dates := AuditDates(); len(dates) != 0 {
		t.Fatalf("empty dates = %v", dates)
	}

	// 造两天日志（乱序文件名验证排序）
	write := func(date string, lines []string) {
		content := strings.Join(lines, "\n") + "\n"
		if err := os.WriteFile(filepath.Join(dir, "audit-"+date+".jsonl"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("2026-10-19", []string{
		`{"time":"2026-10-19 10:00:00.000","conn":"A","connId":"1","cmd":"SET","args":"k v"}`,
		`not-json-line`,
		`{"time":"2026-10-19 11:00:00.000","conn":"B","connId":"2","cmd":"DEL","args":"k"}`,
	})
	write("2026-10-20", []string{
		`{"time":"2026-10-20 09:00:00.000","conn":"A","connId":"1","cmd":"HSET","args":"h f v"}`,
	})

	dates := AuditDates()
	if len(dates) != 2 || dates[0] != "2026-10-20" || dates[1] != "2026-10-19" {
		t.Fatalf("dates = %v", dates)
	}

	// 倒序返回（新记录在前），坏行跳过
	entries, err := AuditRead("2026-10-19", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Cmd != "DEL" || entries[1].Cmd != "SET" {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[1].Conn != "A" || entries[1].Args != "k v" {
		t.Fatalf("entry = %+v", entries[1])
	}

	// maxLines 截断保留尾部（最新）
	entries2, err := AuditRead("2026-10-19", 1)
	if err != nil || len(entries2) != 1 || entries2[0].Cmd != "DEL" {
		t.Fatalf("capped = %+v err=%v", entries2, err)
	}

	// 不存在的日期返回空；非法日期拒绝（防路径穿越）
	if e, err := AuditRead("2026-01-01", 100); err != nil || e != nil {
		t.Fatalf("missing date = %+v err=%v", e, err)
	}
	if e, err := AuditRead("../etc", 100); err != nil || e != nil {
		t.Fatalf("bad date = %+v err=%v", e, err)
	}
}
