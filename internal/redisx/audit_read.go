package redisx

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// 审计日志读取（审计日志查看器）。写入侧在 guard.go（writeAuditLine）。

// AuditEntry 单行审计记录（导出给前端展示）
type AuditEntry struct {
	Time string `json:"time"`
	Conn string `json:"conn"`
	ID   string `json:"connId"`
	Cmd  string `json:"cmd"`
	Args string `json:"args"`
}

// AuditDates 列出有审计日志的日期（YYYY-MM-DD，新→旧）
func AuditDates() []string {
	dir := auditDirPath()
	if dir == "" {
		return nil
	}
	files, err := filepath.Glob(filepath.Join(dir, "audit-*.jsonl"))
	if err != nil {
		return nil
	}
	dates := make([]string, 0, len(files))
	for _, f := range files {
		name := filepath.Base(f)
		d := strings.TrimSuffix(strings.TrimPrefix(name, "audit-"), ".jsonl")
		if len(d) == 10 {
			dates = append(dates, d)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))
	return dates
}

// AuditRead 读取某天的审计日志（倒序返回，新记录在前；maxLines<=0 时默认 2000 行防卡顿）
func AuditRead(date string, maxLines int) ([]AuditEntry, error) {
	if maxLines <= 0 || maxLines > 10000 {
		maxLines = 2000
	}
	dir := auditDirPath()
	if dir == "" {
		return nil, nil
	}
	// date 只接受 YYYY-MM-DD，防止路径穿越
	if len(date) != 10 || strings.ContainsAny(date, `/\`) {
		return nil, nil
	}
	f, err := os.Open(filepath.Join(dir, "audit-"+date+".jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	// 环形缓冲保留最后 maxLines 行
	ring := make([]string, 0, maxLines+1)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		ring = append(ring, line)
		if len(ring) > maxLines {
			ring = ring[1:]
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	out := make([]AuditEntry, 0, len(ring))
	for i := len(ring) - 1; i >= 0; i-- {
		var e AuditEntry
		if json.Unmarshal([]byte(ring[i]), &e) == nil {
			out = append(out, e)
		}
	}
	return out, nil
}
