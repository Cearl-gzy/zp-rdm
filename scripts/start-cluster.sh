#!/usr/bin/env bash
# 启动 zp-rdm 集群集成测试用的 3 主节点 Redis Cluster（幂等）：
#   127.0.0.1:6390 / 6391 / 6392（无副本）
# 状态目录 /tmp/zp-cluster（WSL 重启后需重跑本脚本）
set -euo pipefail

BASE=/tmp/zp-cluster

for port in 6390 6391 6392; do
  if redis-cli -p "$port" ping 2>/dev/null | grep -q PONG; then
    echo "port $port already running"
    continue
  fi
  mkdir -p "$BASE/$port"
  redis-server --port "$port" --bind 127.0.0.1 \
    --cluster-enabled yes --cluster-config-file "$BASE/$port/nodes.conf" \
    --cluster-node-timeout 5000 \
    --dir "$BASE/$port" --daemonize yes --save '' --appendonly no 2>/dev/null
  sleep 0.3
  echo "port $port started"
done

# 已建群则跳过
if redis-cli -p 6390 cluster info 2>/dev/null | grep -q 'cluster_state:ok'; then
  echo "cluster already formed"
  exit 0
fi

# 旧集群状态残留时先重置
for port in 6390 6391 6392; do
  redis-cli -p "$port" flushall >/dev/null 2>&1 || true
  redis-cli -p "$port" cluster reset hard >/dev/null 2>&1 || true
done

redis-cli --cluster create 127.0.0.1:6390 127.0.0.1:6391 127.0.0.1:6392 \
  --cluster-replicas 0 --cluster-yes
sleep 1
redis-cli -p 6390 cluster info | grep cluster_state
