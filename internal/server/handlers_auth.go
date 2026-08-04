// handlers_auth.go - UI 访问鉴权与 iCloud 两段式自动授权。
package server

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
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
//
// 管理员账号未创建时 (首次部署) 暂时放行,等待 /api/ui/setup。
func (s *Server) uiMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.creds != nil && !s.creds.Initialized() {
			c.Next()
			return
		}
		if s.ui != nil && !s.ui.ValidRequest(c.Request) {
			c.JSON(http.StatusUnauthorized, apiResp{
				Success: false,
				Message: "未登录或会话已过期",
				Data:    gin.H{"reason": "ui_auth_expired"},
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

type uiLoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Token    string `json:"token"` // 令牌模式 (-token 启动) 时使用
}

// uiLogin 校验登录,成功则写入会话 Cookie。
// 令牌模式校验 token;管理员账号模式校验 username+password。
func (s *Server) uiLogin(c *gin.Context) {
	if s.ui == nil {
		ok(c, gin.H{"auth_required": false})
		return
	}

	var req uiLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}

	// 管理员账号模式
	if s.creds != nil && req.Username != "" {
		if _, valid := s.creds.Verify(req.Username, req.Password); !valid {
			fail(c, http.StatusUnauthorized, "用户名或密码错误")
			return
		}
		s.ui.IssueCookie(c.Writer)
		ok(c, gin.H{"auth_required": true})
		return
	}

	// 令牌模式
	if req.Token == "" {
		fail(c, http.StatusBadRequest, "参数错误: 请输入用户名密码")
		return
	}
	if !s.ui.CheckToken(req.Token) {
		fail(c, http.StatusUnauthorized, "口令错误")
		return
	}
	s.ui.IssueCookie(c.Writer)
	ok(c, gin.H{"auth_required": true})
}

// uiSetup 首次部署时创建管理员账号 (仅管理员账号模式且未初始化)。
func (s *Server) uiSetup(c *gin.Context) {
	if s.creds == nil {
		fail(c, http.StatusForbidden, "当前为令牌模式,不支持初始化设置")
		return
	}
	if s.creds.Initialized() {
		fail(c, http.StatusForbidden, "管理员账号已存在")
		return
	}

	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: username, password 必填")
		return
	}
	if err := s.creds.Setup(req.Username, req.Password); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	s.ui.IssueCookie(c.Writer)
	ok(c, gin.H{"username": req.Username})
}

// uiLogout 清除会话 Cookie。
func (s *Server) uiLogout(c *gin.Context) {
	if s.ui != nil {
		s.ui.ClearCookie(c.Writer)
	}
	ok(c, gin.H{"message": "已注销"})
}

// uiStatus 返回当前鉴权状态(前端据此决定渲染登录页/初始化页/主界面)。
func (s *Server) uiStatus(c *gin.Context) {
	ok(c, gin.H{
		"auth_required": s.ui != nil,
		"initialized":   s.creds == nil || s.creds.Initialized(),
		"token_mode":    s.creds == nil,
		"authenticated": s.ui == nil || s.ui.ValidRequest(c.Request),
	})
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
		// 需要 2FA: 保存会话,自动请求推送验证码到受信任设备 (尽力而为)
		if err := client.ResendOTP(); err != nil {
			log.Printf("自动推送 2FA 验证码失败 (account=%s): %v", id, err)
		}
		sessionID := s.logins.Put(id, client)
		ok(c, gin.H{"status": "otp_required", "session_id": sessionID})
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
//   GET /api/accounts/:id/login/phones?session_id=xxx
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
//   POST /api/accounts/:id/login/resend  body: {"session_id": "..."}
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
//   POST /api/accounts/:id/login/sms  body: {"session_id": "...", "phone_id": 1}
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
