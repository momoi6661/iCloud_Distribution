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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Share 一个分享链接。
type Share struct {
	Token       string `json:"token"`
	OwnerUserID string `json:"owner_user_id,omitempty"`
	AccountID   string `json:"account_id"`
	Alias       string `json:"alias"`
	Label       string `json:"label,omitempty"`
	CreatedAt   string `json:"created_at"`
	ExpiresAt   string `json:"expires_at,omitempty"`
}

func (s *Store) AssignMissingOwners(ownerForAccount func(string) string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for _, sh := range s.shares {
		if sh.OwnerUserID == "" {
			sh.OwnerUserID = ownerForAccount(sh.AccountID)
			changed = true
		}
	}
	if changed {
		return s.save()
	}
	return nil
}

func (s *Store) SetOwner(token, ownerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sh, ok := s.shares[token]
	if !ok {
		return fmt.Errorf("分享不存在")
	}
	sh.OwnerUserID = ownerID
	return s.save()
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

// Create 为 (accountID, alias) 创建分享链接。同一别名允许创建多个链接。
func (s *Store) Create(accountID, alias, label string, expiresMinutes ...int) (*Share, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	minutes := 0
	if len(expiresMinutes) > 0 {
		minutes = expiresMinutes[0]
	}
	if minutes < 0 || minutes > 5_256_000 {
		return nil, fmt.Errorf("分享有效期必须是 0 到 5256000 分钟，0 表示永久")
	}
	duration := time.Duration(minutes) * time.Minute

	now := time.Now()
	sh := &Share{
		Token:     newToken(),
		AccountID: accountID,
		Alias:     alias,
		Label:     label,
		CreatedAt: now.Format(time.RFC3339),
	}
	if duration > 0 {
		sh.ExpiresAt = now.Add(duration).Format(time.RFC3339)
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
	if !ok || shareExpired(sh, time.Now()) {
		return nil, false
	}
	cp := *sh
	return &cp, true
}

// GetAny returns a stored share for management operations, including expired links.
func (s *Store) GetAny(token string) (*Share, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sh, ok := s.shares[token]
	if !ok {
		return nil, false
	}
	cp := *sh
	return &cp, true
}

// UpdateLabel updates the management note without changing the token or expiry.
func (s *Store) UpdateLabel(token, label string) (*Share, error) {
	label = strings.TrimSpace(label)
	if len(label) > 200 {
		return nil, fmt.Errorf("分享备注不能超过 200 个字符")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sh, ok := s.shares[token]
	if !ok {
		return nil, fmt.Errorf("分享不存在")
	}
	previous := sh.Label
	sh.Label = label
	if err := s.save(); err != nil {
		sh.Label = previous
		return nil, err
	}
	cp := *sh
	return &cp, nil
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
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

func shareExpired(sh *Share, now time.Time) bool {
	if sh == nil || sh.ExpiresAt == "" {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339, sh.ExpiresAt)
	return err != nil || !now.Before(expiresAt)
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

// DeleteMany 批量吊销分享链接，并只写入一次持久化文件。
func (s *Store) DeleteMany(tokens []string) (deleted, notFound int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unique := make([]string, 0, len(tokens))
	seen := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		if token == "" {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		unique = append(unique, token)
	}
	removed := make(map[string]*Share, len(unique))
	for _, token := range unique {
		sh, ok := s.shares[token]
		if !ok {
			notFound++
			continue
		}
		removed[token] = sh
		delete(s.shares, token)
		deleted++
	}
	if deleted == 0 {
		return 0, notFound, nil
	}
	if err := s.save(); err != nil {
		for token, sh := range removed {
			s.shares[token] = sh
		}
		return 0, notFound, err
	}
	return deleted, notFound, nil
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
