// Package auth - 管理员凭证存储。
//
// 首次访问时通过 Web UI 创建管理员用户名+密码,持久化到 data/admin.json (0600)。
// 密码以 bcrypt 哈希存储,会话签名密钥派生自密码哈希——修改密码后旧会话全部失效。
package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Credentials 管理员账号。
type Credentials struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	CreatedAt    string `json:"created_at"`
}

// CredentialStore 管理员凭证存储,线程安全。
type CredentialStore struct {
	mu    sync.Mutex
	file  string
	creds *Credentials
}

// NewCredentialStore 加载 dataDir 下的 admin.json。
func NewCredentialStore(dataDir string) (*CredentialStore, error) {
	s := &CredentialStore{file: filepath.Join(dataDir, "admin.json")}
	raw, err := os.ReadFile(s.file)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	var creds Credentials
	if err := json.Unmarshal(raw, &creds); err != nil {
		return nil, err
	}
	s.creds = &creds
	return s, nil
}

// Initialized 返回是否已创建管理员账号。
func (s *CredentialStore) Initialized() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creds != nil
}

// Setup 创建管理员账号。已初始化时返回错误。
// 用户名 2-32 字符,密码至少 6 位。
func (s *CredentialStore) Setup(username, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.creds != nil {
		return fmt.Errorf("管理员账号已存在")
	}
	if len(username) < 2 || len(username) > 32 {
		return fmt.Errorf("用户名长度须为 2-32 字符")
	}
	if len(password) < 6 {
		return fmt.Errorf("密码至少 6 位")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	s.creds = &Credentials{
		Username:     username,
		PasswordHash: string(hash),
		CreatedAt:    time.Now().Format(time.RFC3339),
	}
	return s.save()
}

// Verify 校验用户名密码,并返回会话签名密钥 (派生自密码哈希)。
func (s *CredentialStore) Verify(username, password string) (secret string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.creds == nil || s.creds.Username != username {
		return "", false
	}
	if err := bcrypt.CompareHashAndPassword([]byte(s.creds.PasswordHash), []byte(password)); err != nil {
		return "", false
	}
	return s.creds.PasswordHash, true
}

// SessionSecret 返回当前会话签名密钥 (未初始化时为空)。
func (s *CredentialStore) SessionSecret() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.creds == nil {
		return ""
	}
	return s.creds.PasswordHash
}

func (s *CredentialStore) save() error {
	raw, err := json.MarshalIndent(s.creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.file, raw, 0600)
}
