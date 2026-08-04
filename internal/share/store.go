// Package share 实现邮件别名的公开分享链接存储。
//
// 每个 Share 把一个随机 token 绑定到 (账号, 别名) 上,
// 持有链接的人无需登录即可通过公开端点查看该别名收到的邮件 (只读)。
// 持久化到 data/shares.json (0600,含敏感访问凭证)。
package share

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Share 一个分享链接。
type Share struct {
	Token     string `json:"token"`
	AccountID string `json:"account_id"`
	Alias     string `json:"alias"`
	Label     string `json:"label,omitempty"`
	CreatedAt string `json:"created_at"`
}

// Store 分享链接存储,线程安全。
type Store struct {
	mu     sync.Mutex
	shares map[string]*Share
	file   string
}

// NewStore 加载或创建 dataDir 下的 shares.json。
func NewStore(dataDir string) (*Store, error) {
	s := &Store{
		shares: make(map[string]*Share),
		file:   filepath.Join(dataDir, "shares.json"),
	}
	raw, err := os.ReadFile(s.file)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	var wrapper struct {
		Shares map[string]*Share `json:"shares"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, err
	}
	if wrapper.Shares != nil {
		s.shares = wrapper.Shares
	}
	return s, nil
}

// Create 为 (accountID, alias) 创建分享链接。同一别名重复调用返回已有链接。
func (s *Store) Create(accountID, alias, label string) (*Share, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 幂等: 同一账号同一别名复用已有 token
	for _, sh := range s.shares {
		if sh.AccountID == accountID && sh.Alias == alias {
			return sh, nil
		}
	}

	sh := &Share{
		Token:     newToken(),
		AccountID: accountID,
		Alias:     alias,
		Label:     label,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	s.shares[sh.Token] = sh
	if err := s.save(); err != nil {
		delete(s.shares, sh.Token)
		return nil, err
	}
	return sh, nil
}

// Get 按 token 查找分享。
func (s *Store) Get(token string) (*Share, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sh, ok := s.shares[token]
	return sh, ok
}

// List 返回指定账号的全部分享 (accountID 为空则返回全部)。
func (s *Store) List(accountID string) []*Share {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Share, 0, len(s.shares))
	for _, sh := range s.shares {
		if accountID == "" || sh.AccountID == accountID {
			cp := *sh
			out = append(out, &cp)
		}
	}
	return out
}

// Delete 吊销分享链接。
func (s *Store) Delete(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.shares[token]; !ok {
		return false
	}
	delete(s.shares, token)
	_ = s.save()
	return true
}

func (s *Store) save() error {
	wrapper := struct {
		Shares map[string]*Share `json:"shares"`
	}{s.shares}
	raw, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.file, raw, 0600)
}

// newToken 生成 192-bit 随机 token (URL 安全)。
func newToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
