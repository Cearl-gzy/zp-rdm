package service

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"zp-rdm/internal/model"
)

// 主从复制拓扑（INFO replication）。集群模式请用集群视图（本方法只反映单个被路由节点，
// 语义有误导性，故拒绝）；哨兵模式作用在 master 上，可用。

// errClusterReplication 集群模式的复制拓扑应走集群视图
var errClusterReplication = errors.New("集群模式请使用集群视图（各节点复制状态可用 CLI 逐节点执行 INFO replication 查看）")

// ReplicationInfo 主从复制拓扑：角色、上游链路状态（副本视角）、下游副本列表与延迟（master 视角）
func (s *ServerService) ReplicationInfo(connID string, db int) (*model.ReplicationInfo, error) {
	if s.manager.IsCluster(connID) {
		return nil, errClusterReplication
	}
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	raw, err := c.Do(s.ctx, "INFO", "replication").Text()
	if err != nil {
		return nil, err
	}
	return parseReplicationInfo(raw), nil
}

// parseReplicationInfo 解析 INFO replication 段。
// master 视角：connected_slaves + slaveN:ip=..,port=..,state=..,offset=..,lag=..
// 副本视角：master_host/master_port/master_link_status/master_last_io_seconds_ago/master_sync_in_progress
func parseReplicationInfo(raw string) *model.ReplicationInfo {
	kv := make(map[string]string)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if k, v, ok := strings.Cut(line, ":"); ok {
			kv[k] = v
		}
	}
	atoi := func(key string) int64 {
		n, _ := strconv.ParseInt(kv[key], 10, 64)
		return n
	}
	info := &model.ReplicationInfo{
		Role:           kv["role"],
		Replicas:       []model.ReplicaInfo{},
		BacklogActive:  kv["repl_backlog_active"] == "1",
		BacklogSize:    atoi("repl_backlog_size"),
		BacklogHistlen: atoi("repl_backlog_histlen"),
	}
	if info.Role == "master" {
		info.Offset = atoi("master_repl_offset")
		type indexedReplica struct {
			idx int
			r   model.ReplicaInfo
		}
		var indexed []indexedReplica
		for k, v := range kv {
			idxStr, ok := strings.CutPrefix(k, "slave")
			if !ok {
				continue
			}
			idx, err := strconv.Atoi(idxStr)
			if err != nil { // 非 slaveN 键（如 connected_slaves 已由前缀过滤，双保险）
				continue
			}
			// slaveN:ip=1.2.3.4,port=6379,state=online,offset=123,lag=1
			fields := map[string]string{}
			for _, pair := range strings.Split(v, ",") {
				if fk, fv, ok := strings.Cut(pair, "="); ok {
					fields[fk] = fv
				}
			}
			off, _ := strconv.ParseInt(fields["offset"], 10, 64)
			lag, _ := strconv.ParseInt(fields["lag"], 10, 64)
			addr := fields["ip"]
			if p := fields["port"]; p != "" {
				addr += ":" + p
			}
			indexed = append(indexed, indexedReplica{idx, model.ReplicaInfo{
				Addr: addr, State: fields["state"], Offset: off, Lag: lag,
			}})
		}
		sort.Slice(indexed, func(a, b int) bool { return indexed[a].idx < indexed[b].idx })
		for _, ir := range indexed {
			info.Replicas = append(info.Replicas, ir.r)
		}
		return info
	}
	// 副本视角
	info.Offset = atoi("slave_repl_offset")
	host, port := kv["master_host"], kv["master_port"]
	info.MasterAddr = host
	if port != "" {
		info.MasterAddr = host + ":" + port
	}
	info.LinkStatus = kv["master_link_status"]
	info.LastIOSeconds = atoi("master_last_io_seconds_ago")
	info.SyncInProgress = kv["master_sync_in_progress"] == "1"
	return info
}
