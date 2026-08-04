// handlers_alias.go - HME 别名管理接口。
package server

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ====================================================================
// 创建别名 (单个)
//   POST /api/create  body: {"account_id": "acc_xxx", "label": "可选标签"}
// ====================================================================

type createReq struct {
	AccountID string `json:"account_id" binding:"required"`
	Label     string `json:"label"`
}

func (s *Server) createAlias(c *gin.Context) {
	var req createReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: account_id 必填 — "+err.Error())
		return
	}

	client, err := s.mgr.HMEClient(req.AccountID, false)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}

	result, err := client.CreateAlias(req.Label, 5)

	// 操作完成后,保存可能已刷新的 Cookie（validate 会轮换 token）
	_ = s.mgr.SaveCookies(req.AccountID, client.Cookies)

	if err != nil {
		msg := err.Error()
		if isSessionError(msg) {
			fail(c, http.StatusUnauthorized, "iCloud 会话失效,请更新 Cookie: "+msg)
		} else {
			fail(c, http.StatusBadGateway, "创建邮箱失败: "+msg)
		}
		return
	}

	ok(c, gin.H{
		"email":      result.Email,
		"label":      result.Label,
		"created_at": result.CreatedAt,
		"account_id": req.AccountID,
	})
}

// ====================================================================
// 批量创建别名
//   POST /api/accounts/:id/aliases/batch
//   body: {"count": 5, "label_prefix": "注册"}  (count ≤ 50)
//
//   逐个创建,单个失败不中断;返回每个别名的成功/失败明细。
// ====================================================================

// maxBatchCount 单次批量创建上限,避免触发 iCloud 风控。
const maxBatchCount = 50

type batchCreateReq struct {
	Count       int    `json:"count" binding:"required,min=1"`
	LabelPrefix string `json:"label_prefix"`
}

type batchResult struct {
	Index   int    `json:"index"`
	Success bool   `json:"success"`
	Email   string `json:"email,omitempty"`
	Label   string `json:"label"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) batchCreateAliases(c *gin.Context) {
	id := c.Param("id")
	var req batchCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: count 必填且 ≥ 1 — "+err.Error())
		return
	}
	if req.Count > maxBatchCount {
		fail(c, http.StatusBadRequest, "单次最多创建 50 个别名")
		return
	}

	client, err := s.mgr.HMEClient(id, false)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}

	results := make([]batchResult, 0, req.Count)
	succeeded := 0
	for i := 0; i < req.Count; i++ {
		label := req.LabelPrefix
		if label != "" {
			label = label + " " + strconv.Itoa(i+1)
		}

		res, err := client.CreateAlias(label, 5)
		if err != nil {
			results = append(results, batchResult{Index: i + 1, Label: label, Error: err.Error()})
			// 会话失效时没有继续的必要,直接中断
			if isSessionError(err.Error()) {
				break
			}
			continue
		}
		succeeded++
		results = append(results, batchResult{Index: i + 1, Success: true, Email: res.Email, Label: res.Label})
	}

	_ = s.mgr.SaveCookies(id, client.Cookies)

	ok(c, gin.H{
		"account_id": id,
		"requested":  req.Count,
		"succeeded":  succeeded,
		"failed":     len(results) - succeeded,
		"interrupted": len(results) < req.Count,
		"results":    results,
	})
}

// ====================================================================
// 别名列表与状态管理
// ====================================================================

// setForwardTo 修改 HME 转发目标邮箱 (账号级,影响全部别名)。
//   POST /api/accounts/:id/forward-to  body: {"email": "muskzhou@icloud.com"}
func (s *Server) setForwardTo(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Email string `json:"email" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: email 必填")
		return
	}

	client, err := s.mgr.HMEClient(id, false)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	if err := client.UpdateForwardTo(req.Email); err != nil {
		_ = s.mgr.SaveCookies(id, client.Cookies)
		fail(c, http.StatusBadGateway, "修改转发地址失败: "+err.Error())
		return
	}
	_ = s.mgr.SaveCookies(id, client.Cookies)
	ok(c, gin.H{"forward_to": req.Email})
}

func (s *Server) listAliases(c *gin.Context) {
	accountID := c.Query("account_id")
	if accountID == "" {
		fail(c, http.StatusBadRequest, "参数缺失: account_id")
		return
	}
	client, err := s.mgr.HMEClient(accountID, false)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	aliases, err := client.ListAliases()
	_ = s.mgr.SaveCookies(accountID, client.Cookies)
	if err != nil {
		if isSessionError(err.Error()) {
			fail(c, http.StatusUnauthorized, "iCloud 会话失效,请更新 Cookie: "+err.Error())
		} else {
			fail(c, http.StatusBadGateway, err.Error())
		}
		return
	}
	ok(c, gin.H{
		"account_id": accountID,
		"count":      len(aliases),
		"aliases":    aliases,
	})
}

type aliasActionReq struct {
	AccountID string `json:"account_id" binding:"required"`
}

func (s *Server) deactivateAlias(c *gin.Context) {
	s.aliasAction(c, "停用", func(client aliasOperator, anonymousID string) (bool, error) {
		return client.DeactivateHME(anonymousID)
	})
}

func (s *Server) reactivateAlias(c *gin.Context) {
	s.aliasAction(c, "激活", func(client aliasOperator, anonymousID string) (bool, error) {
		return client.ReactivateHME(anonymousID)
	})
}

// aliasOperator 抽象 HME 客户端的别名操作(便于复用与测试)。
type aliasOperator interface {
	DeactivateHME(anonymousID string) (bool, error)
	ReactivateHME(anonymousID string) (bool, error)
}

func (s *Server) aliasAction(c *gin.Context, actionName string, op func(aliasOperator, string) (bool, error)) {
	anonymousID := c.Param("id")
	var req aliasActionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: account_id 必填 — "+err.Error())
		return
	}

	client, err := s.mgr.HMEClient(req.AccountID, false)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}

	success, err := op(client, anonymousID)
	_ = s.mgr.SaveCookies(req.AccountID, client.Cookies)
	if err != nil {
		fail(c, http.StatusBadGateway, actionName+"失败: "+err.Error())
		return
	}
	ok(c, gin.H{"anonymous_id": anonymousID, "success": success})
}

func (s *Server) deleteAlias(c *gin.Context) {
	anonymousID := c.Param("id")
	var req aliasActionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: account_id 必填 — "+err.Error())
		return
	}

	client, err := s.mgr.HMEClient(req.AccountID, false)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}

	err = client.Delete(anonymousID)
	_ = s.mgr.SaveCookies(req.AccountID, client.Cookies)
	if err != nil {
		fail(c, http.StatusBadGateway, "删除失败: "+err.Error())
		return
	}
	ok(c, gin.H{"anonymous_id": anonymousID})
}
