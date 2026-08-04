// Package server 提供 HTTP API 与 Web UI 静态资源服务,基于 Gin。
//
// 核心能力:
//   - 账号管理 (增删查、Cookie 更新、App 密码)
//   - 两段式自动授权 (login/start → login/otp)
//   - HME 别名管理 (创建/批量创建/停用/激活/删除)
//   - 邮件读取 (IMAP 优先,Web API 回退)
//   - UI 访问鉴权 (可选,设置 token 后所有 API 需要会话 Cookie)
package server

import (
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"icloud_distribution/internal/account"
	"icloud_distribution/internal/auth"
	"icloud_distribution/internal/share"
)

// Server 封装 Gin 引擎、账号管理器和鉴权组件。
type Server struct {
	mgr    *account.Manager
	logins *auth.LoginStore
	ui     *auth.UIAuth // nil 表示关闭 UI 鉴权
	creds  *auth.CredentialStore
	shares *share.Store
	r      *gin.Engine
	static fs.FS // 前端构建产物 (web/dist)
}

// New 创建 Server。static 为前端构建产物目录 (embed.FS 的子目录)。
// ui 为 nil 且 creds 未初始化时,API 暂时放行,等待首次访问创建管理员账号。
func New(mgr *account.Manager, logins *auth.LoginStore, ui *auth.UIAuth, creds *auth.CredentialStore, shares *share.Store, static fs.FS, debug bool) *Server {
	if !debug {
		gin.SetMode(gin.ReleaseMode)
	}
	if ui == nil && (creds == nil || !creds.Initialized()) {
		log.Printf("提示: 未设置管理员账号,首次访问 Web UI 时请创建")
	}
	s := &Server{mgr: mgr, logins: logins, ui: ui, creds: creds, shares: shares, static: static}
	s.r = gin.Default() // 自带 Logger + Recovery 中间件
	s.register()
	return s
}

// Run 启动 HTTP 服务。
func (s *Server) Run(addr string) error {
	return s.r.Run(addr)
}

// Handler 返回底层 gin 引擎(便于测试)。
func (s *Server) Handler() http.Handler { return s.r }

func (s *Server) register() {
	api := s.r.Group("/api")
	{
		// ===== UI 鉴权 (login/setup/status 无需会话,其余 API 需要) =====
		api.POST("/ui/login", s.uiLogin)
		api.POST("/ui/logout", s.uiLogout)
		api.GET("/ui/status", s.uiStatus)
		api.POST("/ui/setup", s.uiSetup)

		// ===== 公开分享端点 (免登录,只读) =====
		api.GET("/public/share/:token", s.publicShareInfo)
		api.GET("/public/share/:token/inbox", s.publicShareInbox)
		api.GET("/public/share/:token/message", s.publicShareMessage)

		authed := api.Group("")
		authed.Use(s.uiMiddleware())

		// ===== 账号管理 =====
		authed.GET("/accounts", s.listAccounts)
		authed.POST("/accounts", s.addAccount)
		authed.DELETE("/accounts/:id", s.removeAccount)
		authed.POST("/accounts/:id/password", s.setAppPassword)
		authed.PUT("/accounts/:id/cookies", s.updateCookies)

		// ===== 两段式自动授权 =====
		authed.POST("/accounts/:id/login/start", s.loginStart)
		authed.POST("/accounts/:id/login/otp", s.loginOTP)
		authed.GET("/accounts/:id/login/phones", s.loginPhones)
		authed.POST("/accounts/:id/login/sms", s.loginSMS)
		authed.POST("/accounts/:id/login/resend", s.loginResend)

		// ===== HME 别名 =====
		authed.POST("/create", s.createAlias)
		authed.POST("/accounts/:id/aliases/batch", s.batchCreateAliases)
		authed.POST("/accounts/:id/forward-to", s.setForwardTo)
		authed.GET("/aliases", s.listAliases)
		authed.POST("/aliases/:id/deactivate", s.deactivateAlias)
		authed.POST("/aliases/:id/reactivate", s.reactivateAlias)
		authed.DELETE("/aliases/:id", s.deleteAlias)

		// ===== 别名分享链接 (管理) =====
		authed.POST("/aliases/share", s.createShare)
		authed.GET("/shares", s.listShares)
		authed.DELETE("/shares/:token", s.deleteShare)

		// ===== 邮件 =====
		authed.GET("/inbox", s.listInbox)
		authed.GET("/inbox/message", s.getMessage)

		// ===== 系统 =====
		authed.POST("/reload", s.reloadConfig)
	}

	// ===== Web UI 静态资源 (SPA fallback) =====
	s.r.NoRoute(s.serveSPA)
}

// serveSPA 服务前端静态资源,非 /api 路径回退到 index.html (SPA 路由)。
func (s *Server) serveSPA(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		c.JSON(http.StatusNotFound, apiResp{Success: false, Message: "接口不存在"})
		return
	}
	if s.static == nil {
		c.String(http.StatusServiceUnavailable, "前端资源未构建,请先执行 cd web && npm run build")
		return
	}

	path := strings.TrimPrefix(c.Request.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	data, err := fs.ReadFile(s.static, path)
	if err != nil {
		// SPA fallback: 前端路由路径统一回退到 index.html
		path = "index.html"
		data, err = fs.ReadFile(s.static, path)
		if err != nil {
			c.String(http.StatusNotFound, "资源不存在")
			return
		}
	}
	// 直接返回内容,避免 http.ServeFile 对 index.html 的 301 重定向
	c.Data(http.StatusOK, contentType(path), data)
}

// contentType 按扩展名推断 MIME 类型。
func contentType(path string) string {
	switch {
	case strings.HasSuffix(path, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(path, ".js"):
		return "application/javascript; charset=utf-8"
	case strings.HasSuffix(path, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(path, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(path, ".png"):
		return "image/png"
	case strings.HasSuffix(path, ".woff2"):
		return "font/woff2"
	default:
		return "application/octet-stream"
	}
}

// ---- 统一响应 ----

type apiResp struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func ok(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, apiResp{Success: true, Data: data})
}

func fail(c *gin.Context, code int, msg string) {
	c.JSON(code, apiResp{Success: false, Message: msg})
}

// isSessionError 判断错误是否由 iCloud 会话失效引起。
func isSessionError(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "401") || strings.Contains(m, "403") ||
		strings.Contains(m, "session") || strings.Contains(m, "cookie") ||
		strings.Contains(m, "unauthorized") || strings.Contains(m, "认证") ||
		strings.Contains(m, "会话校验失败")
}
