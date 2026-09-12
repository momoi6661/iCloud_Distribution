// handlers_account.go - 账号管理接口。
package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"icloud_distribution/internal/account"
)

func (s *Server) listAccounts(c *gin.Context) {
	ok(c, s.mgr.ListAccounts())
}

func (s *Server) listDisabledAccounts(c *gin.Context) {
	ok(c, s.mgr.ListDisabledAccounts())
}

func (s *Server) getOrganizer(c *gin.Context) {
	groups, metadata, err := s.mgr.Organizer(c.Param("id"))
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, gin.H{"groups": groups, "metadata": metadata})
}

type organizerGroupReq struct {
	Name string `json:"name"`
}

func (s *Server) createOrganizerGroup(c *gin.Context) {
	var req organizerGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: name 必填 — "+err.Error())
		return
	}
	group, err := s.mgr.CreateGroup(c.Param("id"), req.Name)
	if err != nil {
		organizerFail(c, err)
		return
	}
	c.JSON(http.StatusCreated, apiResp{Success: true, Data: group})
}

func (s *Server) updateOrganizerGroup(c *gin.Context) {
	var req organizerGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: name 必填 — "+err.Error())
		return
	}
	if err := s.mgr.UpdateGroup(c.Param("id"), c.Param("group_id"), req.Name); err != nil {
		organizerFail(c, err)
		return
	}
	ok(c, gin.H{"id": c.Param("group_id"), "name": req.Name})
}

func (s *Server) deleteOrganizerGroup(c *gin.Context) {
	if err := s.mgr.DeleteGroup(c.Param("id"), c.Param("group_id")); err != nil {
		organizerFail(c, err)
		return
	}
	ok(c, gin.H{"id": c.Param("group_id")})
}

type aliasMetadataReq struct {
	AliasID string `json:"alias_id"`
	Email   string `json:"email"`
	GroupID string `json:"group_id"`
	Note    string `json:"note"`
}

func (s *Server) updateAliasMetadata(c *gin.Context) {
	var req aliasMetadataReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: alias_id 必填 — "+err.Error())
		return
	}
	meta, err := s.mgr.UpdateAliasMetadata(c.Param("id"), account.AliasMetadata{
		AliasID: req.AliasID, Email: req.Email, GroupID: req.GroupID, Note: req.Note,
	})
	if err != nil {
		organizerFail(c, err)
		return
	}
	ok(c, meta)
}

func organizerFail(c *gin.Context, err error) {
	if strings.Contains(err.Error(), "不存在") {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	fail(c, http.StatusBadRequest, err.Error())
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
	acc.HasCookies = len(acc.Cookies) > 0
	acc.HasAppPassword = strings.TrimSpace(acc.AppPassword) != ""
	acc.Cookies = nil
	acc.AppPassword = ""
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

func (s *Server) deactivateAccount(c *gin.Context) {
	id := c.Param("id")
	if err := s.mgr.DeactivateAccount(id); err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, gin.H{"id": id, "status": "disabled"})
}

func (s *Server) restoreAccount(c *gin.Context) {
	id := c.Param("id")
	if err := s.mgr.RestoreAccount(id); err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	acc, _ := s.mgr.GetAccount(id)
	ok(c, gin.H{"id": id, "status": acc.Status})
}

type batchAccountReq struct {
	IDs []string `json:"ids" binding:"required"`
}

func (s *Server) batchDeactivateAccounts(c *gin.Context) {
	var req batchAccountReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: ids 必填 — "+err.Error())
		return
	}
	changed, alreadyDisabled, notFound, err := s.mgr.BatchDeactivate(req.IDs)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"requested": len(req.IDs), "changed": changed, "already_disabled": alreadyDisabled, "not_found": notFound})
}

func (s *Server) batchRemoveAccounts(c *gin.Context) {
	var req batchAccountReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: ids 必填 — "+err.Error())
		return
	}
	deleted, notFound, err := s.mgr.BatchRemove(req.IDs)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"requested": len(req.IDs), "deleted": deleted, "not_found": notFound})
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
