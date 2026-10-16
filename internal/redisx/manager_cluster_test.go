package redisx

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"zp-rdm/internal/model"
)

// Redis Cluster 集成测试。
// 需要一个 3 主节点集群（scripts/start-cluster.sh 在 WSL 拉起 6390-6392），
// 未设置 ZP_RDM_TEST_CLUSTER_ADDR 时跳过。
func clusterTestProfile(t *testing.T) model.ConnectionProfile {
	t.Helper()
	addr := os.Getenv("ZP_RDM_TEST_CLUSTER_ADDR")
	if addr == "" {
		t.Skip("未设置 ZP_RDM_TEST_CLUSTER_ADDR，跳过集群集成测试")
	}
	host, portStr, _ := strings.Cut(addr, ":")
	port, _ := strconv.Atoi(portStr)
	return model.ConnectionProfile{
		ID: "test-cluster-conn", Name: "cluster-test",
		Host: host, Port: port, Cluster: true,
	}
}

func TestClusterOpenAndRoute(t *testing.T) {
	p := clusterTestProfile(t)
	m := NewManager()
	ctx := context.Background()

	st, err := m.Open(ctx, p)
	if err != nil {
		t.Fatalf("open cluster: %v", err)
	}
	t.Cleanup(func() { m.Close(p.ID) })

	if st.Mode != "cluster" {
		t.Errorf("mode = %q, want cluster", st.Mode)
	}
	if st.DBCount != 1 {
		t.Errorf("dbCount = %d, want 1（集群仅 db 0）", st.DBCount)
	}
	if !m.IsCluster(p.ID) {
		t.Error("IsCluster 应为 true")
	}

	// db 参数被忽略：db 7 与 db 0 拿到同一个集群 client
	c0, err := m.Get(p.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	c7, err := m.Get(p.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if c0 != c7 {
		t.Error("集群连接 Get(db 7) 应返回与 db 0 相同的 client")
	}

	// 跨槽读写（hash tag 强制不同槽）
	for _, k := range []string{"{zp-a}1", "{zp-b}1", "{zp-c}1"} {
		if err := c0.Set(ctx, k, "v-"+k, 0).Err(); err != nil {
			t.Fatalf("set %s: %v", k, err)
		}
	}
	for _, k := range []string{"{zp-a}1", "{zp-b}1", "{zp-c}1"} {
		if v, err := c0.Get(ctx, k).Result(); err != nil || v != "v-"+k {
			t.Errorf("get %s = %q, %v", k, v, err)
		}
	}
	// go-redis 集群客户端不拆多键 DEL（跨槽 CROSSSLOT），逐键删除；
	// 服务层 DeleteKeys 另有管道拆分逻辑（见 key.go）
	for _, k := range []string{"{zp-a}1", "{zp-b}1", "{zp-c}1"} {
		if err := c0.Del(ctx, k).Err(); err != nil {
			t.Errorf("del %s: %v", k, err)
		}
	}
}
