package share

import (
	"testing"
	"time"
)

func TestStore_CreateGetDelete(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	sh, err := s.Create("acc_1", "a@icloud.com", "测试")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sh.Token == "" {
		t.Fatal("token 不应为空")
	}

	got, ok := s.Get(sh.Token)
	if !ok {
		t.Fatal("Get 应找到刚创建的分享")
	}
	if got.Alias != "a@icloud.com" || got.AccountID != "acc_1" || got.Label != "测试" {
		t.Errorf("分享内容不符: %+v", got)
	}

	if !s.Delete(sh.Token) {
		t.Error("Delete 应返回 true")
	}
	if _, ok := s.Get(sh.Token); ok {
		t.Error("删除后 Get 不应找到")
	}
	if s.Delete(sh.Token) {
		t.Error("重复删除应返回 false")
	}
}

func TestStore_MultipleLinksPerAlias(t *testing.T) {
	s, _ := NewStore(t.TempDir())

	sh1, _ := s.Create("acc_1", "a@icloud.com", "")
	sh2, _ := s.Create("acc_1", "a@icloud.com", "")
	if sh1.Token == sh2.Token {
		t.Error("同一别名应允许创建多个不同 token")
	}

	sh3, _ := s.Create("acc_1", "b@icloud.com", "")
	if sh3.Token == sh1.Token {
		t.Error("不同别名应生成不同 token")
	}
}

func TestStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	s1, _ := NewStore(dir)
	sh, _ := s1.Create("acc_1", "a@icloud.com", "")

	// 重新加载应能读回
	s2, err := NewStore(dir)
	if err != nil {
		t.Fatalf("重新加载失败: %v", err)
	}
	if _, ok := s2.Get(sh.Token); !ok {
		t.Error("持久化后重新加载应能找到分享")
	}
}

func TestStore_UpdateLabel(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	sh, _ := s.Create("acc_1", "a@icloud.com", "旧备注")
	updated, err := s.UpdateLabel(sh.Token, "  新备注  ")
	if err != nil || updated.Label != "新备注" {
		t.Fatalf("UpdateLabel = %+v, %v", updated, err)
	}
	reloaded, _ := NewStore(dir)
	got, ok := reloaded.GetAny(sh.Token)
	if !ok || got.Label != "新备注" {
		t.Fatalf("重新加载后的备注 = %+v", got)
	}
}

func TestStore_List(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	s.Create("acc_1", "a@icloud.com", "")
	s.Create("acc_1", "b@icloud.com", "")
	s.Create("acc_2", "c@icloud.com", "")

	if got := len(s.List("acc_1")); got != 2 {
		t.Errorf("acc_1 应有 2 个分享, 实际 %d", got)
	}
	if got := len(s.List("")); got != 3 {
		t.Errorf("全部应有 3 个分享, 实际 %d", got)
	}
}

func TestStore_CustomMinuteExpiry(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	before := time.Now().Add(119 * time.Minute)
	sh, err := s.Create("acc_1", "minute@icloud.com", "限时", 120)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	expiresAt, err := time.Parse(time.RFC3339, sh.ExpiresAt)
	if err != nil || expiresAt.Before(before) || expiresAt.After(time.Now().Add(121*time.Minute)) {
		t.Fatalf("自定义分钟到期时间异常: %q", sh.ExpiresAt)
	}
	if _, ok := s.Get(sh.Token); !ok {
		t.Fatal("未到期链接应可读取")
	}
	if _, err := s.Create("acc_1", "bad@icloud.com", "", -1); err == nil {
		t.Fatal("负数分钟应被拒绝")
	}
}

func TestStore_ExpiredLinkIsHidden(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	sh, _ := s.Create("acc_1", "expired@icloud.com", "")
	s.shares[sh.Token].ExpiresAt = time.Now().Add(-time.Minute).Format(time.RFC3339)
	if _, ok := s.Get(sh.Token); ok {
		t.Fatal("过期链接不应再被读取")
	}
	if got := len(s.List("acc_1")); got != 1 {
		t.Fatalf("管理列表应保留过期链接: %d", got)
	}
}

func TestStore_DeleteMany(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	first, _ := s.Create("acc_1", "a@icloud.com", "")
	second, _ := s.Create("acc_1", "b@icloud.com", "")
	deleted, notFound, err := s.DeleteMany([]string{first.Token, second.Token, first.Token, "missing"})
	if err != nil || deleted != 2 || notFound != 1 {
		t.Fatalf("DeleteMany = deleted %d, notFound %d, err %v", deleted, notFound, err)
	}
	if got := len(s.List("acc_1")); got != 0 {
		t.Fatalf("批量删除后应为空: %d", got)
	}
}
