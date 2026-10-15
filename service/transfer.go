package service

import (
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
	"zp-rdm/internal/redisx"
)

// EventExportProgress 整库导出进度事件（payload: 已导出键数）
const EventExportProgress = "export:progress"

const exportBatchSize = 500

// TransferService 数据导入导出 / 跨连接复制
type TransferService struct {
	ctx     context.Context
	manager *redisx.Manager
}

func NewTransferService(m *redisx.Manager) *TransferService { return &TransferService{manager: m} }

func (s *TransferService) Startup(ctx context.Context) { s.ctx = ctx }

func (s *TransferService) cli(connID string, db int) (*redis.Client, error) {
	return s.manager.Get(connID, db)
}

// dumpKeys 批量取 DUMP + PTTL + TYPE（全类型无损序列化）
func (s *TransferService) dumpKeys(c *redis.Client, keys []string) ([]model.DumpedKey, error) {
	ctx := s.ctx
	pipe := c.Pipeline()
	dumpCmds := make([]*redis.StringCmd, len(keys))
	ttlCmds := make([]*redis.DurationCmd, len(keys))
	typeCmds := make([]*redis.StatusCmd, len(keys))
	for i, k := range keys {
		dumpCmds[i] = pipe.Dump(ctx, k)
		ttlCmds[i] = pipe.PTTL(ctx, k)
		typeCmds[i] = pipe.Type(ctx, k)
	}
	_, _ = pipe.Exec(ctx)

	out := make([]model.DumpedKey, 0, len(keys))
	for i, k := range keys {
		dv, err := dumpCmds[i].Result()
		if err != nil { // 键在扫描后过期/删除
			continue
		}
		ttl := ttlMillis(ttlCmds[i].Val())
		out = append(out, model.DumpedKey{
			Key:  k,
			Type: typeCmds[i].Val(),
			TTL:  ttl,
			Dump: base64.StdEncoding.EncodeToString([]byte(dv)),
		})
	}
	return out, nil
}

// ExportKeys 导出选中键（format: json | csv），写入 path，返回导出数量
func (s *TransferService) ExportKeys(connID string, db int, keys []string, format, path string) (int, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return 0, err
	}
	if len(keys) == 0 {
		return 0, fmt.Errorf("未选择任何键")
	}

	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	total := 0
	if format == "csv" {
		w := csv.NewWriter(f)
		defer w.Flush()
		_ = w.Write([]string{"key", "type", "ttl_seconds", "value"})
		for start := 0; start < len(keys); start += exportBatchSize {
			end := start + exportBatchSize
			if end > len(keys) {
				end = len(keys)
			}
			dumped, err := s.dumpKeys(c, keys[start:end])
			if err != nil {
				return total, err
			}
			for _, dk := range dumped {
				// CSV 面向查看：string 类型附带可读值，其余类型标注类型
				value := fmt.Sprintf("<%s>", dk.Type)
				if dk.Type == "string" {
					if v, err := c.Get(s.ctx, dk.Key).Result(); err == nil {
						value = v
					}
				}
				ttlSec := ""
				if dk.TTL >= 0 {
					ttlSec = fmt.Sprintf("%d", dk.TTL/1000)
				}
				_ = w.Write([]string{dk.Key, dk.Type, ttlSec, value})
			}
			total += len(dumped)
		}
		return total, w.Error()
	}

	// JSON（含 dump，可再导入）
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	all := make([]model.DumpedKey, 0, len(keys))
	for start := 0; start < len(keys); start += exportBatchSize {
		end := start + exportBatchSize
		if end > len(keys) {
			end = len(keys)
		}
		dumped, err := s.dumpKeys(c, keys[start:end])
		if err != nil {
			return total, err
		}
		all = append(all, dumped...)
		total += len(dumped)
	}
	return total, enc.Encode(all)
}

// ExportDatabase 整库逻辑导出（JSONL，流式写盘 + 进度事件）
func (s *TransferService) ExportDatabase(connID string, db int, pattern, path string) (int64, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return 0, err
	}
	if pattern == "" {
		pattern = "*"
	}
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var total int64
	var cursor uint64
	ctx := s.ctx
	for {
		var batch []string
		batch, cursor, err = c.Scan(ctx, cursor, pattern, 1000).Result()
		if err != nil {
			return total, err
		}
		if len(batch) > 0 {
			dumped, derr := s.dumpKeys(c, batch)
			if derr != nil {
				return total, derr
			}
			for _, dk := range dumped {
				line, _ := json.Marshal(dk)
				if _, werr := f.Write(append(line, '\n')); werr != nil {
					return total, werr
				}
			}
			total += int64(len(dumped))
			emit(s.ctx, EventExportProgress, total)
		}
		if cursor == 0 {
			break
		}
	}
	return total, nil
}

// ImportKeys 导入键数据（JSON 数组或 JSONL，RESTORE REPLACE 覆盖同名键）
func (s *TransferService) ImportKeys(connID string, db int, path string) (int64, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return 0, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	var items []model.DumpedKey
	if err := json.Unmarshal(raw, &items); err != nil {
		// 按 JSONL 逐行解析
		items = items[:0]
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var dk model.DumpedKey
			if jerr := json.Unmarshal([]byte(line), &dk); jerr != nil {
				return 0, fmt.Errorf("无法识别的文件格式（既非 JSON 数组也非 JSONL）")
			}
			items = append(items, dk)
		}
	}
	if len(items) == 0 {
		return 0, fmt.Errorf("文件中没有可导入的键")
	}

	ctx := s.ctx
	var total int64
	for start := 0; start < len(items); start += exportBatchSize {
		end := start + exportBatchSize
		if end > len(items) {
			end = len(items)
		}
		pipe := c.Pipeline()
		cmds := make([]*redis.StatusCmd, 0, end-start)
		for _, dk := range items[start:end] {
			dumpBytes, derr := base64.StdEncoding.DecodeString(dk.Dump)
			if derr != nil {
				continue
			}
			ttl := time.Duration(dk.TTL) * time.Millisecond
			if ttl < 0 {
				ttl = 0 // RESTORE 0 表示永久
			}
			cmds = append(cmds, pipe.RestoreReplace(ctx, dk.Key, ttl, string(dumpBytes)))
		}
		_, _ = pipe.Exec(ctx)
		for _, cmd := range cmds {
			if cmd.Err() == nil {
				total++
			}
		}
	}
	return total, nil
}

// CopyKeyAcross 跨连接复制键（DUMP/RESTORE，全类型无损）
func (s *TransferService) CopyKeyAcross(srcConnID string, srcDb int, dstConnID string, dstDb int, key, dstKey string, replace bool) error {
	src, err := s.cli(srcConnID, srcDb)
	if err != nil {
		return fmt.Errorf("源连接不可用: %w", err)
	}
	dst, err := s.cli(dstConnID, dstDb)
	if err != nil {
		return fmt.Errorf("目标连接不可用: %w", err)
	}
	ctx := s.ctx

	dump, err := src.Dump(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return fmt.Errorf("源键不存在或已过期")
		}
		return err
	}
	ttl, _ := src.PTTL(ctx, key).Result()
	if ttl < 0 {
		ttl = 0 // RESTORE 0 表示永久
	}
	if replace {
		return dst.RestoreReplace(ctx, dstKey, ttl, dump).Err()
	}
	if err := dst.Restore(ctx, dstKey, ttl, dump).Err(); err != nil {
		if strings.Contains(err.Error(), "BUSYKEY") {
			return fmt.Errorf("目标键已存在（可勾选覆盖选项）")
		}
		return err
	}
	return nil
}
