package service

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
)

// 热点键统计两种模式：
//  1. OBJECT FREQ —— Redis LFU 计数（仅 maxmemory-policy 为 LFU 系时可用）
//  2. MONITOR 采样 —— 独立裸连接监听命令流，客户端聚合键命中（任意实例可用，
//     集群模式只见单节点命令流，与 MONITOR 相同的已知限制）

const (
	hotKeyMaxSampleSec = 300    // MONITOR 采样上限，防止忘关
	hotKeyMaxUnique    = 200000 // 采样跟踪的不同键上限，防止内存膨胀
	hotKeyReportTopN   = 50
	freqScanDefault    = 20000
)

// ---------- MONITOR 行解析 ----------

// parseQuotedTokens 解析 MONITOR 行中 "..."] "CMD" "arg1" ... 的引号参数串
// （参数内转义 \" 与 \\）
func parseQuotedTokens(s string) ([]string, bool) {
	var out []string
	i := 0
	for i < len(s) {
		if s[i] != '"' {
			i++
			continue
		}
		i++
		var sb strings.Builder
		closed := false
		for i < len(s) {
			c := s[i]
			if c == '\\' && i+1 < len(s) {
				sb.WriteByte(s[i+1])
				i += 2
				continue
			}
			if c == '"' {
				closed = true
				i++
				break
			}
			sb.WriteByte(c)
			i++
		}
		if !closed {
			return nil, false
		}
		out = append(out, sb.String())
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// parseMonitorLine 解析一行 MONITOR 输出，返回命令名与全部参数（不含命令名）
func parseMonitorLine(line string) (cmd string, args []string, ok bool) {
	idx := strings.Index(line, "] ")
	if idx < 0 {
		return "", nil, false
	}
	tokens, ok := parseQuotedTokens(line[idx+2:])
	if !ok {
		return "", nil, false
	}
	return strings.ToUpper(tokens[0]), tokens[1:], true
}

// hkAllKeys 全部参数皆为键的命令
var hkAllKeys = map[string]bool{
	"DEL": true, "UNLINK": true, "EXISTS": true, "MGET": true, "TOUCH": true,
}

// hkPairs 键值成对（奇数位为键）的命令
var hkPairs = map[string]bool{"MSET": true, "MSETNX": true}

// hkTwoKeys 前两个参数都是键的命令
var hkTwoKeys = map[string]bool{"RENAME": true, "RENAMENX": true, "COPY": true}

// hotKeyTargets 提取一条命令涉及的键（解析失败或无键返回 nil）
func hotKeyTargets(cmd string, args []string) []string {
	if len(args) == 0 {
		return nil
	}
	switch {
	case hkAllKeys[cmd]:
		return args
	case hkPairs[cmd]:
		keys := make([]string, 0, (len(args)+1)/2)
		for i := 0; i < len(args); i += 2 {
			keys = append(keys, args[i])
		}
		return keys
	case hkTwoKeys[cmd]:
		if len(args) >= 2 {
			return args[:2]
		}
		return args[:1]
	case cmd == "EVAL" || cmd == "EVALSHA" || cmd == "EVAL_RO" || cmd == "EVALSHA_RO":
		// EVAL script numkeys key... arg...
		if len(args) >= 2 {
			var n int
			if _, err := fmt.Sscanf(args[1], "%d", &n); err == nil && n > 0 && len(args) >= 2+n {
				return args[2 : 2+n]
			}
		}
		return nil
	default:
		return args[:1]
	}
}

// hkReadCmds 只读命令（MONITOR 采样的读/写分类）；未收录命令按写计（SET 系为主流）
var hkReadCmds = map[string]bool{
	"GET": true, "GETRANGE": true, "SUBSTR": true, "STRLEN": true, "MGET": true,
	"HGET": true, "HMGET": true, "HGETALL": true, "HKEYS": true, "HVALS": true,
	"HLEN": true, "HEXISTS": true, "HSTRLEN": true, "HRANDFIELD": true,
	"LRANGE": true, "LINDEX": true, "LLEN": true, "LPOS": true,
	"SMEMBERS": true, "SISMEMBER": true, "SMISMEMBER": true, "SCARD": true,
	"SRANDMEMBER": true, "SDIFF": true, "SUNION": true, "SINTER": true, "SINTERCARD": true,
	"ZRANGE": true, "ZREVRANGE": true, "ZRANGEBYSCORE": true, "ZREVRANGEBYSCORE": true,
	"ZRANGEBYLEX": true, "ZREVRANGEBYLEX": true, "ZSCORE": true, "ZMSCORE": true,
	"ZCARD": true, "ZCOUNT": true, "ZLEXCOUNT": true, "ZRANK": true, "ZREVRANK": true, "ZRANDMEMBER": true,
	"EXISTS": true, "TTL": true, "PTTL": true, "TYPE": true, "OBJECT": true,
	"DUMP": true, "GETEX": true,
	"XRANGE": true, "XREVRANGE": true, "XLEN": true, "XREAD": true, "XINFO": true,
	"JSON.GET": true, "JSON.MGET": true, "JSON.STRLEN": true, "JSON.OBJLEN": true,
	"JSON.TYPE": true, "JSON.ARRLEN": true,
}

func hotKeyCmdClass(cmd string) bool { return hkReadCmds[cmd] }

// ---------- MONITOR 采样 ----------

type hotKeyStat struct {
	hits, reads, writes int64
}

// hotKeySampler MONITOR 采样器。停止（手动/超时/断线）后仍保留在 map 中，
// 直到 HotKeySampleStop 取走报告——超时自动停止不会丢数据。
type hotKeySampler struct {
	mu        sync.Mutex
	start     time.Time
	stopTime  time.Time
	total     int64
	keys      map[string]*hotKeyStat
	conn      net.Conn
	timer     *time.Timer
	stopped   bool
	truncated bool // 达到采样上限自动停止
}

func (sp *hotKeySampler) add(cmd string, keys []string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.total++
	read := hotKeyCmdClass(cmd)
	for _, k := range keys {
		st, ok := sp.keys[k]
		if !ok {
			if len(sp.keys) >= hotKeyMaxUnique {
				continue
			}
			st = &hotKeyStat{}
			sp.keys[k] = st
		}
		st.hits++
		if read {
			st.reads++
		} else {
			st.writes++
		}
	}
}

// finish 标记停止并关连接（幂等）；读循环随即因连接关闭退出
func (sp *hotKeySampler) finish(truncated bool) {
	sp.mu.Lock()
	if !sp.stopped {
		sp.stopped = true
		sp.truncated = truncated
		sp.stopTime = time.Now()
	}
	sp.mu.Unlock()
	if sp.timer != nil {
		sp.timer.Stop()
	}
	_ = sp.conn.Close()
}

func (sp *hotKeySampler) duration() time.Duration {
	if sp.stopped {
		return sp.stopTime.Sub(sp.start)
	}
	return time.Since(sp.start)
}

func (sp *hotKeySampler) report() *model.HotKeySampleReport {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	rep := &model.HotKeySampleReport{
		DurationSec: sp.duration().Seconds(),
		TotalCmds:   sp.total,
		UniqueKeys:  int64(len(sp.keys)),
		Truncated:   sp.truncated,
		Keys:        []model.HotKeyEntry{},
	}
	for k, st := range sp.keys {
		rep.Keys = append(rep.Keys, model.HotKeyEntry{Key: k, Hits: st.hits, Reads: st.reads, Writes: st.writes})
	}
	sort.Slice(rep.Keys, func(a, b int) bool {
		if rep.Keys[a].Hits != rep.Keys[b].Hits {
			return rep.Keys[a].Hits > rep.Keys[b].Hits
		}
		return rep.Keys[a].Key < rep.Keys[b].Key
	})
	if len(rep.Keys) > hotKeyReportTopN {
		rep.Keys = rep.Keys[:hotKeyReportTopN]
	}
	return rep
}

// HotKeySampleStart 启动 MONITOR 热点键采样（独立连接，与 MONITOR 视图互不干扰）。
// 上次采样已停止但未取报告时直接替换（旧报告丢弃）。
func (s *ServerService) HotKeySampleStart(connID string, db int) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("%s|%d", connID, db)
	s.mu.Lock()
	if cur, ok := s.hotkeys[key]; ok {
		cur.mu.Lock()
		stopped := cur.stopped
		cur.mu.Unlock()
		if !stopped {
			s.mu.Unlock()
			return fmt.Errorf("热点键采样已在运行")
		}
		delete(s.hotkeys, key)
	}
	s.mu.Unlock()

	raw, r, err := openMonitorConn(s.ctx, c, db)
	if err != nil {
		return err
	}

	sp := &hotKeySampler{start: time.Now(), keys: map[string]*hotKeyStat{}, conn: raw}
	sp.timer = time.AfterFunc(hotKeyMaxSampleSec*time.Second, func() { sp.finish(true) })

	s.mu.Lock()
	if cur, ok := s.hotkeys[key]; ok {
		// 拨号期间已有别的启动抢先 → 丢弃本次（防泄漏）
		cur.mu.Lock()
		running := !cur.stopped
		cur.mu.Unlock()
		if running {
			s.mu.Unlock()
			sp.timer.Stop()
			_ = raw.Close()
			return fmt.Errorf("热点键采样已在运行")
		}
	}
	s.hotkeys[key] = sp
	s.mu.Unlock()

	go func() {
		defer sp.finish(false) // 断线/被 Stop 关闭 → 标记停止（报告仍可取）
		for {
			line, err := readRESPLine(r)
			if err != nil {
				return
			}
			if cmd, keys, ok := parseMonitorLineKeys(line); ok {
				sp.add(cmd, keys)
			}
		}
	}()
	return nil
}

// parseMonitorLineKeys 解析一行并提取命令涉及的键
func parseMonitorLineKeys(line string) (string, []string, bool) {
	cmd, args, ok := parseMonitorLine(line)
	if !ok {
		return "", nil, false
	}
	keys := hotKeyTargets(cmd, args)
	if len(keys) == 0 {
		return "", nil, false
	}
	return cmd, keys, true
}

// HotKeySampleStats 采样进度（前端轮询）
func (s *ServerService) HotKeySampleStats(connID string, db int) model.HotKeySampleStats {
	key := fmt.Sprintf("%s|%d", connID, db)
	s.mu.Lock()
	sp, ok := s.hotkeys[key]
	s.mu.Unlock()
	if !ok {
		return model.HotKeySampleStats{Running: false}
	}
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return model.HotKeySampleStats{
		Running:    !sp.stopped,
		ElapsedSec: int64(sp.duration().Seconds()),
		TotalCmds:  sp.total,
		UniqueKeys: int64(len(sp.keys)),
	}
}

// HotKeySampleStop 停止采样并取走报告（超时/断线后同样可取，取走后状态清除）
func (s *ServerService) HotKeySampleStop(connID string, db int) (*model.HotKeySampleReport, error) {
	key := fmt.Sprintf("%s|%d", connID, db)
	s.mu.Lock()
	sp, ok := s.hotkeys[key]
	if ok {
		delete(s.hotkeys, key)
	}
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("热点键采样未在运行")
	}
	rep := sp.report()
	sp.finish(sp.truncated)
	return rep, nil
}

