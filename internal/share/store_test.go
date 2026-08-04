package share

import "testing"

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

func TestStore_Idempotent(t *testing.T) {
	s, _ := NewStore(t.TempDir())

	sh1, _ := s.Create("acc_1", "a@icloud.com", "")
	sh2, _ := s.Create("acc_1", "a@icloud.com", "")
	if sh1.Token != sh2.Token {
		t.Error("同一账号同一别名应复用 token")
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
