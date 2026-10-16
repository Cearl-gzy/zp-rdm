package service

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
	"zp-rdm/internal/redisx"
)

// Redis Cluster 集成测试。
// 需要 3 主节点集群（scripts/start-cluster.sh 在 WSL 拉起 6390-6392），
// 通过 ZP_RDM_TEST_CLUSTER_ADDR=host:port 指定；未设置时跳过。
func newClusterFixture(t *testing.T) (*redisx.Manager, *redis.ClusterClient, string) {
	t.Helper()
	addr := os.Getenv("ZP_RDM_TEST_CLUSTER_ADDR")
	if addr == "" {
		t.Skip("未设置 ZP_RDM_TEST_CLUSTER_ADDR，跳过集群集成测试")
	}
	host, portStr, _ := strings.Cut(addr, ":")
	port, _ := strconv.Atoi(portStr)

	seed := redis.NewClusterClient(&redis.ClusterOptions{Addrs: []string{addr}})
	t.Cleanup(func() { _ = seed.Close() })
	if err := seed.Ping(tctx).Err(); err != nil {
		t.Skipf("集群不可达: %v", err)
	}
	// 清空所有 master（ClusterClient.FlushAll 只路由单节点）
	if err := seed.ForEachMaster(tctx, func(ctx context.Context, c *redis.Client) error {
		return c.FlushAll(ctx).Err()
	}); err != nil {
		t.Fatalf("flush cluster: %v", err)
	}

	m := redisx.NewManager()
	connID := "test-cluster-conn"
	p := model.ConnectionProfile{ID: connID, Name: "cluster-test", Host: host, Port: port, Cluster: true}
	if _, err := m.Open(context.Background(), p); err != nil {
		t.Fatalf("open cluster connection: %v", err)
	}
	t.Cleanup(func() { m.Close(connID) })
	return m, seed, connID
}

func TestClusterInfoAndNodes(t *testing.T) {
	m, seed, connID := newClusterFixture(t)
	s := NewServerService(m)
	s.Startup(context.Background())

	info, err := s.ClusterInfo(connID)
	if err != nil {
		t.Fatal(err)
	}
	if info["cluster_state"] != "ok" {
		t.Errorf("cluster_state = %q", info["cluster_state"])
	}
	if info["cluster_slots_assigned"] != "16384" {
		t.Errorf("slots_assigned = %q", info["cluster_slots_assigned"])
	}
	if info["cluster_known_nodes"] != "3" {
		t.Errorf("known_nodes = %q", info["cluster_known_nodes"])
	}

	nodes, err := s.ClusterNodes(connID)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("nodes = %+v", nodes)
	}
	slotSum, myself := 0, 0
	for _, n := range nodes {
		if n.Role != "master" {
			t.Errorf("node %s role = %q, want master", n.Addr, n.Role)
		}
		if n.LinkState != "connected" {
			t.Errorf("node %s link = %q", n.Addr, n.LinkState)
		}
		if len(n.Slots) == 0 || n.SlotCount == 0 {
			t.Errorf("node %s slots = %v count=%d", n.Addr, n.Slots, n.SlotCount)
		}
		slotSum += n.SlotCount
		if n.Myself {
			myself++
		}
	}
	if slotSum != 16384 {
		t.Errorf("slot sum = %d, want 16384", slotSum)
	}
	if myself != 1 {
		t.Errorf("myself count = %d, want 1", myself)
	}

	// KeySlot 与服务端 CLUSTER KEYSLOT 一致
	want, err := seed.ClusterKeySlot(tctx, "{zp-slot}k").Result()
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.KeySlot(connID, "{zp-slot}k")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("KeySlot = %d, want %d", got, want)
	}

	// 非集群连接应报错
	m2, _, connID2 := newTestManager(t)
	s2 := NewServerService(m2)
	s2.Startup(context.Background())
	if _, err := s2.ClusterNodes(connID2); err == nil {
		t.Error("非集群连接 ClusterNodes 应报错")
	}
}

