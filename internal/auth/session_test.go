package auth

import (
	"testing"
	"time"

	"icloud_distribution/internal/hme"
)

func TestLoginStore_PutGet(t *testing.T) {
	s := NewLoginStore()
	defer s.Close()

	client := &hme.Client{}
	id := s.Put("acc_1", client)
	if id == "" {
		t.Fatal("Put 应返回非空会话 ID")
	}

	accountID, got, err := s.Get(id)
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if accountID != "acc_1" {
		t.Errorf("accountID = %q, 期望 acc_1", accountID)
	}
	if got != client {
		t.Error("Get 返回的 client 与 Put 的不一致")
	}
}

func TestLoginStore_GetTwiceFails(t *testing.T) {
	s := NewLoginStore()
	defer s.Close()

	id := s.Put("acc_1", &hme.Client{})
	if _, _, err := s.Get(id); err != nil {
		t.Fatalf("第一次 Get 失败: %v", err)
	}
	// 会话是一次性的: 第二次必须失败
	if _, _, err := s.Get(id); err == nil {
		t.Error("第二次 Get 应失败 (会话已消费)")
	}
}

func TestLoginStore_UnknownID(t *testing.T) {
	s := NewLoginStore()
	defer s.Close()

	if _, _, err := s.Get("不存在的会话"); err == nil {
		t.Error("未知会话 ID 应返回错误")
	}
}

func TestLoginStore_Expired(t *testing.T) {
	s := NewLoginStore()
	defer s.Close()

	id := s.Put("acc_1", &hme.Client{})
	// 手动把过期时间改到过去 (同包测试可直接操作内部结构)
	s.mu.Lock()
	s.entries[id].expiresAt = time.Now().Add(-time.Minute)
	s.mu.Unlock()

	if _, _, err := s.Get(id); err == nil {
		t.Error("过期会话应返回错误")
	}
}

func TestLoginStore_Concurrent(t *testing.T) {
	s := NewLoginStore()
	defer s.Close()

	done := make(chan string, 100)
	for i := 0; i < 100; i++ {
		go func() {
			done <- s.Put("acc_x", &hme.Client{})
		}()
	}
	for i := 0; i < 100; i++ {
		id := <-done
		go func(id string) {
			_, _, _ = s.Get(id)
		}(id)
	}
}
