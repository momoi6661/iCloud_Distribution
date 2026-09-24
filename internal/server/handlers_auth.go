// handlers_auth.go - UI 访问鉴权与 iCloud 两段式自动授权。
package server

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"icloud_distribution/internal/auth"
	"icloud_distribution/internal/hme"
)

// ====================================================================
// UI 访问鉴权
// ====================================================================

// uiMiddleware 校验 UI 会话 Cookie。
//
// 鉴权失败的 401 带有 data.reason = "ui_auth_expired" 标记,
// 前端只对带标记的 401 跳转登录页——业务接口的 401 (如 iCloud 登录失败)
// 不应把用户踢出 UI 会话。
func (s *Server) uiMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if authHeader := strings.TrimSpace(c.GetHeader("Authorization")); strings.HasPrefix(authHeader, "Bearer ") {
			account, valid := s.mgr.AuthenticateMCPToken(strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer ")))
			if !valid || account.OwnerUserID == "" {
				c.JSON(http.StatusUnauthorized, apiResp{Success: false, Message: "MCP Token 无效或已撤销"})
				c.Abort()
				return
			}
			role := "user"
			if account.OwnerUserID == auth.SuperadminID {
				role = "superadmin"
			}
			c.Set("ui_identity", auth.Identity{ID: account.OwnerUserID, Username: "mcp:" + account.MCPTokenPrefix, Role: role})
			c.Set("mcp_account_id", account.ID)
			c.Next()
			return
		}
		identity, valid := s.ui.Identity(c.Request)
		if !valid {
			c.JSON(http.StatusUnauthorized, apiResp{
				Success: false,
				Message: "未登录或会话已过期",
				Data:    gin.H{"reason": "ui_auth_expired"},
			})
			c.Abort()
			return
		}
		c.Set("ui_identity", *identity)
		// Keep the system console session alive while it is actively used.
		// This is independent from the iCloud account authorization session.
		s.ui.RefreshSession(c.Writer, c.Request)
		c.Next()
	}
}

type uiLoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// uiLogin 校验登录,成功则写入会话 Cookie。
func (s *Server) uiLogin(c *gin.Context) {
	var req uiLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}

	if req.Username == "" || req.Password == "" {
		fail(c, http.StatusBadRequest, "参数错误: 请输入用户名和密码")
		return
	}
	identity, token, err := s.ui.Login(req.Username, req.Password)
	if err != nil {
		fail(c, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	s.ui.IssueCookie(c.Writer, c.Request, token)
	ok(c, gin.H{"auth_required": true, "user": identity})
}

// uiLogout 清除会话 Cookie。
func (s *Server) uiLogout(c *gin.Context) {
	s.ui.ClearCookie(c.Writer, c.Request)
	ok(c, gin.H{"message": "已注销"})
}

// uiStatus 返回当前鉴权状态。
func (s *Server) uiStatus(c *gin.Context) {
	identity, authenticated := s.ui.Identity(c.Request)
	ok(c, gin.H{"auth_required": true, "authenticated": authenticated, "user": identity})
}

// ====================================================================
// iCloud 两段式自动授权
//   POST /api/accounts/:id/login/start  body: {"password": "..."}
//     → {"status": "done"}                        无需 2FA,登录完成
//     → {"status": "otp_required", "session_id"}  需要 2FA,等待验证码
//   POST /api/accounts/:id/login/otp    body: {"session_id": "...", "code": "123456"}
//     → {"status": "done"}                        2FA 通过,登录完成
// ====================================================================

type loginStartReq struct {
	Password string `json:"password" binding:"required"`
}

func (s *Server) loginStart(c *gin.Context) {
	id := c.Param("id")
	var req loginStartReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: password 必填 — "+err.Error())
		return
	}

	client, email, err := s.mgr.NewLoginClient(id)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}

	err = client.BeginLogin(email, req.Password)
	switch {
	case err == nil:
		// 无需 2FA,直接完成
		if saveErr := s.mgr.UpdateCookies(id, client.Cookies); saveErr != nil {
			fail(c, http.StatusInternalServerError, "登录成功但保存 Cookie 失败: "+saveErr.Error())
			return
		}
		ok(c, gin.H{"status": "done", "cookies_count": len(client.Cookies)})
	case errors.Is(err, hme.ErrOTPRequired):
		// 需要 2FA: 优先获取第一个受信任手机号并发送短信。手机号接口偶尔
		// 会被 Apple 拒绝，但不能因此阻断已建立的登录会话或把它显示成失败。
		phones, phoneErr := client.TrustedPhones()
		method := "device"
		smsSent := false
		warning := ""
		if phoneErr == nil && len(phones) > 0 {
			if err := client.SendSMS(phones[0].ID); err != nil {
				warning = "手机号已获取，但短信发送失败，可改用设备推送"
				log.Printf("自动发送短信验证码失败 (account=%s): %v", id, err)
			} else {
				method = "sms"
				smsSent = true
			}
		} else {
			warning = "已进入双重验证，请使用设备推送；手机号列表暂时不可用"
			if phoneErr != nil {
				log.Printf("获取受信任手机号失败 (account=%s): %v", id, phoneErr)
			}
			if err := client.ResendOTP(); err != nil {
				log.Printf("自动推送 2FA 验证码失败 (account=%s): %v", id, err)
			}
		}
		sessionID := s.logins.Put(id, client)
		ok(c, gin.H{"status": "otp_required", "session_id": sessionID, "method": method, "sms_sent": smsSent, "phones": phones, "warning": warning})
	default:
		msg := err.Error()
		if isSessionError(msg) {
			fail(c, http.StatusUnauthorized, "登录失败: "+msg)
		} else {
			fail(c, http.StatusBadGateway, "登录失败: "+msg)
		}
	}
}

