#!/usr/bin/env bash
# 启动 zp-rdm TLS 集成测试用的两个 Redis 实例（幂等，已在跑则跳过）：
#   6381 — TLS，不校验客户端证书
#   6382 — TLS，要求客户端证书（mTLS）
# 证书来自仓库 internal/redisx/testdata/tls/
set -euo pipefail

DIR="/mnt/d/workspace/gzy/zp-rdm/internal/redisx/testdata/tls"

start() {
  local port="$1" auth="$2"
  if redis-cli -p "$port" --tls --cert "$DIR/client.crt" --key "$DIR/client.key" --cacert "$DIR/ca.crt" --insecure ping 2>/dev/null | grep -q PONG; then
    echo "port $port already running"
    return
  fi
  redis-server --port 0 --tls-port "$port" \
    --tls-cert-file "$DIR/server.crt" --tls-key-file "$DIR/server.key" \
    --tls-ca-cert-file "$DIR/ca.crt" --tls-auth-clients "$auth" \
    --daemonize yes --save '' --appendonly no
  sleep 0.5
  echo "port $port started (tls-auth-clients $auth)"
}

start 6381 no
start 6382 yes
