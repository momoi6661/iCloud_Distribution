package auth

import (
	"net/http/httptest"
	"testing"
)

func TestNewUIAuth_EmptyToken(t *testing.T) {
	if NewUIAuth("") != nil {
		t.Error("空口令应返回 nil (关闭鉴权)")
	}
}

func TestUIAuth_CheckToken(t *testing.T) {
	a := NewUIAuth("secret123")

	if !a.CheckToken("secret123") {
		t.Error("正确口令应通过校验")
	}
	if a.CheckToken("wrong") {
		t.Error("错误口令不应通过校验")
	}
	if a.CheckToken("") {
		t.Error("空口令不应通过校验")
	}
}

func TestUIAuth_CookieRoundtrip(t *testing.T) {
	a := NewUIAuth("secret123")

	// 签发 Cookie
	w := httptest.NewRecorder()
	a.IssueCookie(w)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("应签发 1 个 Cookie, 实际 %d", len(cookies))
	}
	if !cookies[0].HttpOnly {
		t.Error("会话 Cookie 必须是 HttpOnly")
	}

	// 携带 Cookie 的请求应校验通过
	req := httptest.NewRequest("GET", "/api/accounts", nil)
	req.AddCookie(cookies[0])
	if !a.ValidRequest(req) {
		t.Error("有效 Cookie 应通过校验")
	}

	// 无 Cookie 请求应拒绝
	if a.ValidRequest(httptest.NewRequest("GET", "/api/accounts", nil)) {
		t.Error("无 Cookie 请求不应通过校验")
	}

	// 篡改签名的 Cookie 应拒绝 (确定性翻转签名中段一个字符)
	tampered := *cookies[0]
	mid := len(tampered.Value) / 2
	flip := byte('0')
	if tampered.Value[mid] == '0' {
		flip = '1'
	}
	tampered.Value = tampered.Value[:mid] + string(flip) + tampered.Value[mid+1:]
	req2 := httptest.NewRequest("GET", "/api/accounts", nil)
	req2.AddCookie(&tampered)
	if a.ValidRequest(req2) {
		t.Error("篡改的 Cookie 不应通过校验")
	}
}

func TestUIAuth_ClearCookie(t *testing.T) {
	a := NewUIAuth("secret123")
	w := httptest.NewRecorder()
	a.ClearCookie(w)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Error("ClearCookie 应设置 MaxAge=-1 使浏览器删除 Cookie")
	}
}

// 不同口令签发的 Cookie 不能混用 (换口令即全部会话失效)
func TestUIAuth_DifferentSecret(t *testing.T) {
	a1 := NewUIAuth("token-a")
	a2 := NewUIAuth("token-b")

	w := httptest.NewRecorder()
	a1.IssueCookie(w)

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(w.Result().Cookies()[0])
	if a2.ValidRequest(req) {
		t.Error("其他口令签发的 Cookie 不应通过校验")
	}
}