// peekSession 从会话存储中非消耗性地取出并校验账号匹配。
func (s *Server) peekSession(c *gin.Context, accountID, sessionID string) (*hme.Client, bool) {
	owner, client, err := s.logins.Peek(sessionID)
	if err != nil {
		fail(c, http.StatusGone, err.Error())
		return nil, false
	}
	if owner != accountID {
		fail(c, http.StatusBadRequest, "登录会话与账号不匹配")
		return nil, false
	}
	return client, true
}

// loginPhones 获取受信任手机号列表 (短信验证通道)。
//
//	GET /api/accounts/:id/login/phones?session_id=xxx
func (s *Server) loginPhones(c *gin.Context) {
	id := c.Param("id")
	client, sessOK := s.peekSession(c, id, c.Query("session_id"))
	if !sessOK {
		return
	}
	phones, err := client.TrustedPhones()
	if err != nil {
		fail(c, http.StatusBadGateway, "获取手机号列表失败: "+err.Error())
		return
	}
	ok(c, gin.H{"phones": phones})
}

// loginResend 重新推送验证码到受信任设备。
//
//	POST /api/accounts/:id/login/resend  body: {"session_id": "..."}
func (s *Server) loginResend(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		SessionID string `json:"session_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: session_id 必填")
		return
	}
	client, sessOK := s.peekSession(c, id, req.SessionID)
	if !sessOK {
		return
	}
	if err := client.ResendOTP(); err != nil {
		fail(c, http.StatusBadGateway, "请求推送失败: "+err.Error())
		return
	}
	ok(c, gin.H{"sent": true})
}

// loginSMS 向受信任手机号发送短信验证码。
//
//	POST /api/accounts/:id/login/sms  body: {"session_id": "...", "phone_id": 1}
func (s *Server) loginSMS(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		SessionID string `json:"session_id" binding:"required"`
		PhoneID   int    `json:"phone_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: session_id, phone_id 必填")
		return
	}
	client, sessOK := s.peekSession(c, id, req.SessionID)
	if !sessOK {
		return
	}
	if err := client.SendSMS(req.PhoneID); err != nil {
		fail(c, http.StatusBadGateway, "发送短信失败: "+err.Error())
		return
	}
	ok(c, gin.H{"sent": true})
}

type loginOTPReq struct {
	SessionID string `json:"session_id" binding:"required"`
	Code      string `json:"code" binding:"required"`
	Method    string `json:"method"`   // "device" (默认) 或 "sms"
	PhoneID   int    `json:"phone_id"` // method=sms 时必填
}

func (s *Server) loginOTP(c *gin.Context) {
	id := c.Param("id")
	var req loginOTPReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: session_id, code 必填 — "+err.Error())
		return
	}

	accountID, client, err := s.logins.Get(req.SessionID)
	if err != nil {
		fail(c, http.StatusGone, err.Error())
		return
	}
	if accountID != id {
		fail(c, http.StatusBadRequest, "登录会话与账号不匹配")
		return
	}

	if req.Method == "sms" {
		err = client.CompleteSMS(req.PhoneID, req.Code)
	} else {
		err = client.CompleteOTP(req.Code)
	}
	if err != nil {
		fail(c, http.StatusBadGateway, "验证码校验失败: "+err.Error())
		return
	}

	if err := s.mgr.UpdateCookies(id, client.Cookies); err != nil {
		fail(c, http.StatusInternalServerError, "登录成功但保存 Cookie 失败: "+err.Error())
		return
	}
	ok(c, gin.H{"status": "done", "cookies_count": len(client.Cookies)})
}
