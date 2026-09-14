// handlers_inbox.go - 邮件读取接口。
//
//	GET /api/inbox?account_id=acc_xxx[&alias=xxx@icloud.com][&limit=20][&days=7]
//	GET /api/inbox/message?account_id=acc_xxx&uid=1042   (仅 IMAP 路径支持正文)
//
// 认证优先级: IMAP (App Password) 优先 > Web API (Cookie) 回退。
package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"icloud_distribution/internal/mail"
)

// readInbox 按指定方式读取邮件。preferred 为空或 auto 时按
// 转发邮箱 IMAP > iCloud IMAP > Web API 的顺序自动选择。
// 供 /api/inbox 与公开分享端点共用。
func (s *Server) readInbox(accountID, alias string, limit, days int, preferred string) (string, []mail.Message, error) {
	switch preferred {
	case "forward_imap":
		return s.readForwardInbox(accountID, alias, limit, days)
	case "imap":
		return s.readICloudIMAPInbox(accountID, alias, limit, days)
	case "web_api":
		return s.readWebInbox(accountID, alias, limit)
	case "", "auto":
		// 继续自动选择。
	default:
		return "", nil, fmt.Errorf("不支持的邮件读取方式: %s", preferred)
	}

	if method, messages, err := s.readForwardInbox(accountID, alias, limit, days); err == nil {
		return method, messages, nil
	}
	if method, messages, err := s.readICloudIMAPInbox(accountID, alias, limit, days); err == nil {
		return method, messages, nil
	}
	return s.readWebInbox(accountID, alias, limit)
}

func (s *Server) readForwardInbox(accountID, alias string, limit, days int) (string, []mail.Message, error) {
	mc, unlock, folders, err := s.mgr.AcquireForwardIMAP(accountID)
	if err != nil {
		return "", nil, err
	}
	var messages []mail.Message
	var readErr error
	if alias != "" {
		messages, readErr = mc.ListForwardedByAlias(alias, folders, limit, days)
	} else {
		client, clientErr := s.mgr.HMEClient(accountID, false)
		if clientErr != nil {
			unlock.Unlock()
			return "", nil, clientErr
		}
		aliases, aliasErr := client.ListAliases()
		_ = s.mgr.SaveCookies(accountID, client.Cookies)
		if aliasErr != nil {
			unlock.Unlock()
			return "", nil, aliasErr
		}
		emails := make([]string, 0, len(aliases))
		for _, item := range aliases {
			if item.Email != "" {
				emails = append(emails, item.Email)
			}
		}
		messages, readErr = mc.ListForwardedByAliases(emails, folders, limit, days)
	}
	unlock.Unlock()
	if readErr != nil {
		return "", nil, readErr
	}
	if messages == nil {
		messages = []mail.Message{}
	}
	mail.SortMessagesNewest(messages)
	return "forward_imap", messages, nil
}

func (s *Server) readICloudIMAPInbox(accountID, alias string, limit, days int) (string, []mail.Message, error) {
	mc, unlock, err := s.mgr.AcquireIMAP(accountID)
	if err != nil {
		return "", nil, err
	}
	var messages []mail.Message
	if alias != "" {
		messages, err = mc.FindByRecipient(alias, limit, days)
	} else {
		messages, err = mc.ListInbox(limit, days)
	}
	unlock.Unlock()
	if err != nil {
		return "", nil, err
	}
	if messages == nil {
		messages = []mail.Message{}
	}
	mail.SortMessagesNewest(messages)
	return "imap", messages, nil
}

func (s *Server) readWebInbox(accountID, alias string, limit int) (string, []mail.Message, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		wmc, err := s.mgr.WebMailClient(accountID)
		if err != nil {
			return "", nil, fmt.Errorf("Web API 不可用: %w", err)
		}
		var messages []mail.Message
		if alias != "" {
			messages, err = wmc.FindByAlias(alias, limit)
		} else {
			messages, err = wmc.ListInbox(limit)
		}
		if err == nil {
			s.mgr.CacheGateway(accountID, wmc.GatewayURL())
			if messages == nil {
				messages = []mail.Message{}
			}
			mail.SortMessagesNewest(messages)
			return "web_api", messages, nil
		}
		lastErr = err
	}
	return "", nil, fmt.Errorf("Web API 读取失败: %w", lastErr)
}

