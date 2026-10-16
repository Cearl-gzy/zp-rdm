package redisx

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"zp-rdm/internal/model"
)

// ---------- buildTLSConfig 单元测试（内存生成证书，不依赖网络） ----------

// genCertPEM 生成一张 ECDSA 证书与其私钥的 PEM（parent 为 nil 时自签 CA）
func genCertPEM(t *testing.T, cn string, isCA bool, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (certPEM, keyPEM string, cert *x509.Certificate, key *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  isCA,
	}
	if isCA {
		tpl.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature
	} else {
		tpl.KeyUsage = x509.KeyUsageDigitalSignature
		tpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
		tpl.DNSNames = []string{"localhost"}
		tpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}
	if parent == nil {
		parent, parentKey = tpl, key
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, parent, &key.PublicKey, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	return certPEM, keyPEM, cert, key
}

func TestBuildTLSConfigDefaults(t *testing.T) {
	// 旧配置（TLSSkipVerify 为 nil）保持历史行为：跳过校验
	tc, err := buildTLSConfig(model.ConnectionProfile{UseTLS: true})
	if err != nil {
		t.Fatal(err)
	}
	if !tc.InsecureSkipVerify {
		t.Error("nil TLSSkipVerify 应按跳过校验处理")
	}
	if tc.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %x, want TLS1.2", tc.MinVersion)
	}

	f := false
	tc, err = buildTLSConfig(model.ConnectionProfile{UseTLS: true, TLSSkipVerify: &f})
	if err != nil {
		t.Fatal(err)
	}
	if tc.InsecureSkipVerify {
		t.Error("显式 false 不应跳过校验")
	}
	if tc.RootCAs != nil {
		t.Error("未提供 CA 时 RootCAs 应为 nil（走系统池）")
	}
}

func TestBuildTLSConfigErrors(t *testing.T) {
	// 无效 CA PEM
	if _, err := buildTLSConfig(model.ConnectionProfile{UseTLS: true, TLSCaCert: "not a pem"}); err == nil {
		t.Error("无效 CA PEM 应报错")
	}
	// mTLS 只填证书不填私钥
	if _, err := buildTLSConfig(model.ConnectionProfile{UseTLS: true, TLSClientCert: "x"}); err == nil {
		t.Error("只填客户端证书应报错")
	}
	// mTLS 只填私钥不填证书
	if _, err := buildTLSConfig(model.ConnectionProfile{UseTLS: true, TLSClientKey: "x"}); err == nil {
		t.Error("只填客户端私钥应报错")
	}
	// 证书与私钥不匹配/无效
	if _, err := buildTLSConfig(model.ConnectionProfile{UseTLS: true, TLSClientCert: "a", TLSClientKey: "b"}); err == nil {
		t.Error("无效证书对应报错")
	}
}

func TestBuildTLSConfigCAAndMTLS(t *testing.T) {
	caPEM, _, caCert, caKey := genCertPEM(t, "test-ca", true, nil, nil)
	clientCert, clientKey, _, _ := genCertPEM(t, "test-client", false, caCert, caKey)

	tc, err := buildTLSConfig(model.ConnectionProfile{
		UseTLS:        true,
		TLSCaCert:     caPEM,
		TLSClientCert: clientCert,
		TLSClientKey:  clientKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tc.RootCAs == nil {
		t.Error("提供 CA 后 RootCAs 应非空")
	}
	if len(tc.Certificates) != 1 {
		t.Fatalf("Certificates = %d, want 1", len(tc.Certificates))
	}
}

// ---------- 真实 TLS Redis 集成测试 ----------
// 需要一台启用了 TLS 的 Redis（自签证书），未设置环境变量时跳过：
//
//	ZP_RDM_TEST_TLS_ADDR       TLS Redis 地址（host:port，不校验客户端证书）
//	ZP_RDM_TEST_TLS_MTLS_ADDR  要求客户端证书的 TLS Redis 地址（可选）
//
// 证书位于 testdata/tls/（测试专用自签，30 年有效期）。
func tlsTestEnv(t *testing.T) (addr, caPEM string) {
	t.Helper()
	addr = os.Getenv("ZP_RDM_TEST_TLS_ADDR")
	if addr == "" {
		t.Skip("未设置 ZP_RDM_TEST_TLS_ADDR，跳过 TLS 集成测试")
	}
	b, err := os.ReadFile(filepath.Join("testdata", "tls", "ca.crt"))
	if err != nil {
		t.Skipf("缺少测试 CA 证书 testdata/tls/ca.crt: %v", err)
	}
	return addr, string(b)
}

func tlsProfile(addr string, mutate func(*model.ConnectionProfile)) model.ConnectionProfile {
	host, portStr, _ := strings.Cut(addr, ":")
	port, _ := strconv.Atoi(portStr)
	p := model.ConnectionProfile{ID: "test-tls-conn", Name: "tls-test", Host: host, Port: port, UseTLS: true}
	if mutate != nil {
		mutate(&p)
	}
	return p
}

func TestTLSConnect(t *testing.T) {
	addr, caPEM := tlsTestEnv(t)
	m := NewManager()
	ctx := context.Background()

	// 1) 跳过校验 → 成功
	p := tlsProfile(addr, nil)
	if err := m.Test(ctx, p); err != nil {
		t.Errorf("跳过校验连接应成功: %v", err)
	}

	// 2) 严格校验 + 自定义 CA → 成功
	noSkip := false
	p = tlsProfile(addr, func(p *model.ConnectionProfile) {
		p.TLSSkipVerify = &noSkip
		p.TLSCaCert = caPEM
	})
	if err := m.Test(ctx, p); err != nil {
		t.Errorf("自定义 CA 校验连接应成功: %v", err)
	}

	// 3) 严格校验、无 CA → 失败（自签证书不在系统信任链）
	p = tlsProfile(addr, func(p *model.ConnectionProfile) {
		p.TLSSkipVerify = &noSkip
	})
	if err := m.Test(ctx, p); err == nil {
		t.Error("无 CA 严格校验自签证书应失败")
	}
}

func TestTLSMutualTLS(t *testing.T) {
	addr := os.Getenv("ZP_RDM_TEST_TLS_MTLS_ADDR")
	if addr == "" {
		t.Skip("未设置 ZP_RDM_TEST_TLS_MTLS_ADDR，跳过 mTLS 集成测试")
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join("testdata", "tls", name))
		if err != nil {
			t.Skipf("缺少 %s: %v", name, err)
		}
		return string(b)
	}
	caPEM, clientCert, clientKey := read("ca.crt"), read("client.crt"), read("client.key")

	m := NewManager()
	ctx := context.Background()

	// 带客户端证书 → 成功
	p := tlsProfile(addr, func(p *model.ConnectionProfile) {
		p.TLSCaCert = caPEM
		p.TLSClientCert = clientCert
		p.TLSClientKey = clientKey
	})
	if err := m.Test(ctx, p); err != nil {
		t.Errorf("mTLS 带客户端证书应成功: %v", err)
	}

	// 不带客户端证书 → 失败（服务端 tls-auth-clients yes）
	p = tlsProfile(addr, func(p *model.ConnectionProfile) {
		p.TLSCaCert = caPEM
	})
	if err := m.Test(ctx, p); err == nil {
		t.Error("mTLS 不带客户端证书应失败")
	}
}
