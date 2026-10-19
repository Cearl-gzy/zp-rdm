package service

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"zp-rdm/internal/model"
	"zp-rdm/internal/redisx"
)

func TestParseReplicationInfoMaster(t *testing.T) {
	raw := "# Replication\r\n" +
		"role:master\r\n" +
		"connected_slaves:2\r\n" +
		"slave1:ip=10.0.0.12,port=6380,state=online,offset=1820,lag=0\r\n" +
		"slave0:ip=10.0.0.11,port=6379,state=online,offset=1820,lag=1\r\n" +
		"master_failover_state:no-failover\r\n" +
		"master_replid:9f2c5b1e\r\n" +
		"master_repl_offset:1820\r\n" +
		"repl_backlog_active:1\r\n" +
		"repl_backlog_size:1048576\r\n" +
		"repl_backlog_histlen:2048\r\n"
	info := parseReplicationInfo(raw)
	if info.Role != "master" {
		t.Fatalf("role = %q", info.Role)
	}
	if info.Offset != 1820 {
		t.Errorf("offset = %d", info.Offset)
	}
	if !info.BacklogActive || info.BacklogSize != 1048576 || info.BacklogHistlen != 2048 {
		t.Errorf("backlog = %+v", info)
	}
	if len(info.Replicas) != 2 {
		t.Fatalf("replicas = %+v", info.Replicas)
	}
	// 按 slaveN 下标排序（与 map 遍历顺序无关）
	if info.Replicas[0].Addr != "10.0.0.11:6379" || info.Replicas[1].Addr != "10.0.0.12:6380" {
		t.Errorf("replica order = %+v", info.Replicas)
	}
	r0 := info.Replicas[0]
	if r0.State != "online" || r0.Offset != 1820 || r0.Lag != 1 {
		t.Errorf("replica0 = %+v", r0)
	}
	// master 视角无上游字段
	if info.MasterAddr != "" || info.LinkStatus != "" {
		t.Errorf("master view should have no upstream: %+v", info)
	}
}

func TestParseReplicationInfoReplica(t *testing.T) {
	raw := "# Replication\r\n" +
		"role:slave\r\n" +
		"master_host:10.0.0.1\r\n" +
		"master_port:6379\r\n" +
		"master_link_status:up\r\n" +
		"master_last_io_seconds_ago:3\r\n" +
		"master_sync_in_progress:0\r\n" +
		"slave_repl_offset:4522\r\n" +
		"slave_priority:100\r\n" +
		"slave_read_only:1\r\n"
	info := parseReplicationInfo(raw)
	if info.Role != "slave" {
		t.Fatalf("role = %q", info.Role)
	}
	if info.MasterAddr != "10.0.0.1:6379" {
		t.Errorf("masterAddr = %q", info.MasterAddr)
	}
	if info.LinkStatus != "up" || info.LastIOSeconds != 3 || info.SyncInProgress {
		t.Errorf("link = %+v", info)
	}
	if info.Offset != 4522 {
		t.Errorf("offset = %d", info.Offset)
	}
	if len(info.Replicas) != 0 {
		t.Errorf("replica view should have no downstream: %+v", info.Replicas)
	}
}

// 真实主从集成：scripts/start-sentinel.sh 拉起 master(6395) + replica(6396)。
// 通过 ZP_RDM_TEST_MASTER_ADDR=host:port 指定 master 地址；未设置时只跑 miniredis 部分。
func TestReplicationInfoIntegration(t *testing.T) {
	m, _, connID := newTestManager(t)
	s := NewServerService(m)
	s.Startup(context.Background())
	info, err := s.ReplicationInfo(connID, 0)
	switch {
	case err != nil && strings.Contains(err.Error(), "not supported"):
		// miniredis 不支持 INFO 分段，跳过该部分
	case err != nil:
		t.Fatal(err)
	case info.Role != "master" || len(info.Replicas) != 0:
		t.Errorf("standalone replication = %+v", info)
	}

	addr := os.Getenv("ZP_RDM_TEST_MASTER_ADDR")
	if addr == "" {
		t.Skip("未设置 ZP_RDM_TEST_MASTER_ADDR，跳过真实主从集成测试")
	}
	host, portStr, _ := strings.Cut(addr, ":")
	port, _ := strconv.Atoi(portStr)
	m2 := redisx.NewManager()
	p := model.ConnectionProfile{ID: "test-master-conn", Name: "master-test", Host: host, Port: port}
	if _, err := m2.Open(context.Background(), p); err != nil {
		t.Skipf("master 不可达: %v", err)
	}
	t.Cleanup(func() { m2.Close("test-master-conn") })
	s2 := NewServerService(m2)
	s2.Startup(context.Background())
	info2, err := s2.ReplicationInfo("test-master-conn", 0)
	if err != nil {
		t.Fatal(err)
	}
	if info2.Role != "master" {
		t.Fatalf("master role = %q", info2.Role)
	}
	if len(info2.Replicas) == 0 {
		t.Fatal("master 应有至少一个副本")
	}
	found := false
	for _, r := range info2.Replicas {
		if strings.HasSuffix(r.Addr, ":6396") {
			found = true
			if r.State != "online" {
				t.Errorf("replica state = %q", r.State)
			}
		}
	}
	if !found {
		t.Errorf("未找到 6396 副本: %+v", info2.Replicas)
	}
}
