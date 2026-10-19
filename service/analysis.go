package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
)

const analyzeMaxScan = 200000 // 分析扫描上限，防止超大库长时间阻塞

// analysisClients 返回键分析用的客户端列表：集群模式为全部 master 节点的直连客户端
// （SCAN 只扫本节点 keyspace，逐节点聚合 = 全集群）；其他模式为连接客户端本身。
// cleanup 关闭直连节点客户端（非集群为空操作）。
func analysisClients(ctx context.Context, c redis.UniversalClient) ([]redis.UniversalClient, func(), error) {
	cc, ok := c.(*redis.ClusterClient)
	if !ok {
		return []redis.UniversalClient{c}, func() {}, nil
	}
	raw, err := cc.Do(ctx, "CLUSTER", "NODES").Text()
	if err != nil {
		return nil, nil, fmt.Errorf("读取集群节点失败: %w", err)
	}
	opt := cc.Options()
	var out []redis.UniversalClient
	for _, n := range parseClusterNodes(raw) {
		if n.Role != "master" {
			continue
		}
		// 直连节点客户端：复用集群客户端的认证/TLS/SSH 拨号配置
		out = append(out, redis.NewClient(&redis.Options{
			Addr:         n.Addr,
			Username:     opt.Username,
			Password:     opt.Password,
			DialTimeout:  opt.DialTimeout,
			ReadTimeout:  opt.ReadTimeout,
			WriteTimeout: opt.WriteTimeout,
			TLSConfig:    opt.TLSConfig,
			Dialer:       opt.Dialer,
		}))
	}
	if len(out) == 0 {
		return nil, nil, fmt.Errorf("集群未发现 master 节点")
	}
	return out, func() {
		for _, nc := range out {
			_ = nc.(*redis.Client).Close()
		}
	}, nil
}

// AnalyzeKeys 键空间分析：大 key 排行 / TTL 分布 / 前缀内存聚合（集群模式聚合全部 master 节点）
func (s *KeyService) AnalyzeKeys(connID string, db int, delimiter string) (*model.KeyAnalysis, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	if delimiter == "" {
		delimiter = ":"
	}
	ctx := s.ctx
	clients, cleanup, err := analysisClients(ctx, c)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	result := &model.KeyAnalysis{
		TopKeys:    []model.TopKey{},
		TTLBuckets: []model.TTLBucket{},
		Prefixes:   []model.PrefixStat{},
	}
	bucketLabels := []string{"永久", "< 1 小时", "< 24 小时", "< 7 天", "< 30 天", "≥ 30 天"}
	bucketCounts := make([]int64, len(bucketLabels))
	prefixMap := map[string]*model.PrefixStat{}
	topKeys := make([]model.TopKey, 0, 64)

	for _, nc := range clients {
		var cursor uint64
		for {
			var batch []string
			batch, cursor, err = nc.Scan(ctx, cursor, "*", 1000).Result()
			if err != nil {
				return nil, err
			}
			if len(batch) > 0 {
				pipe := nc.Pipeline()
				typeCmds := make([]*redis.StatusCmd, len(batch))
				ttlCmds := make([]*redis.DurationCmd, len(batch))
				memCmds := make([]*redis.Cmd, len(batch))
				for i, k := range batch {
					typeCmds[i] = pipe.Type(ctx, k)
					ttlCmds[i] = pipe.TTL(ctx, k)
					memCmds[i] = pipe.Do(ctx, "MEMORY", "USAGE", k)
				}
				_, _ = pipe.Exec(ctx)

				for i, k := range batch {
					t := typeCmds[i].Val()
					if t == "" || t == "none" {
						continue
					}
					ttl := ttlSeconds(ttlCmds[i].Val())
					size := int64(0)
					if v, merr := memCmds[i].Int64(); merr == nil {
						size = v
					}
					result.Scanned++
					result.TotalMemory += size

					// TTL 分布
					switch {
					case ttl < 0:
						bucketCounts[0]++
					case ttl < 3600:
						bucketCounts[1]++
					case ttl < 86400:
						bucketCounts[2]++
					case ttl < 7*86400:
						bucketCounts[3]++
					case ttl < 30*86400:
						bucketCounts[4]++
					default:
						bucketCounts[5]++
					}

					// 前缀聚合
					prefix := "(无前缀)"
					if idx := strings.Index(k, delimiter); idx > 0 {
						prefix = k[:idx+len(delimiter)]
					}
					ps, ok := prefixMap[prefix]
					if !ok {
						ps = &model.PrefixStat{Prefix: prefix}
						prefixMap[prefix] = ps
					}
					ps.Count++
					ps.Size += size

					// 大 key 候选（仅保留大于第 20 名的，控制排序规模）
					if len(topKeys) < 20 || size > topKeys[len(topKeys)-1].Size {
						topKeys = append(topKeys, model.TopKey{Key: k, Type: t, Size: size, TTL: ttl})
						sort.Slice(topKeys, func(a, b int) bool { return topKeys[a].Size > topKeys[b].Size })
						if len(topKeys) > 20 {
							topKeys = topKeys[:20]
						}
					}
				}
			}
			if cursor == 0 {
				break
			}
			if result.Scanned >= analyzeMaxScan { // 达上限且 keyspace 未扫完 → 截断
				result.Truncated = true
				break
			}
		}
		if result.Truncated {
			break
		}
	}

	if cc, ok := c.(*redis.ClusterClient); ok {
		result.TotalKeys = clusterDBSize(ctx, cc) // 全集群 DBSIZE 汇总
	} else {
		result.TotalKeys, _ = c.DBSize(ctx).Result()
	}
	for i, label := range bucketLabels {
		result.TTLBuckets = append(result.TTLBuckets, model.TTLBucket{Label: label, Count: bucketCounts[i]})
	}
	for _, ps := range prefixMap {
		result.Prefixes = append(result.Prefixes, *ps)
	}
	sort.Slice(result.Prefixes, func(a, b int) bool { return result.Prefixes[a].Size > result.Prefixes[b].Size })
	if len(result.Prefixes) > 15 {
		// 超出部分合并为"其他"
		rest := model.PrefixStat{Prefix: "(其他)"}
		for _, ps := range result.Prefixes[15:] {
			rest.Count += ps.Count
			rest.Size += ps.Size
		}
		result.Prefixes = append(result.Prefixes[:15], rest)
	}
	result.TopKeys = topKeys
	return result, nil
}

