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

	if alias != "" {
		if method, messages, err := s.readForwardInbox(accountID, alias, limit, days); err == nil {
			return method, messages, nil
		}
	}
	if method, messages, err := s.readICloudIMAPInbox(accountID, alias, limit, days); err == nil {
		return method, messages, nil
	}
	return s.readWebInbox(accountID, alias, limit)
}

func (s *Server) readForwardInbox(accountID, alias string, limit, days int) (string, []mail.Message, error) {
	if alias == "" {
		return "", nil, fmt.Errorf("转发邮箱 IMAP 需要指定隐藏邮箱地址")
	}
	mc, unlock, folders, err := s.mgr.AcquireForwardIMAP(accountID)
	if err != nil {
		return "", nil, err
	}
	messages, readErr := mc.ListForwardedByAlias(alias, folders, limit, days)
	unlock.Unlock()
	if readErr != nil {
		return "", nil, readErr
	}
	if messages == nil {
		messages = []mail.Message{}
	}
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

	method, messages, err := s.readInbox(accountID, alias, limit, days, c.DefaultQuery("method", "auto"))
	if err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "不支持的邮件读取方式") || strings.Contains(err.Error(), "未配置") || strings.Contains(err.Error(), "需要指定") {
			status = http.StatusBadRequest
		}
		fail(c, status, err.Error())
		return
	}
	for i := range messages {
		messages[i].Code = mail.ExtractVerificationCode(messages[i].Subject + "\n" + messages[i].Preview)
		if messages[i].Preview == "" && messages[i].Code != "" {
			messages[i].Preview = "已识别验证码：" + messages[i].Code
		}
	}
	ok(c, gin.H{
		"account_id": accountID,
		"alias":      alias,
		"count":      len(messages),
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
	if c.Query("source") == "forward_imap" {
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

	full, err := mc.GetFull(uint32(uid64), c.Query("folder"))
	if err != nil {
		fail(c, http.StatusBadGateway, "读取邮件正文失败: "+err.Error())
		return
	}
	ok(c, full)
}