func (s *Server) listInbox(c *gin.Context) {
	accountID := c.Query("account_id")
	if accountID == "" {
		fail(c, http.StatusBadRequest, "参数缺失: account_id")
		return
	}
	alias := strings.TrimSpace(c.Query("alias"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	fetchLimit := page*limit + 1

	method, messages, err := s.readInbox(accountID, alias, fetchLimit, days, c.DefaultQuery("method", "auto"))
	if err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "不支持的邮件读取方式") || strings.Contains(err.Error(), "未配置") || strings.Contains(err.Error(), "需要指定") {
			status = http.StatusBadRequest
		}
		fail(c, status, err.Error())
		return
	}
	hasMore := len(messages) > page*limit
	start := (page - 1) * limit
	if start > len(messages) {
		start = len(messages)
	}
	end := start + limit
	if end > len(messages) {
		end = len(messages)
	}
	messages = messages[start:end]
	for i := range messages {
		if code := mail.ExtractVerificationCode(messages[i].Subject + "\n" + messages[i].Preview); code != "" {
			messages[i].Code = code
		}
		if messages[i].Preview == "" && messages[i].Code != "" {
			messages[i].Preview = "已识别验证码：" + messages[i].Code
		}
	}
	ok(c, gin.H{
		"account_id": accountID,
		"alias":      alias,
		"count":      len(messages),
		"page":       page,
		"page_size":  limit,
		"has_more":   hasMore,
		"messages":   messages,
		"method":     method,
	})
}

