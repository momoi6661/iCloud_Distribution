// Package auth 提供登录会话存储和 UI 访问鉴权。
//
// LoginStore 保存两段式登录 (BeginLogin → CompleteOTP) 的中间状态:
// 持有 2FA 等待中的 *hme.Client (含 cookie jar 与 SRP 中间态),
// 键为一次性会话 ID,TTL 到期自动清除。密码等敏感信息只存在于内存,不落盘。
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"icloud_distribution/internal/hme"
)

// LoginTTL 登录会话有效期。2FA 验证码通常几秒内送达,5 分钟足够。
const LoginTTL = 5 * time.Minute

// loginEntry 一个等待 2FA 的登录会话。
type loginEntry struct {
	accountID string
	client    *hme.Client
	expiresAt time.Time
}

// LoginStore 内存登录会话存储,线程安全。
type LoginStore struct {
	mu      sync.Mutex
	entries map[string]*loginEntry
	stopGC  chan struct{}
}

// NewLoginStore 创建存储并启动后台过期清理。
func NewLoginStore() *LoginStore {
	s := &LoginStore{
		entries: make(map[string]*loginEntry),
		stopGC:  make(chan struct{}),
	}
	go s.gcLoop()
	return s
}

// Close 停止后台清理 goroutine。
func (s *LoginStore) Close() { close(s.stopGC) }

// Put 保存一个等待 2FA 的登录会话,返回会话 ID。
func (s *LoginStore) Put(accountID string, client *hme.Client) string {
	id := newSessionID()
	s.mu.Lock()
	s.entries[id] = &loginEntry{
		accountID: accountID,
		client:    client,
		expiresAt: time.Now().Add(LoginTTL),
	}
	s.mu.Unlock()
	return id
}

// Get 取出会话。过期或不存在的会话返回错误。
// 取出后从存储中移除——验证码只能提交一次,失败后需重新发起登录。
func (s *LoginStore) Get(id string) (string, *hme.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.entries[id]
	if !ok {
		return "", nil, fmt.Errorf("登录会话不存在或已过期,请重新发起登录")
	}
	delete(s.entries, id)

	if time.Now().After(e.expiresAt) {
		return "", nil, fmt.Errorf("登录会话已过期,请重新发起登录")
	}
	return e.accountID, e.client, nil
}

// Peek 查看会话但不移除,用于登录过程中的辅助操作 (查手机号、发短信)。
func (s *LoginStore) Peek(id string) (string, *hme.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.entries[id]
	if !ok {
		return "", nil, fmt.Errorf("登录会话不存在或已过期,请重新发起登录")
	}
	if time.Now().After(e.expiresAt) {
		return "", nil, fmt.Errorf("登录会话已过期,请重新发起登录")
	}
	return e.accountID, e.client, nil
}

// gcLoop 每分钟清理一次过期会话。
func (s *LoginStore) gcLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopGC:
			return
		case now := <-ticker.C:
			s.mu.Lock()
			for id, e := range s.entries {
				if now.After(e.expiresAt) {
					delete(s.entries, id)
				}
			}
			s.mu.Unlock()
		}
	}
}

// newSessionID 生成 128-bit 随机会话 ID。
func newSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