func TestClusterScanKeys(t *testing.T) {
	m, seed, connID := newClusterFixture(t)
	ks := NewKeyService(m)
	ks.Startup(context.Background())

	// 种 60 个键，hash tag 散开确保落到不同槽/节点
	const total = 60
	for i := 0; i < total; i++ {
		k := fmt.Sprintf("zp-scan:{t%d}:%d", i%10, i)
		if err := seed.Set(tctx, k, "v", 0).Err(); err != nil {
			t.Fatal(err)
		}
	}

	seen := make(map[string]bool)
	var cursor uint64
	for i := 0; i < 50; i++ {
		res, err := ks.ScanKeys(model.ScanRequest{ConnID: connID, DB: 0, Cursor: cursor, Pattern: "zp-scan:*", Count: 20})
		if err != nil {
			t.Fatal(err)
		}
		for _, ki := range res.Keys {
			seen[ki.Key] = true
		}
		cursor = res.Cursor
		if cursor == 0 {
			break
		}
	}
	if cursor != 0 {
		t.Fatal("扫描 50 轮仍未完成")
	}
	if len(seen) != total {
		t.Errorf("聚合扫描找到 %d 个键，want %d", len(seen), total)
	}

	// Total 为全集群 DBSIZE 汇总（共享集群可能有其他测试的并发键，只断言下界）
	res, err := ks.ScanKeys(model.ScanRequest{ConnID: connID, DB: 0, Cursor: 0, Count: 10})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total < total {
		t.Errorf("Total = %d, want >= %d", res.Total, total)
	}
}

func TestClusterDeleteKeys(t *testing.T) {
	m, seed, connID := newClusterFixture(t)
	ks := NewKeyService(m)
	ks.Startup(context.Background())

	// 跨槽多键删除（go-redis 集群客户端不拆 DEL，服务层逐键管道）
	keys := []string{"{zp-d}a", "{zp-d2}b", "{zp-d3}c"}
	for _, k := range keys {
		if err := seed.Set(tctx, k, "v", 0).Err(); err != nil {
			t.Fatal(err)
		}
	}
	n, err := ks.DeleteKeys(connID, 0, keys)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(keys)) {
		t.Errorf("deleted = %d, want %d", n, len(keys))
	}
	if got := seed.Exists(tctx, keys...).Val(); got != 0 {
		t.Errorf("keys still exist: %d", got)
	}
}

func TestParseClusterNodes(t *testing.T) {
	raw := `07c37dfeb235213a872192d90877d0cd55635b91 127.0.0.1:30004@31004 slave e7d1eecce10fd6bb5eb35b9f99a514335d9ba9ca 0 1426238317239 4 connected
67ed2db8d677e59ec4a4cefb06858cf2a1a89fa1 127.0.0.1:30002@31002 master - 0 1426238316232 2 connected 5461-10922
e7d1eecce10fd6bb5eb35b9f99a514335d9ba9ca 127.0.0.1:30001@31001 myself,master - 0 1426238318241 1 connected 0-5460 10000 [1234-<-292f8b365bb7edb5e285caf0b7e6ddc7265d2f4f]
`
	nodes := parseClusterNodes(raw)
	if len(nodes) != 3 {
		t.Fatalf("nodes = %+v", nodes)
	}
	// slave
	if nodes[0].Role != "slave" || nodes[0].MasterID == "" || nodes[0].MasterID == "-" {
		t.Errorf("slave node = %+v", nodes[0])
	}
	if nodes[0].Addr != "127.0.0.1:30004" {
		t.Errorf("addr @cport 未剥离: %q", nodes[0].Addr)
	}
	// master with range
	if nodes[1].Role != "master" || nodes[1].SlotCount != 5462 {
		t.Errorf("master node = %+v", nodes[1])
	}
	// myself master：范围 + 单槽；迁移槽位被忽略
	n := nodes[2]
	if !n.Myself || n.Role != "master" {
		t.Errorf("myself flags = %+v", n)
	}
	if n.SlotCount != 5461+1 {
		t.Errorf("slot count = %d, want %d", n.SlotCount, 5462)
	}
	if len(n.Slots) != 2 {
		t.Errorf("slots = %v（迁移槽应忽略）", n.Slots)
	}
}
