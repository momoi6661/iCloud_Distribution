// handlers_share.go - 别名分享链接。
//
// 管理端 (需 UI 鉴权): 创建/列出/吊销分享。
// 公开端 (免登录): 持有 token 链接的人可只读查看该别名的邮件。
package server

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"icloud_distribution/internal/mail"
)

// ====================================================================
// 管理端
// ====================================================================

type createShareReq struct {
	AccountID string `json:"account_id" binding:"required"`
	Alias     string `json:"alias" binding:"required"`
	Label     string `json:"label"`
}

// createShare 为别名创建 (或复用) 分享链接。
//   POST /api/aliases/share  body: {"account_id": "...", "alias": "x@icloud.com", "label": "..."}
func (s *Server) createShare(c *gin.Context) {
	var req createShareReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: account_id, alias 必填 — "+err.Error())
		return
	}
	if _, exists := s.mgr.GetAccount(req.AccountID); !exists {
		fail(c, http.StatusNotFound, "账号不存在")
		return
	}

	sh, err := s.shares.Create(req.AccountID, req.Alias, req.Label)
	if err != nil {
		fail(c, http.StatusInternalServerError, "创建分享失败: "+err.Error())
		return
	}
	ok(c, gin.H{
		"token":      sh.Token,
		"alias":      sh.Alias,
		"label":      sh.Label,
		"url":        "/share/" + sh.Token,
		"created_at": sh.CreatedAt,
	})
}

// listShares 列出分享链接。
//   GET /api/shares?account_id=acc_xxx
func (s *Server) listShares(c *gin.Context) {
	ok(c, s.shares.List(c.Query("account_id")))
}

// deleteShare 吊销分享链接。
//   DELETE /api/shares/:token
func (s *Server) deleteShare(c *gin.Context) {
	token := c.Param("token")
	if !s.shares.Delete(token) {
		fail(c, http.StatusNotFound, "分享不存在")
		return
	}
	ok(c, gin.H{"token": token})
}

// ====================================================================
// 公开端 (免登录,只读)
// ====================================================================

// publicShareInfo 返回分享的基本信息。
//   GET /api/public/share/:token
func (s *Server) publicShareInfo(c *gin.Context) {
	sh, exists := s.shares.Get(c.Param("token"))
	if !exists {
		fail(c, http.StatusNotFound, "分享链接不存在或已吊销")
		return
	}
	ok(c, gin.H{
		"alias":      sh.Alias,
		"label":      sh.Label,
		"created_at": sh.CreatedAt,
	})
}

// publicShareInbox 读取分享别名的邮件。
//   GET /api/public/share/:token/inbox?limit=30&days=7
func (s *Server) publicShareInbox(c *gin.Context) {
	sh, exists := s.shares.Get(c.Param("token"))
	if !exists {
		fail(c, http.StatusNotFound, "分享链接不存在或已吊销")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))

	method, messages, err := s.readInbox(sh.AccountID, sh.Alias, limit, days)
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	for i := range messages {
		messages[i].Code = mail.ExtractVerificationCode(messages[i].Subject + "\n" + messages[i].Preview)
	}
	ok(c, gin.H{
		"alias":    sh.Alias,
		"count":    len(messages),
		"messages": messages,
		"method":   method,
	})
}

// publicShareMessage 读取分享别名的单封邮件正文 (仅 IMAP 路径支持)。
//   GET /api/public/share/:token/message?uid=1042
func (s *Server) publicShareMessage(c *gin.Context) {
	sh, exists := s.shares.Get(c.Param("token"))
	if !exists {
		fail(c, http.StatusNotFound, "分享链接不存在或已吊销")
		return
	}
	uid64, err := strconv.ParseUint(c.Query("uid"), 10, 32)
	if err != nil {
		fail(c, http.StatusBadRequest, "参数错误: uid 必须是数字")
		return
	}

	mc, unlock, err := s.mgr.AcquireIMAP(sh.AccountID)
	if err != nil {
		fail(c, http.StatusBadRequest, "正文读取需要 App Password (IMAP)")
		return
	}
	defer unlock.Unlock()

	full, err := mc.GetFull(uint32(uid64), c.Query("folder"))
	if err != nil {
		fail(c, http.StatusBadGateway, "读取邮件正文失败: "+err.Error())
		return
	}
	ok(c, full)
}
