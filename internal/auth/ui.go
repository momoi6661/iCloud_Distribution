// Package auth - UI 访问鉴权。
//
// 登录密码由 HME_UI_PASSWORD 环境变量提供，不在应用数据中创建或存储用户。
//
// 会话 Cookie 格式: "<unix_expiry>|<hmac_sha256(expiry, secret)>",
// HttpOnly + SameSite=Strict,有效期 12 小时。
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// UICookieName 是 UI 会话 Cookie 的名字。
const UICookieName = "hme_ui_session"

// uiSessionTTL UI 会话有效期。
const uiSessionTTL = 12 * time.Hour

// UIAuth 校验 UI 会话 Cookie。
type UIAuth struct {
	secretFn func() []byte
}

// NewUIAuth 从环境变量提供的密码派生会话签名密钥。
func NewUIAuth(password string) *UIAuth {
	if password == "" {
		return nil
	}
	sum := sha256.Sum256([]byte("hme-ui:" + password))
	return &UIAuth{secretFn: func() []byte { return sum[:] }}
}

// CheckPassword 以常量时间校验登录密码。
func (a *UIAuth) CheckPassword(password string) bool {
	want := sha256.Sum256([]byte("hme-ui:" + password))
	return subtle.ConstantTimeCompare(want[:], a.secretFn()) == 1
}

// IssueCookie 生成会话 Cookie 值并写回响应。
func (a *UIAuth) IssueCookie(w http.ResponseWriter) {
	expiry := time.Now().Add(uiSessionTTL).Unix()
	value := fmt.Sprintf("%d|%s", expiry, a.sign(expiry))
	http.SetCookie(w, &http.Cookie{
		Name:     UICookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(uiSessionTTL.Seconds()),
	})
}

// ClearCookie 清除会话 Cookie (注销)。
func (a *UIAuth) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     UICookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// ValidRequest 检查请求是否携带有效会话 Cookie。
func (a *UIAuth) ValidRequest(r *http.Request) bool {
	cookie, err := r.Cookie(UICookieName)
	if err != nil {
		return false
	}

	var expiry int64
	var sig string
	if _, err := fmt.Sscanf(cookie.Value, "%d|%s", &expiry, &sig); err != nil {
		return false
	}
	if time.Now().Unix() > expiry {
		return false
	}
	want := a.sign(expiry)
	return subtle.ConstantTimeCompare([]byte(sig), []byte(want)) == 1
}

// sign 计算 expiry 的 HMAC 签名。
func (a *UIAuth) sign(expiry int64) string {
	mac := hmac.New(sha256.New, a.secretFn())
	mac.Write([]byte(strconv.FormatInt(expiry, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}
