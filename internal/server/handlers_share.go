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
	AccountID      string `json:"account_id" binding:"required"`
	Alias          string `json:"alias" binding:"required"`
	Label          string `json:"label"`
	ExpiresMinutes int    `json:"expires_minutes"`
}

type batchDeleteSharesReq struct {
	Tokens []string `json:"tokens" binding:"required"`
}

// createShare 为别名创建 (或复用) 分享链接。
//
//	POST /api/aliases/share  body: {"account_id": "...", "alias": "x@icloud.com", "label": "..."}
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

	sh, err := s.shares.Create(req.AccountID, req.Alias, req.Label, req.ExpiresMinutes)
	if err != nil {
		fail(c, http.StatusBadRequest, "创建分享失败: "+err.Error())
		return
	}
	ok(c, gin.H{
		"token":      sh.Token,
		"alias":      sh.Alias,
		"label":      sh.Label,
		"url":        "/share/" + sh.Token,
		"created_at": sh.CreatedAt,
		"expires_at": sh.ExpiresAt,
	})
}

// listShares 列出分享链接。
//
//	GET /api/shares?account_id=acc_xxx
func (s *Server) listShares(c *gin.Context) {
	ok(c, s.shares.List(c.Query("account_id")))
}

// deleteShare 吊销分享链接。
//
//	DELETE /api/shares/:token
func (s *Server) deleteShare(c *gin.Context) {
	token := c.Param("token")
	if !s.shares.Delete(token) {
		fail(c, http.StatusNotFound, "分享不存在")
		return
	}
	ok(c, gin.H{"token": token})
}

// batchDeleteShares 批量吊销分享链接。
func (s *Server) batchDeleteShares(c *gin.Context) {
	var req batchDeleteSharesReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Tokens) == 0 {
		fail(c, http.StatusBadRequest, "参数错误: tokens 必须是非空数组")
		return
	}
	if len(req.Tokens) > 500 {
		fail(c, http.StatusBadRequest, "一次最多删除 500 个分享链接")
		return
	}
	deleted, notFound, err := s.shares.DeleteMany(req.Tokens)
	if err != nil {
		fail(c, http.StatusInternalServerError, "批量删除分享失败: "+err.Error())
		return
	}
	ok(c, gin.H{"requested": len(req.Tokens), "deleted": deleted, "not_found": notFound})
}

// ====================================================================
// 公开端 (免登录,只读)
// ====================================================================

// publicShareInfo 返回分享的基本信息。
//
//	GET /api/public/share/:token
func (s *Server) publicShareInfo(c *gin.Context) {
	sh, exists := s.shares.Get(c.Param("token"))
	if !exists {
		fail(c, http.StatusNotFound, "分享链接不存在、已过期或已吊销")
		return
	}
	ok(c, gin.H{
		"alias":      sh.Alias,
		"label":      sh.Label,
		"created_at": sh.CreatedAt,
		"expires_at": sh.ExpiresAt,
	})
}

// publicShareInbox 读取分享别名的邮件。
//
//	GET /api/public/share/:token/inbox?limit=30&days=7
func (s *Server) publicShareInbox(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Header("Pragma", "no-cache")
	sh, exists := s.shares.Get(c.Param("token"))
	if !exists {
		fail(c, http.StatusNotFound, "分享链接不存在、已过期或已吊销")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))

	method, messages, err := s.readInbox(sh.AccountID, sh.Alias, limit, days, "auto")
	if err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	for i := range messages {
		messages[i].Code = mail.ExtractVerificationCode(messages[i].Subject + "\n" + messages[i].Preview)
		if messages[i].Preview == "" && messages[i].Code != "" {
			messages[i].Preview = "已识别验证码：" + messages[i].Code
		}
	}
	ok(c, gin.H{
		"alias":    sh.Alias,
		"count":    len(messages),
		"messages": messages,
		"method":   method,
	})
}

// publicShareMessage 读取分享别名的单封邮件正文 (仅 IMAP 路径支持)。
//
//	GET /api/public/share/:token/message?uid=1042
func (s *Server) publicShareMessage(c *gin.Context) {
	sh, exists := s.shares.Get(c.Param("token"))
	if !exists {
		fail(c, http.StatusNotFound, "分享链接不存在、已过期或已吊销")
		return
	}
	uid64, err := strconv.ParseUint(c.Query("uid"), 10, 32)
	if err != nil {
		fail(c, http.StatusBadRequest, "参数错误: uid 必须是数字")
		return
	}

	var mc *mail.Client
	var unlock interface{ Unlock() }
	if c.Query("source") == "forward_imap" {
		forwardClient, forwardUnlock, _, forwardErr := s.mgr.AcquireForwardIMAP(sh.AccountID)
		mc, unlock, err = forwardClient, forwardUnlock, forwardErr
	} else {
		imapClient, imapUnlock, imapErr := s.mgr.AcquireIMAP(sh.AccountID)
		mc, unlock, err = imapClient, imapUnlock, imapErr
	}
	if err != nil {
		fail(c, http.StatusBadRequest, "正文读取需要可用的 IMAP 配置")
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
