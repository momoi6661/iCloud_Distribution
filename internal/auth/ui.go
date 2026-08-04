// Package auth - UI 访问鉴权。
//
// 启动时通过 -token 或 HME_UI_TOKEN 设置访问口令:
//   - 设置后: 除 /api/ui/login 外的所有 /api/* 请求需要有效会话 Cookie
//   - 未设置: 跳过鉴权(仅限本地可信环境,启动时打警告日志)
//
// 会话 Cookie 格式: "<unix_expiry>|<hmac_sha256(expiry, secret)>",
// HttpOnly + SameSite=Strict,有效期 12 小时。secret 派生自访问口令,
// 服务重启后所有会话失效(会话仅存在于签名的有效期声明中,无服务端状态)。
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
	secret []byte
}

// NewUIAuth 用访问口令创建校验器。token 为空时返回 nil (表示关闭鉴权)。
func NewUIAuth(token string) *UIAuth {
	if token == "" {
		return nil
	}
	sum := sha256.Sum256([]byte("hme-ui:" + token))
	return &UIAuth{secret: sum[:]}
}

// CheckToken 校验用户提交的口令是否正确。
func (a *UIAuth) CheckToken(token string) bool {
	want := sha256.Sum256([]byte("hme-ui:" + token))
	return subtle.ConstantTimeCompare(want[:], a.secret) == 1
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
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(strconv.FormatInt(expiry, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}
