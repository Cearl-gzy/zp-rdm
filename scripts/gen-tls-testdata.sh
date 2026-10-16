#!/usr/bin/env bash
# 生成 zp-rdm TLS 集成测试专用自签证书（30 年有效期，仅测试用途，可提交仓库）
# 用法：wsl bash scripts/gen-tls-testdata.sh
set -euo pipefail

DIR="/mnt/d/workspace/gzy/zp-rdm/internal/redisx/testdata/tls"
mkdir -p "$DIR"
cd "$DIR"

DAYS=10950

# CA
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout ca.key -out ca.crt -days $DAYS \
  -subj "/CN=zp-rdm-test-ca" 2>/dev/null

# 服务端证书（SAN: localhost / 127.0.0.1）
openssl req -newkey rsa:2048 -nodes \
  -keyout server.key -out server.csr -subj "/CN=localhost" 2>/dev/null
cat > server-ext.cnf <<EOF
subjectAltName=DNS:localhost,IP:127.0.0.1
extendedKeyUsage=serverAuth
EOF
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out server.crt -days $DAYS -extfile server-ext.cnf 2>/dev/null

# 客户端证书（mTLS）
openssl req -newkey rsa:2048 -nodes \
  -keyout client.key -out client.csr -subj "/CN=zp-rdm-test-client" 2>/dev/null
openssl x509 -req -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out client.crt -days $DAYS 2>/dev/null

rm -f *.csr *.srl server-ext.cnf
chmod 644 *.crt *.key

echo "OK: $(ls "$DIR" | tr '\n' ' ')"
