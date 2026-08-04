package auth

import (
	"net/http/httptest"
	"testing"
)

func TestCredentialStore_SetupVerify(t *testing.T) {
	s, err := NewCredentialStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewCredentialStore: %v", err)
	}
	if s.Initialized() {
		t.Fatal("初始应为未初始化")
	}

	// 参数校验
	if err := s.Setup("a", "password123"); err == nil {
		t.Error("用户名过短应报错")
	}
	if err := s.Setup("admin", "123"); err == nil {
		t.Error("密码过短应报错")
	}

	if err := s.Setup("admin", "password123"); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if !s.Initialized() {
		t.Fatal("Setup 后应为已初始化")
	}

	// 重复 Setup 应失败
	if err := s.Setup("admin2", "password456"); err == nil {
		t.Error("重复 Setup 应报错")
	}

	// 校验
	if _, ok := s.Verify("admin", "password123"); !ok {
		t.Error("正确凭证应通过")
	}
	if _, ok := s.Verify("admin", "wrong"); ok {
		t.Error("错误密码不应通过")
	}
	if _, ok := s.Verify("nobody", "password123"); ok {
		t.Error("错误用户名不应通过")
	}
}

func TestCredentialStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	s1, _ := NewCredentialStore(dir)
	_ = s1.Setup("admin", "password123")

	s2, err := NewCredentialStore(dir)
	if err != nil {
		t.Fatalf("重新加载失败: %v", err)
	}
	if _, ok := s2.Verify("admin", "password123"); !ok {
		t.Error("持久化后应能校验")
	}
}

func TestCredentialStore_SessionSecret(t *testing.T) {
	s, _ := NewCredentialStore(t.TempDir())
	if s.SessionSecret() != "" {
		t.Error("未初始化时密钥应为空")
	}
	_ = s.Setup("admin", "password123")
	if s.SessionSecret() == "" {
		t.Error("初始化后密钥不应为空")
	}
}

func TestUIAuth_FromCredentials(t *testing.T) {
	s, _ := NewCredentialStore(t.TempDir())
	_ = s.Setup("admin", "password123")

	a := NewUIAuthFromCredentials(s)
	w := httptest.NewRecorder()
	a.IssueCookie(w)

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(w.Result().Cookies()[0])
	if !a.ValidRequest(req) {
		t.Error("凭证模式签发的 Cookie 应通过校验")
	}
}
