// Package auth - UI 访问鉴权。
//
// 两种模式:
//   1. 启动令牌模式: -token / HME_UI_TOKEN 提供静态口令 (适合自动化/Docker)
//   2. 管理员账号模式 (默认): 首次访问 Web UI 时创建用户名+密码,
//      凭证存 data/admin.json (bcrypt),改密码后旧会话全部失效
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

// NewUIAuth 令牌模式: token 为空时返回 nil (表示关闭鉴权)。
func NewUIAuth(token string) *UIAuth {
	if token == "" {
		return nil
	}
	sum := sha256.Sum256([]byte("hme-ui:" + token))
	return &UIAuth{secretFn: func() []byte { return sum[:] }}
}

// NewUIAuthFromCredentials 管理员账号模式: 密钥派生自存储的密码哈希,
// 修改密码后旧会话自动失效。
func NewUIAuthFromCredentials(creds *CredentialStore) *UIAuth {
	return &UIAuth{secretFn: func() []byte {
		sum := sha256.Sum256([]byte("hme-ui:" + creds.SessionSecret()))
		return sum[:]
	}}
}

// CheckToken 校验启动令牌 (仅令牌模式)。
func (a *UIAuth) CheckToken(token string) bool {
	want := sha256.Sum256([]byte("hme-ui:" + token))
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