// inboxCount 只判断指定邮箱近 N 天的邮件数量，不等待邮件列表和摘要加载。
func (s *Server) inboxCount(c *gin.Context) {
	accountID := c.Query("account_id")
	alias := strings.TrimSpace(c.Query("alias"))
	if accountID == "" {
		fail(c, http.StatusBadRequest, "参数缺失: account_id")
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	if alias != "" {
		if mc, unlock, folders, forwardErr := s.mgr.AcquireForwardIMAP(accountID); forwardErr == nil {
			count, countErr := mc.CountForwardedByAlias(alias, folders, days)
			unlock.Unlock()
			if countErr == nil {
				ok(c, gin.H{"account_id": accountID, "alias": alias, "count": count, "method": "forward_imap"})
				return
			}
		}
	}
	mc, unlock, err := s.mgr.AcquireIMAP(accountID)
	if err != nil {
		fail(c, http.StatusBadRequest, "邮件计数需要 App Password (IMAP): "+err.Error())
		return
	}
	defer unlock.Unlock()
	count := 0
	if alias == "" {
		count, err = mc.InboxCount()
	} else {
		count, err = mc.CountByRecipient(alias, days)
	}
	if err != nil {
		fail(c, http.StatusBadGateway, "读取邮件数量失败: "+err.Error())
		return
	}
	ok(c, gin.H{"account_id": accountID, "alias": alias, "count": count})
}

// getMessage 读取单封邮件完整内容 (含正文)。
//
// 仅 IMAP 路径支持正文读取;Web API 路径的邮件列表只有摘要,
// 前端应在 method=web_api 时提示用户配置 App Password 以阅读正文。
func (s *Server) getMessage(c *gin.Context) {
	accountID := c.Query("account_id")
	uidStr := c.Query("uid")
	if accountID == "" || uidStr == "" {
		fail(c, http.StatusBadRequest, "参数缺失: account_id, uid")
		return
	}
	uid64, err := strconv.ParseUint(uidStr, 10, 32)
	if err != nil {
		fail(c, http.StatusBadRequest, "参数错误: uid 必须是数字")
		return
	}

	var mc *mail.Client
	var unlock interface{ Unlock() }
	source := c.Query("source")
	alias := strings.TrimSpace(c.Query("alias"))
	if source == "forward_imap" {
		if alias == "" {
			fail(c, http.StatusBadRequest, "转发邮箱正文读取需要指定隐藏邮箱地址")
			return
		}
		forwardClient, forwardUnlock, _, forwardErr := s.mgr.AcquireForwardIMAP(accountID)
		mc, unlock, err = forwardClient, forwardUnlock, forwardErr
	} else {
		imapClient, imapUnlock, imapErr := s.mgr.AcquireIMAP(accountID)
		mc, unlock, err = imapClient, imapUnlock, imapErr
	}
	if err != nil {
		fail(c, http.StatusBadRequest, "正文读取需要可用的 IMAP 配置: "+err.Error())
		return
	}
	defer unlock.Unlock()

	var full *mail.FullMessage
	if source == "forward_imap" {
		full, err = mc.GetForwardedFull(uint32(uid64), c.Query("folder"), alias)
	} else {
		full, err = mc.GetFull(uint32(uid64), c.Query("folder"))
	}
	if err != nil {
		fail(c, http.StatusBadGateway, "读取邮件正文失败: "+err.Error())
		return
	}
	ok(c, full)
}

func (s *Server) deleteMessage(c *gin.Context) {
	accountID := c.Query("account_id")
	uidStr := c.Query("uid")
	source := c.Query("source")
	if accountID == "" || uidStr == "" {
		fail(c, http.StatusBadRequest, "参数缺失: account_id, uid")
		return
	}
	if source == "web_api" {
		fail(c, http.StatusBadRequest, "Web API 模式不能删除邮件，请切换到 IMAP")
		return
	}
	uid64, err := strconv.ParseUint(uidStr, 10, 32)
	if err != nil {
		fail(c, http.StatusBadRequest, "参数错误: uid 必须是数字")
		return
	}
	var mc *mail.Client
	var unlock interface{ Unlock() }
	if source == "forward_imap" {
		if strings.TrimSpace(c.Query("alias")) == "" {
			fail(c, http.StatusBadRequest, "转发邮箱删除需要指定隐藏邮箱地址")
			return
		}
		mc, unlock, _, err = s.mgr.AcquireForwardIMAP(accountID)
	} else {
		mc, unlock, err = s.mgr.AcquireIMAP(accountID)
	}
	if err != nil {
		fail(c, http.StatusBadRequest, "删除邮件需要可用的 IMAP 配置: "+err.Error())
		return
	}
	defer unlock.Unlock()
	if source == "forward_imap" {
		if err := mc.VerifyForwardedAlias(uint32(uid64), c.Query("folder"), c.Query("alias")); err != nil {
			fail(c, http.StatusForbidden, err.Error())
			return
		}
	}
	if err := mc.DeleteMessage(uint32(uid64), c.Query("folder")); err != nil {
		fail(c, http.StatusBadGateway, "删除邮件失败: "+err.Error())
		return
	}
	ok(c, gin.H{"uid": uidStr, "folder": c.Query("folder")})
}

type batchDeleteMessagesReq struct {
	AccountID string                   `json:"account_id"`
	Source    string                   `json:"source"`
	Messages  []batchDeleteMessageItem `json:"messages"`
}

type batchDeleteMessageItem struct {
	UID    string `json:"uid"`
	Folder string `json:"folder"`
	Alias  string `json:"alias"`
}

// batchDeleteMessages performs one IMAP STORE+EXPUNGE per folder instead of
// issuing one complete IMAP transaction for every selected message.
func (s *Server) batchDeleteMessages(c *gin.Context) {
	var req batchDeleteMessagesReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.AccountID) == "" || len(req.Messages) == 0 {
		fail(c, http.StatusBadRequest, "参数缺失: account_id, messages")
		return
	}
	if len(req.Messages) > 200 {
		fail(c, http.StatusBadRequest, "一次最多删除 200 封邮件")
		return
	}
	if req.Source == "web_api" {
		fail(c, http.StatusBadRequest, "Web API 模式不能删除邮件，请切换到 IMAP")
		return
	}

	var mc *mail.Client
	var unlock interface{ Unlock() }
	var err error
	if req.Source == "forward_imap" {
		mc, unlock, _, err = s.mgr.AcquireForwardIMAP(req.AccountID)
	} else {
		mc, unlock, err = s.mgr.AcquireIMAP(req.AccountID)
	}
	if err != nil {
		fail(c, http.StatusBadRequest, "删除邮件需要可用的 IMAP 配置: "+err.Error())
		return
	}
	defer unlock.Unlock()

	byFolder := make(map[string][]uint32)
	seen := make(map[string]bool)
	for _, item := range req.Messages {
		uid64, parseErr := strconv.ParseUint(item.UID, 10, 32)
		if parseErr != nil {
			fail(c, http.StatusBadRequest, "参数错误: uid 必须是数字")
			return
		}
		folder := strings.TrimSpace(item.Folder)
		if folder == "" {
			folder = "INBOX"
		}
		key := folder + "\x00" + item.UID
		if seen[key] {
			continue
		}
		seen[key] = true
		if req.Source == "forward_imap" {
			alias := strings.TrimSpace(item.Alias)
			if alias == "" {
				fail(c, http.StatusBadRequest, "转发邮箱删除需要指定隐藏邮箱地址")
				return
			}
			if verifyErr := mc.VerifyForwardedAlias(uint32(uid64), folder, alias); verifyErr != nil {
				fail(c, http.StatusForbidden, verifyErr.Error())
				return
			}
		}
		byFolder[folder] = append(byFolder[folder], uint32(uid64))
	}

	deleted := 0
	for folder, uids := range byFolder {
		if deleteErr := mc.DeleteMessages(uids, folder); deleteErr != nil {
			fail(c, http.StatusBadGateway, fmt.Sprintf("批量删除邮件失败（已删除 %d 封）: %v", deleted, deleteErr))
			return
		}
		deleted += len(uids)
	}
	ok(c, gin.H{"requested": len(req.Messages), "deleted": deleted})
}
