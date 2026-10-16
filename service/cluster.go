package service

import (
	"errors"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
)

// Redis Cluster 集群信息与节点视图（仅集群连接可用）

// clusterClient 取出集群客户端；非集群连接报错
func (s *ServerService) clusterClient(connID string) (*redis.ClusterClient, error) {
	c, err := s.cli(connID, 0)
	if err != nil {
		return nil, err
	}
	cc, ok := c.(*redis.ClusterClient)
	if !ok {
		return nil, errors.New("该连接不是集群模式")
	}
	return cc, nil
}

// ClusterInfo 返回 CLUSTER INFO 的键值对（cluster_state / cluster_slots_assigned / cluster_known_nodes / cluster_size …）
func (s *ServerService) ClusterInfo(connID string) (map[string]string, error) {
	cc, err := s.clusterClient(connID)
	if err != nil {
		return nil, err
	}
	raw, err := cc.ClusterInfo(s.ctx).Result()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if k, v, ok := strings.Cut(line, ":"); ok {
			out[k] = v
		}
	}
	return out, nil
}

// ClusterNodes 解析 CLUSTER NODES 为节点列表（含槽位分布）
func (s *ServerService) ClusterNodes(connID string) ([]model.ClusterNode, error) {
	cc, err := s.clusterClient(connID)
	if err != nil {
		return nil, err
	}
	raw, err := cc.ClusterNodes(s.ctx).Result()
	if err != nil {
		return nil, err
	}
	return parseClusterNodes(raw), nil
}

// parseClusterNodes 解析 CLUSTER NODES 原始文本。
// 行格式：<id> <ip:port@cport> <flags> <master> <ping> <pong> <epoch> <link> [slots...]
func parseClusterNodes(raw string) []model.ClusterNode {
	var out []model.ClusterNode
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 8 {
			continue
		}
		flags := strings.Split(f[2], ",")
		n := model.ClusterNode{
			ID:        f[0],
			Addr:      stripClusterPort(f[1]),
			MasterID:  f[3],
			LinkState: f[7],
		}
		for _, fl := range flags {
			switch fl {
			case "myself":
				n.Myself = true
			case "master":
				n.Role = "master"
			case "slave", "replica":
				n.Role = "slave"
			}
		}
		if n.Role == "" {
			n.Role = "master"
		}
		for _, tok := range f[8:] {
			if strings.HasPrefix(tok, "[") {
				continue // 迁移中槽位 [slot-<-node]，基础视图忽略
			}
			n.Slots = append(n.Slots, tok)
			if lo, hi, ok := strings.Cut(tok, "-"); ok {
				if l, err1 := strconv.Atoi(lo); err1 == nil {
					if h, err2 := strconv.Atoi(hi); err2 == nil && h >= l {
						n.SlotCount += h - l + 1
						continue
					}
				}
			} else if _, err := strconv.Atoi(tok); err == nil {
				n.SlotCount++
			}
		}
		out = append(out, n)
	}
	return out
}

// stripClusterPort 去掉节点地址的 @cport 后缀（集群总线端口）
func stripClusterPort(addr string) string {
	if host, _, ok := strings.Cut(addr, "@"); ok {
		return host
	}
	return addr
}

// KeySlot 键的哈希槽（CLUSTER KEYSLOT）
func (s *ServerService) KeySlot(connID string, key string) (int64, error) {
	cc, err := s.clusterClient(connID)
	if err != nil {
		return -1, err
	}
	return cc.ClusterKeySlot(s.ctx, key).Result()
}
