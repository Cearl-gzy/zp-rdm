//go:build darwin || linux

// Package secrets macOS/Linux 实现：AES-256-GCM 自包含密文（gcm: 前缀），
// 主密钥优先存系统密钥环（macOS Keychain / Linux Secret Service），
// 密钥环不可用时回退 ~/.config/zp-rdm/.master-key（0600）。
// 结构与 Windows DPAPI 实现对齐：密文自包含、换机无法解密。
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
)

const gcmPrefix = "gcm:"

const (
	keyringService = "zp-rdm"
	keyringUser    = "master-key"
	masterKeyLen   = 32 // AES-256
)

var (
	masterKeyOnce sync.Once
	masterKeyVal  []byte
	masterKeyErr  error
)

func masterKey() ([]byte, error) {
	masterKeyOnce.Do(func() {
		masterKeyVal, masterKeyErr = loadOrCreateMasterKey()
	})
	return masterKeyVal, masterKeyErr
}

func loadOrCreateMasterKey() ([]byte, error) {
	s, err := keyring.Get(keyringService, keyringUser)
	if err == nil {
		if raw, derr := base64.StdEncoding.DecodeString(s); derr == nil && len(raw) == masterKeyLen {
			return raw, nil
		}
		// 密钥环中数据损坏：继续走重建流程
	} else if !errors.Is(err, keyring.ErrNotFound) {
		// 密钥环不可用（无 D-Bus / 无桌面环境）→ 密钥文件回退
		return loadOrCreateKeyFile()
	}
	// 密钥环为空（或损坏）：先复用密钥文件——覆盖"密钥环曾不可用、现已恢复"的场景，
	// 否则换密钥后历史密文全部无法解密
	if raw, ferr := readKeyFile(); ferr == nil {
		_ = keyring.Set(keyringService, keyringUser, base64.StdEncoding.EncodeToString(raw)) // 顺手迁移
		return raw, nil
	}
	raw := make([]byte, masterKeyLen)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	if err := keyring.Set(keyringService, keyringUser, base64.StdEncoding.EncodeToString(raw)); err != nil {
		return saveKeyFile(raw)
	}
	return raw, nil
}

func keyFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "zp-rdm", ".master-key"), nil
}

func readKeyFile() ([]byte, error) {
	p, err := keyFilePath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(raw) != masterKeyLen {
		return nil, errors.New("密钥文件格式错误")
	}
	return raw, nil
}

func loadOrCreateKeyFile() ([]byte, error) {
	if raw, err := readKeyFile(); err == nil {
		return raw, nil
	}
	raw := make([]byte, masterKeyLen)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	return saveKeyFile(raw)
}

func saveKeyFile(raw []byte) ([]byte, error) {
	p, err := keyFilePath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(p, []byte(base64.StdEncoding.EncodeToString(raw)), 0o600); err != nil {
		return nil, err
	}
	return raw, nil
}

// Protect 加密字符串，返回带 gcm: 前缀的 base64(nonce‖密文)；空串原样返回
func Protect(plain string) (string, error) {
	if plain == "" {
		return plain, nil
	}
	key, err := masterKey()
	if err != nil {
		return "", err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nil, nonce, []byte(plain), nil)
	return gcmPrefix + base64.StdEncoding.EncodeToString(append(nonce, ct...)), nil
}

// Unprotect 解密；非 gcm: 密文（旧版明文/他机导出）原样返回，解密失败返回空串
func Unprotect(s string) string {
	if !IsProtected(s) {
		return s
	}
	raw, err := base64.StdEncoding.DecodeString(s[len(gcmPrefix):])
	if err != nil {
		return ""
	}
	key, err := masterKey()
	if err != nil {
		return ""
	}
	gcm, err := newGCM(key)
	if err != nil || len(raw) < gcm.NonceSize() {
		return ""
	}
	pt, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return ""
	}
	return string(pt)
}

// IsProtected 判断是否为已加密密文
func IsProtected(s string) bool {
	return len(s) > len(gcmPrefix) && s[:len(gcmPrefix)] == gcmPrefix
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
