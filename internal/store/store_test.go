package store

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"zp-rdm/internal/model"
)

// 加密断言在三大桌面平台生效（windows DPAPI / darwin·linux AES-GCM）；其余平台为透传桩
var encryptionActive = runtime.GOOS == "windows" || runtime.GOOS == "darwin" || runtime.GOOS == "linux"

// cipherPrefix 各平台密文前缀
func cipherPrefix() string {
	if runtime.GOOS == "windows" {
		return "dpapi:"
	}
	return "gcm:"
}

func newTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "zp-rdm", "connections.json")
	s, err := NewAt(path)
	if err != nil {
		t.Fatalf("NewAt: %v", err)
	}
	return s, path
}

func profile(id string) model.ConnectionProfile {
	return model.ConnectionProfile{
		ID: id, Name: "test-" + id, Host: "127.0.0.1", Port: 6379,
		Password: "top-secret-" + id,
		SSH: model.SSHConfig{
			Enabled: true, Host: "jump", Username: "ubuntu",
			Password: "ssh-pass-" + id, PrivateKey: "ssh-key-" + id, Passphrase: "ssh-phrase-" + id,
		},
	}
}

func TestDiskEncryption(t *testing.T) {
	s, path := newTemp(t)
	if err := s.Upsert(profile("c1")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(b)
	if encryptionActive {
		// 磁盘上必须是平台前缀密文，且四个敏感字段的明文都不可见
		if !strings.Contains(content, cipherPrefix()) {
			t.Fatalf("no %s ciphertext found on disk", cipherPrefix())
		}
		for _, plain := range []string{"top-secret-c1", "ssh-pass-c1", "ssh-key-c1", "ssh-phrase-c1"} {
			if strings.Contains(content, plain) {
				t.Fatalf("plaintext %q leaked to disk", plain)
			}
		}
	} else if !strings.Contains(content, "top-secret-c1") {
		t.Fatal("non-windows stub should persist plaintext passthrough")
	}
	// 内存中保持明文
	got, ok := s.Get("c1")
	if !ok {
		t.Fatal("profile not found in memory")
	}
	if got.Password != "top-secret-c1" || got.SSH.Password != "ssh-pass-c1" ||
		got.SSH.PrivateKey != "ssh-key-c1" || got.SSH.Passphrase != "ssh-phrase-c1" {
		t.Fatalf("in-memory values not plaintext: %+v", got)
	}
}

func TestLoadDecrypts(t *testing.T) {
	s, path := newTemp(t)
	_ = s.Upsert(profile("c1"))

	s2, err := NewAt(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Get("c1")
	if !ok {
		t.Fatal("profile lost after reload")
	}
	if got.Password != "top-secret-c1" {
		t.Fatalf("password after reload = %q", got.Password)
	}
	if got.SSH.PrivateKey != "ssh-key-c1" {
		t.Fatalf("ssh key after reload = %q", got.SSH.PrivateKey)
	}
}

func TestLegacyPlaintextMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "connections.json")
	legacy := `{"connections":[{"id":"old","name":"old","host":"127.0.0.1","port":6379,"password":"legacy-plain"}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := NewAt(path)
	if err != nil {
		t.Fatal(err)
	}
	// 内存可读
	if got, _ := s.Get("old"); got.Password != "legacy-plain" {
		t.Fatalf("legacy password not loaded: %q", got.Password)
	}
	// 文件应被自动重写为密文（加密平台；透传平台不迁移）
	if encryptionActive {
		b, _ := os.ReadFile(path)
		if strings.Contains(string(b), "legacy-plain") {
			t.Fatal("legacy plaintext file not migrated to ciphertext")
		}
		if !strings.Contains(string(b), cipherPrefix()) {
			t.Fatalf("migrated file missing %s prefix", cipherPrefix())
		}
	}
}

func TestUpsertDelete(t *testing.T) {
	s, _ := newTemp(t)
	_ = s.Upsert(profile("a"))
	_ = s.Upsert(profile("b"))
	if len(s.List()) != 2 {
		t.Fatalf("list len = %d, want 2", len(s.List()))
	}
	// 覆盖同 ID
	p := profile("a")
	p.Name = "renamed"
	_ = s.Upsert(p)
	if len(s.List()) != 2 {
		t.Fatal("upsert duplicated existing id")
	}
	if got, _ := s.Get("a"); got.Name != "renamed" {
		t.Fatal("upsert did not overwrite")
	}
	_ = s.Delete("a")
	if _, ok := s.Get("a"); ok {
		t.Fatal("delete failed")
	}
	if len(s.List()) != 1 {
		t.Fatalf("list len = %d, want 1", len(s.List()))
	}
}

func TestExportIsPlaintext(t *testing.T) {
	s, dir := newTemp(t)
	_ = s.Upsert(profile("c1"))
	out := filepath.Join(filepath.Dir(dir), "export.json")
	if err := s.Export(out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	// 导出保持明文（可移植性，有意设计）
	if !strings.Contains(string(b), "top-secret-c1") {
		t.Fatal("export should contain plaintext password for portability")
	}
}

func TestImportMerges(t *testing.T) {
	s, dir := newTemp(t)
	_ = s.Upsert(profile("a"))
	in := filepath.Join(filepath.Dir(dir), "import.json")
	content := `{"connections":[
		{"id":"a","name":"a2","host":"h","port":6379,"password":"np"},
		{"id":"c","name":"c","host":"h","port":6379}
	]}`
	if err := os.WriteFile(in, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	n, err := s.Import(in)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("imported %d, want 2", n)
	}
	if len(s.List()) != 2 {
		t.Fatalf("total = %d, want 2 (merge by id)", len(s.List()))
	}
	if got, _ := s.Get("a"); got.Name != "a2" {
		t.Fatal("existing id not overwritten on import")
	}
	// 导入的明文密码落盘时应被加密（仅 Windows）
	if encryptionActive {
		b, _ := os.ReadFile(filepath.Join(dir, "connections.json"))
		if strings.Contains(string(b), `"np"`) {
			t.Fatal("imported plaintext password persisted unencrypted")
		}
	}
}
