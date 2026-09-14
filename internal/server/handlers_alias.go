// handlers_alias.go - HME 别名管理接口。
package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"icloud_distribution/internal/account"
)

// ====================================================================
// 创建别名 (单个)
//   POST /api/create  body: {"account_id": "acc_xxx", "label": "可选标签"}
// ====================================================================

type createReq struct {
	AccountID string `json:"account_id" binding:"required"`
	Label     string `json:"label"`
	GroupID   string `json:"group_id"`
	Note      string `json:"note"`
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
	// 创建成功后立即把本地整理信息绑定到真实的 anonymousId，避免新别名
	// 只能在刷新后手动归类。iCloud 不保存这些字段，它们只存在本项目。
	var anonymousID string
	if req.GroupID != "" || req.Note != "" {
		if aliases, listErr := client.ListAliases(); listErr == nil {
			for _, alias := range aliases {
				if alias.Email == result.Email {
					anonymousID = alias.AnonymousID
					break
				}
			}
		}
		if anonymousID != "" {
			if _, metaErr := s.mgr.UpdateAliasMetadata(req.AccountID, account.AliasMetadata{AliasID: anonymousID, Email: result.Email, GroupID: req.GroupID, Note: req.Note}); metaErr != nil {
				fail(c, http.StatusInternalServerError, "邮箱已创建，但保存分组失败: "+metaErr.Error())
				return
			}
		}
	}

	ok(c, gin.H{
		"email":        result.Email,
		"anonymous_id": anonymousID,
		"label":        result.Label,
		"created_at":   result.CreatedAt,
		"account_id":   req.AccountID,
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
	NamingRule  string `json:"naming_rule"`
	Separator   string `json:"separator"`
	StartNumber int    `json:"start_number"`
	Padding     int    `json:"padding"`
	GroupID     string `json:"group_id"`
	Note        string `json:"note"`
}

type batchResult struct {
	Index         int    `json:"index"`
	Success       bool   `json:"success"`
	Email         string `json:"email,omitempty"`
	Label         string `json:"label"`
	Error         string `json:"error,omitempty"`
	AnonymousID   string `json:"anonymous_id,omitempty"`
	MetadataError string `json:"metadata_error,omitempty"`
}

func batchLabel(req batchCreateReq, index int) (string, error) {
	prefix := strings.TrimSpace(req.LabelPrefix)
	if prefix == "" {
		return "", fmt.Errorf("名称前缀不能为空")
	}
	rule := req.NamingRule
	if rule == "" {
		rule = "sequence"
	}
	if rule == "same" {
		if len(prefix) > 200 {
			return "", fmt.Errorf("名称不能超过 200 个字符")
		}
		return prefix, nil
	}
	if rule != "sequence" {
		return "", fmt.Errorf("不支持的命名规则")
	}
	if req.Separator != "" && req.Separator != "-" && req.Separator != "_" && req.Separator != " " {
		return "", fmt.Errorf("不支持的名称分隔符")
	}
	start := req.StartNumber
	if start < 1 {
		start = 1
	}
	padding := req.Padding
	if padding == 0 {
		padding = 3
	}
	if padding < 1 || padding > 8 {
		return "", fmt.Errorf("编号位数必须在 1 到 8 之间")
	}
	label := fmt.Sprintf("%s%s%0*d", prefix, req.Separator, padding, start+index)
	if len(label) > 200 {
		return "", fmt.Errorf("生成的名称不能超过 200 个字符")
	}
	return label, nil
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
	for i := 0; i < req.Count; i++ {
		if _, err := batchLabel(req, i); err != nil {
			fail(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	if req.GroupID != "" {
		groups, _, err := s.mgr.Organizer(id)
		if err != nil {
			fail(c, http.StatusNotFound, err.Error())
			return
		}
		found := false
		for _, group := range groups {
			if group.ID == req.GroupID {
				found = true
				break
			}
		}
		if !found {
			fail(c, http.StatusBadRequest, "分组不存在")
			return
		}
	}

	client, err := s.mgr.HMEClient(id, false)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}

	results := make([]batchResult, 0, req.Count)
	succeeded := 0
	for i := 0; i < req.Count; i++ {
		label, _ := batchLabel(req, i)

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
	metadataFailed := 0
	if succeeded > 0 && (req.GroupID != "" || strings.TrimSpace(req.Note) != "") {
		if aliases, listErr := client.ListAliases(); listErr == nil {
			byEmail := make(map[string]account.AliasMetadata, len(aliases))
			for _, alias := range aliases {
				byEmail[strings.ToLower(alias.Email)] = account.AliasMetadata{AliasID: alias.AnonymousID, Email: alias.Email, Label: alias.Label, GroupID: req.GroupID, Note: req.Note}
			}
			metas := make([]account.AliasMetadata, 0, succeeded)
			resultIndexes := make([]int, 0, succeeded)
			for i := range results {
				if !results[i].Success {
					continue
				}
				meta, found := byEmail[strings.ToLower(results[i].Email)]
				if !found || meta.AliasID == "" {
					results[i].MetadataError = "创建成功，但未能匹配 iCloud 别名 ID"
					metadataFailed++
					continue
				}
				results[i].AnonymousID = meta.AliasID
				metas = append(metas, meta)
				resultIndexes = append(resultIndexes, i)
			}
			if _, metaErr := s.mgr.UpdateAliasMetadataBatch(id, metas); metaErr != nil {
				for _, index := range resultIndexes {
					results[index].MetadataError = metaErr.Error()
					metadataFailed++
				}
			}
		} else {
			for i := range results {
				if results[i].Success {
					results[i].MetadataError = "创建成功，但保存分组和备注失败: " + listErr.Error()
					metadataFailed++
				}
			}
		}
	}

	_ = s.mgr.SaveCookies(id, client.Cookies)

	ok(c, gin.H{
		"account_id":      id,
		"requested":       req.Count,
		"succeeded":       succeeded,
		"failed":          len(results) - succeeded,
		"interrupted":     len(results) < req.Count,
		"metadata_failed": metadataFailed,
		"results":         results,
	})
}

// ====================================================================
// 别名列表与状态管理
// ====================================================================

// setForwardTo 修改 HME 转发目标邮箱 (账号级,影响全部别名)。
//
//	POST /api/accounts/:id/forward-to  body: {"email": "muskzhou@icloud.com"}
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

type batchDeleteAliasesReq struct {
	AccountID string   `json:"account_id" binding:"required"`
	IDs       []string `json:"ids" binding:"required"`
}

// batchDeleteAliases 批量删除隐藏邮箱。HME 客户端会在需要时先停用再删除，
// 单个失败不会中断其余项，并把每项结果返回给界面。
func (s *Server) batchDeleteAliases(c *gin.Context) {
	var req batchDeleteAliasesReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		fail(c, http.StatusBadRequest, "参数错误: account_id 和 ids 必填")
		return
	}
	client, err := s.mgr.HMEClient(req.AccountID, false)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	type result struct {
		ID      string `json:"id"`
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	results := make([]result, 0, len(req.IDs))
	deleted := 0
	seen := map[string]bool{}
	for _, rawID := range req.IDs {
		id := strings.TrimSpace(rawID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		item := result{ID: id}
		if deleteErr := client.Delete(id); deleteErr != nil {
			item.Error = deleteErr.Error()
		} else {
			item.Success = true
			deleted++
		}
		results = append(results, item)
	}
	_ = s.mgr.SaveCookies(req.AccountID, client.Cookies)
	ok(c, gin.H{"requested": len(results), "deleted": deleted, "failed": len(results) - deleted, "results": results})
}
