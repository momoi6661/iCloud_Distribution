// handlers_account.go - 账号管理接口。
package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) listAccounts(c *gin.Context) {
	ok(c, s.mgr.ListAccounts())
}

type addAccountReq struct {
	Name    string `json:"name" binding:"required"`
	Email   string `json:"email"`   // 可选: Apple ID,用于后续密码登录
	Cookies string `json:"cookies"` // 可选,后续可通过 login/start 获取
	Host    string `json:"host"`
	Proxy   string `json:"proxy"` // HTTP/SOCKS5 代理
}

func (s *Server) addAccount(c *gin.Context) {
	var req addAccountReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: name 必填 — "+err.Error())
		return
	}
	acc, err := s.mgr.AddAccount(req.Name, req.Cookies, req.Host, req.Proxy)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	// 补充设置登录邮箱 (用于两段式授权)
	if req.Email != "" {
		_ = s.mgr.SetLoginEmail(acc.ID, req.Email)
	}
	// 返回时脱敏
	acc.Cookies = nil
	c.JSON(http.StatusCreated, apiResp{Success: true, Data: acc})
}

func (s *Server) removeAccount(c *gin.Context) {
	id := c.Param("id")
	if !s.mgr.RemoveAccount(id) {
		fail(c, http.StatusNotFound, "账号不存在")
		return
	}
	ok(c, gin.H{"id": id})
}

type setPwdReq struct {
	ICloudEmail string `json:"icloud_email" binding:"required"`
	AppPassword string `json:"app_password" binding:"required"`
}

func (s *Server) setAppPassword(c *gin.Context) {
	id := c.Param("id")
	var req setPwdReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: icloud_email, app_password 必填 — "+err.Error())
		return
	}
	if err := s.mgr.SetAppPassword(id, req.ICloudEmail, req.AppPassword); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"id": id, "icloud_email": req.ICloudEmail})
}

type updateCookiesReq struct {
	Cookies map[string]string `json:"cookies" binding:"required"`
}

func (s *Server) updateCookies(c *gin.Context) {
	id := c.Param("id")
	var req updateCookiesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: cookies 必填 — "+err.Error())
		return
	}
	if err := s.mgr.UpdateCookies(id, req.Cookies); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"id": id, "cookies_count": len(req.Cookies)})
}

// reloadConfig 重新加载 accounts.json 配置文件。
func (s *Server) reloadConfig(c *gin.Context) {
	if err := s.mgr.Reload(); err != nil {
		fail(c, http.StatusInternalServerError, "重新加载配置失败: "+err.Error())
		return
	}
	ok(c, gin.H{"message": "配置已重新加载"})
}