// bigKeyLenCmd 各类型取元素数的命令（string 为字节长度）
var bigKeyLenCmd = map[string]string{
	"string": "STRLEN",
	"list":   "LLEN",
	"set":    "SCARD",
	"zset":   "ZCARD",
	"hash":   "HLEN",
	"stream": "XLEN",
}

// insertBigKey 把候选插入按 Size 降序的 top 切片（保持长度 ≤ topN）
func insertBigKey(top []model.BigKeyEntry, e model.BigKeyEntry, topN int) []model.BigKeyEntry {
	if len(top) == topN && e.Size <= top[len(top)-1].Size {
		return top
	}
	pos := len(top)
	for i, cur := range top {
		if e.Size > cur.Size {
			pos = i
			break
		}
	}
	top = append(top, model.BigKeyEntry{})
	copy(top[pos+1:], top[pos:])
	top[pos] = e
	if len(top) > topN {
		top = top[:topN]
	}
	return top
}

// BigKeyReport 大键扫描报告：按 pattern 采样扫描，逐键统计内存/元素数/TTL，
// 输出 TOP N 排行 + 类型聚合（集群模式聚合全部 master 节点，maxScan 为全局上限）。
func (s *KeyService) BigKeyReport(connID string, db int, pattern string, maxScan int64, topN int) (*model.BigKeyReport, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	if pattern == "" {
		pattern = "*"
	}
	if maxScan <= 0 {
		maxScan = 50000
	}
	if maxScan > analyzeMaxScan {
		maxScan = analyzeMaxScan
	}
	if topN < 10 {
		topN = 10
	}
	if topN > 200 {
		topN = 200
	}
	ctx := s.ctx
	clients, cleanup, err := analysisClients(ctx, c)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	start := time.Now()

	report := &model.BigKeyReport{Top: []model.BigKeyEntry{}, Types: []model.BigKeyTypeStat{}}
	typeStats := map[string]*model.BigKeyTypeStat{}

	for _, nc := range clients {
		var cursor uint64
		for {
			var batch []string
			batch, cursor, err = nc.Scan(ctx, cursor, pattern, 1000).Result()
			if err != nil {
				return nil, fmt.Errorf("SCAN 失败: %w", err)
			}
			if len(batch) > 0 {
				// 第一批管道：TYPE（元素数命令依赖类型）
				pipe := nc.Pipeline()
				typeCmds := make([]*redis.StatusCmd, len(batch))
				for i, k := range batch {
					typeCmds[i] = pipe.Type(ctx, k)
				}
				_, _ = pipe.Exec(ctx)

				// 第二批管道：MEMORY USAGE + TTL + 元素数
				pipe2 := nc.Pipeline()
				memCmds := make([]*redis.Cmd, len(batch))
				ttlCmds := make([]*redis.DurationCmd, len(batch))
				lenCmds := make([]*redis.Cmd, len(batch))
				types := make([]string, len(batch))
				for i, k := range batch {
					t := typeCmds[i].Val()
					types[i] = t
					if t == "" || t == "none" {
						continue
					}
					memCmds[i] = pipe2.Do(ctx, "MEMORY", "USAGE", k)
					ttlCmds[i] = pipe2.TTL(ctx, k)
					if cmd, ok := bigKeyLenCmd[t]; ok {
						lenCmds[i] = pipe2.Do(ctx, cmd, k)
					}
				}
				_, _ = pipe2.Exec(ctx)

				for i, k := range batch {
					t := types[i]
					if t == "" || t == "none" {
						continue
					}
					var size, length int64
					if memCmds[i] != nil {
						size, _ = memCmds[i].Int64()
					}
					if lenCmds[i] != nil {
						length, _ = lenCmds[i].Int64()
					}
					var ttl int64
					if ttlCmds[i] != nil {
						ttl = ttlSeconds(ttlCmds[i].Val())
					}
					report.Scanned++

					ts, ok := typeStats[t]
					if !ok {
						ts = &model.BigKeyTypeStat{Type: t}
						typeStats[t] = ts
					}
					ts.Count++
					ts.Size += size

					report.Top = insertBigKey(report.Top, model.BigKeyEntry{
						Key: k, Type: t, Size: size, Len: length, TTL: ttl,
					}, topN)
				}
			}
			if cursor == 0 {
				break
			}
			if report.Scanned >= maxScan { // 达上限且 keyspace 未扫完 → 截断
				report.Truncated = true
				break
			}
		}
		if report.Truncated {
			break
		}
	}

	if cc, ok := c.(*redis.ClusterClient); ok {
		report.TotalKeys = clusterDBSize(ctx, cc) // 全集群 DBSIZE 汇总
	} else {
		report.TotalKeys, _ = c.DBSize(ctx).Result()
	}
	report.ElapsedMs = time.Since(start).Milliseconds()
	for _, ts := range typeStats {
		report.Types = append(report.Types, *ts)
	}
	sort.Slice(report.Types, func(a, b int) bool { return report.Types[a].Size > report.Types[b].Size })
	return report, nil
}
