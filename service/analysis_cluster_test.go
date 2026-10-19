package service

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// 集群版键分析集成测试：聚合全部 master 节点。
// 需要 3 主节点集群（scripts/start-cluster.sh，6390-6392），
// 通过 ZP_RDM_TEST_CLUSTER_ADDR 指定；未设置时跳过。
func TestClusterAnalyzeKeys(t *testing.T) {
	m, seed, connID := newClusterFixture(t)
	ks := NewKeyService(m)
	ks.Startup(context.Background())

	// 种 90 个键，hash tag 散开确保落到不同节点；两个前缀 + 部分 TTL
	const total = 90
	for i := 0; i < total; i++ {
		prefix := "zp-ana"
		if i%2 == 0 {
			prefix = "zp-anb"
		}
		k := fmt.Sprintf("%s:{t%d}:%d", prefix, i%10, i)
		if err := seed.Set(tctx, k, "v", 0).Err(); err != nil {
			t.Fatal(err)
		}
		if i%3 == 0 {
			// 30 分钟：远离 1 小时分桶边界，避免 TTL 取整落在边界上
			if err := seed.Expire(tctx, k, 30*time.Minute).Err(); err != nil {
				t.Fatal(err)
			}
		}
	}

	res, err := ks.AnalyzeKeys(connID, 0, ":")
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != total {
		t.Errorf("Scanned = %d, want %d（应聚合全部 master 节点）", res.Scanned, total)
	}
	if res.TotalKeys < total {
		t.Errorf("TotalKeys = %d, want >= %d（全集群 DBSIZE 汇总）", res.TotalKeys, total)
	}
	// 两个前缀都应出现且键数各半
	counts := map[string]int64{}
	for _, p := range res.Prefixes {
		counts[p.Prefix] = p.Count
	}
	if counts["zp-ana:"] != 45 || counts["zp-anb:"] != 45 {
		t.Errorf("prefixes = %+v", res.Prefixes)
	}
	// TTL 分布：1/3 的键在 < 1 小时 桶，其余永久
	if res.TTLBuckets[0].Count != 60 || res.TTLBuckets[1].Count != 30 {
		t.Errorf("ttl buckets = %+v", res.TTLBuckets)
	}
}

func TestClusterBigKeyReport(t *testing.T) {
	m, seed, connID := newClusterFixture(t)
	ks := NewKeyService(m)
	ks.Startup(context.Background())

	// 30 个小键 + 1 个大键（带 hash tag 分散）
	for i := 0; i < 30; i++ {
		if err := seed.Set(tctx, fmt.Sprintf("zp-bk:{t%d}:%d", i%10, i), "v", 0).Err(); err != nil {
			t.Fatal(err)
		}
	}
	bigVal := make([]byte, 64*1024)
	if err := seed.Set(tctx, "zp-bk:{big}huge", bigVal, 0).Err(); err != nil {
		t.Fatal(err)
	}

	rep, err := ks.BigKeyReport(connID, 0, "zp-bk:*", 50000, 20)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scanned != 31 {
		t.Errorf("Scanned = %d, want 31（应聚合全部 master 节点）", rep.Scanned)
	}
	if len(rep.Top) == 0 || rep.Top[0].Key != "zp-bk:{big}huge" {
		t.Fatalf("top[0] = %+v", rep.Top)
	}
	if rep.Top[0].Size < 64*1024 {
		t.Errorf("top size = %d, want >= 64K", rep.Top[0].Size)
	}
	if rep.TotalKeys < 31 {
		t.Errorf("TotalKeys = %d, want >= 31", rep.TotalKeys)
	}
	var stringStat *int64
	for _, ts := range rep.Types {
		if ts.Type == "string" {
			stringStat = &ts.Count
		}
	}
	if stringStat == nil || *stringStat != 31 {
		t.Errorf("types = %+v", rep.Types)
	}
}