// ---------- OBJECT FREQ ----------

// HotKeyFreq 基于 OBJECT FREQ 的热点键扫描（仅 LFU maxmemory-policy 可用）
func (s *ServerService) HotKeyFreq(connID string, db int, maxScan int64) (*model.HotKeyFreqReport, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	if maxScan <= 0 {
		maxScan = freqScanDefault
	}
	if maxScan > analyzeMaxScan {
		maxScan = analyzeMaxScan
	}
	ctx := s.ctx

	// CONFIG GET 在 RESP2 是扁平数组、RESP3 是 map
	policy := ""
	cfg, err := c.Do(ctx, "CONFIG", "GET", "maxmemory-policy").Result()
	if err == nil {
		switch v := cfg.(type) {
		case map[interface{}]interface{}:
			policy = asString(mapGet(v, "maxmemory-policy"))
		case []interface{}:
			for i := 0; i+1 < len(v); i += 2 {
				if asString(v[i]) == "maxmemory-policy" {
					policy = asString(v[i+1])
				}
			}
		}
	}
	rep := &model.HotKeyFreqReport{Policy: policy, Keys: []model.HotKeyFreqEntry{}}
	if !strings.Contains(policy, "lfu") {
		return rep, nil // Available=false，由前端提示切换策略或用 MONITOR 采样
	}
	rep.Available = true

	var cursor uint64
	for {
		var batch []string
		batch, cursor, err = c.Scan(ctx, cursor, "*", 1000).Result()
		if err != nil {
			return nil, fmt.Errorf("SCAN 失败: %w", err)
		}
		if len(batch) > 0 {
			pipe := c.Pipeline()
			typeCmds := make([]*redis.StatusCmd, len(batch))
			freqCmds := make([]*redis.Cmd, len(batch))
			for i, k := range batch {
				typeCmds[i] = pipe.Type(ctx, k)
				freqCmds[i] = pipe.Do(ctx, "OBJECT", "FREQ", k)
			}
			_, _ = pipe.Exec(ctx)
			for i, k := range batch {
				t := typeCmds[i].Val()
				if t == "" || t == "none" {
					continue
				}
				var freq int64
				if freqCmds[i] != nil {
					freq, _ = freqCmds[i].Int64() // LFU 未命中过返回 nil → 0
				}
				rep.Scanned++
				rep.Keys = insertFreqKey(rep.Keys, model.HotKeyFreqEntry{Key: k, Type: t, Freq: freq}, hotKeyReportTopN)
			}
		}
		if cursor == 0 || rep.Scanned >= maxScan {
			break
		}
	}
	return rep, nil
}

// insertFreqKey 按 Freq 降序维护 top-N 切片
func insertFreqKey(top []model.HotKeyFreqEntry, e model.HotKeyFreqEntry, topN int) []model.HotKeyFreqEntry {
	if len(top) == topN && e.Freq <= top[len(top)-1].Freq {
		return top
	}
	pos := len(top)
	for i, cur := range top {
		if e.Freq > cur.Freq {
			pos = i
			break
		}
	}
	top = append(top, model.HotKeyFreqEntry{})
	copy(top[pos+1:], top[pos:])
	top[pos] = e
	if len(top) > topN {
		top = top[:topN]
	}
	return top
}
