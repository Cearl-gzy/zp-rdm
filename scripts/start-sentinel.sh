#!/usr/bin/env bash
# 启动 zp-rdm 哨兵集成测试环境（幂等）：
#   master   127.0.0.1:6395
#   replica  127.0.0.1:6396（replicaof 6395）
#   sentinel 127.0.0.1:26379（监控 master 名 mymaster，quorum 1）
# 状态目录 /tmp/zp-sentinel（WSL 重启后需重跑本脚本）
set -euo pipefail

BASE=/tmp/zp-sentinel
mkdir -p "$BASE"

# master
if redis-cli -p 6395 ping 2>/dev/null | grep -q PONG; then
  echo "master 6395 already running"
else
  redis-server --port 6395 --bind 127.0.0.1 \
    --dir "$BASE" --daemonize yes --save '' --appendonly no 2>/dev/null
  sleep 0.3
  echo "master 6395 started"
fi

# replica
if redis-cli -p 6396 ping 2>/dev/null | grep -q PONG; then
  echo "replica 6396 already running"
else
  redis-server --port 6396 --bind 127.0.0.1 --replicaof 127.0.0.1 6395 \
    --dir "$BASE" --daemonize yes --save '' --appendonly no 2>/dev/null
  sleep 0.3
  echo "replica 6396 started"
fi

# sentinel
if redis-cli -p 26379 ping 2>/dev/null | grep -q PONG; then
  echo "sentinel 26379 already running"
else
  cat > "$BASE/sentinel.conf" <<EOF
port 26379
bind 127.0.0.1
dir $BASE
daemonize yes
sentinel monitor mymaster 127.0.0.1 6395 1
sentinel down-after-milliseconds mymaster 3000
sentinel failover-timeout mymaster 10000
EOF
  redis-server "$BASE/sentinel.conf" --sentinel 2>/dev/null
  sleep 0.5
  echo "sentinel 26379 started"
fi

# 验证哨兵已发现 master
redis-cli -p 26379 sentinel get-master-addr-by-name mymaster
