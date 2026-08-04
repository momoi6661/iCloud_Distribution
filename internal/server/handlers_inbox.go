// handlers_inbox.go - 邮件读取接口。
//
//   GET /api/inbox?account_id=acc_xxx[&alias=xxx@icloud.com][&limit=20][&days=7]
//   GET /api/inbox/message?account_id=acc_xxx&uid=1042   (仅 IMAP 路径支持正文)
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

// readInbox 按认证优先级读取邮件: IMAP (App Password) 优先,Web API (Cookie) 回退。
// 返回读取方式 (imap/web_api) 与邮件列表;无可用客户端或全部失败时返回错误。
// 供 /api/inbox 与公开分享端点共用。
func (s *Server) readInbox(accountID, alias string, limit, days int) (string, []mail.Message, error) {
	// 优先 IMAP (连接池复用,免每次重新登录)
	if mc, unlock, err := s.mgr.AcquireIMAP(accountID); err == nil {
		var messages []mail.Message
		var ferr error
		if alias != "" {
			messages, ferr = mc.FindByRecipient(alias, limit, days)
		} else {
			messages, ferr = mc.ListInbox(limit, days)
		}
		unlock.Unlock()
		if ferr == nil {
			return "imap", messages, nil
		}
		// IMAP 失败,继续尝试 Web API
	}

	// 回退 Web API (mccgateway 端点偶发不稳定,失败重试一次)
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		wmc, err := s.mgr.WebMailClient(accountID)
		if err != nil {
			return "", nil, fmt.Errorf("无可用邮件客户端: 需要 App Password 或 Cookie")
		}

		var messages []mail.Message
		if alias != "" {
			messages, err = wmc.FindByAlias(alias, limit)
		} else {
			messages, err = wmc.ListInbox(limit)
		}
		if err == nil {
			// 回填网关缓存,后续请求免重新 validate
			s.mgr.CacheGateway(accountID, wmc.GatewayURL())
			return "web_api", messages, nil
		}
		lastErr = err
	}
	return "", nil, fmt.Errorf("读取邮件失败: %w", lastErr)
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

	method, messages, err := s.readInbox(accountID, alias, limit, days)
	if err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "无可用邮件客户端") {
			status = http.StatusBadRequest
		}
		fail(c, status, err.Error())
		return
	}
	ok(c, gin.H{
		"account_id": accountID,
		"alias":      alias,
		"count":      len(messages),
		"messages":   messages,
		"method":     method,
	})
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

	mc, unlock, err := s.mgr.AcquireIMAP(accountID)
	if err != nil {
		fail(c, http.StatusBadRequest, "正文读取需要 App Password (IMAP): "+err.Error())
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
