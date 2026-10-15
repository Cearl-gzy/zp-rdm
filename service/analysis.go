package service

import (
	"sort"
	"strings"

	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
)

const analyzeMaxScan = 200000 // 分析扫描上限，防止超大库长时间阻塞

// AnalyzeKeys 键空间分析：大 key 排行 / TTL 分布 / 前缀内存聚合
func (s *KeyService) AnalyzeKeys(connID string, db int, delimiter string) (*model.KeyAnalysis, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	if delimiter == "" {
		delimiter = ":"
	}
	ctx := s.ctx

	result := &model.KeyAnalysis{
		TopKeys:    []model.TopKey{},
		TTLBuckets: []model.TTLBucket{},
		Prefixes:   []model.PrefixStat{},
	}
	bucketLabels := []string{"永久", "< 1 小时", "< 24 小时", "< 7 天", "< 30 天", "≥ 30 天"}
	bucketCounts := make([]int64, len(bucketLabels))
	prefixMap := map[string]*model.PrefixStat{}
	topKeys := make([]model.TopKey, 0, 64)

	var cursor uint64
	for {
		var batch []string
		batch, cursor, err = c.Scan(ctx, cursor, "*", 1000).Result()
		if err != nil {
			return nil, err
		}
		if len(batch) > 0 {
			pipe := c.Pipeline()
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
		if cursor == 0 || result.Scanned >= analyzeMaxScan {
			if result.Scanned >= analyzeMaxScan && cursor != 0 {
				result.Truncated = true
			}
			break
		}
	}

	result.TotalKeys, _ = c.DBSize(ctx).Result()
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
