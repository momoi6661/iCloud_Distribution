package server

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"icloud_distribution/internal/auth"
)

func currentIdentity(c *gin.Context) auth.Identity {
	value, _ := c.Get("ui_identity")
	identity, _ := value.(auth.Identity)
	return identity
}

func (s *Server) superadminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !currentIdentity(c).IsSuperadmin() {
			fail(c, http.StatusForbidden, "只有超级管理员可以管理用户")
			c.Abort()
			return
		}
		c.Next()
	}
}

func (s *Server) listUsers(c *gin.Context) {
	users, err := s.ui.Store().List()
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"users": users})
}

func (s *Server) createUser(c *gin.Context) {
	var req struct {
		Username   string `json:"username" binding:"required"`
		Password   string `json:"password" binding:"required"`
		MustChange *bool  `json:"must_change_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请输入用户名和初始密码")
		return
	}
	must := true
	if req.MustChange != nil {
		must = *req.MustChange
	}
	user, err := s.ui.Store().Create(req.Username, req.Password, must)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	c.JSON(http.StatusCreated, apiResp{Success: true, Data: user})
}

func (s *Server) resetUserPassword(c *gin.Context) {
	var req struct {
		Password   string `json:"password" binding:"required"`
		MustChange *bool  `json:"must_change_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请输入新密码")
		return
	}
	must := true
	if req.MustChange != nil {
		must = *req.MustChange
	}
	if err := s.ui.Store().SetPassword(c.Param("id"), req.Password, must); err != nil {
		userStoreError(c, err)
		return
	}
	ok(c, gin.H{"message": "密码已重置，原有会话已失效"})
}

func (s *Server) changeOwnPassword(c *gin.Context) {
	identity := currentIdentity(c)
	if identity.IsSuperadmin() {
		fail(c, http.StatusBadRequest, "超级管理员密码请修改服务器环境变量")
		return
	}
	var req struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		NewPassword     string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请输入当前密码和新密码")
		return
	}
	if err := s.ui.Store().ChangeOwnPassword(identity.ID, req.CurrentPassword, req.NewPassword); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	s.ui.ClearCookie(c.Writer, c.Request)
	ok(c, gin.H{"message": "密码已修改，请重新登录"})
}

func (s *Server) setUserStatus(c *gin.Context) {
	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "状态不能为空")
		return
	}
	if err := s.ui.Store().SetStatus(c.Param("id"), req.Status); err != nil {
		userStoreError(c, err)
		return
	}
	ok(c, gin.H{"status": req.Status})
}

func (s *Server) deleteUser(c *gin.Context) {
	if err := s.ui.Store().Delete(c.Param("id")); err != nil {
		userStoreError(c, err)
		return
	}
	ok(c, gin.H{"id": c.Param("id")})
}

func userStoreError(c *gin.Context, err error) {
	if err == sql.ErrNoRows {
		fail(c, http.StatusNotFound, "用户不存在")
		return
	}
	fail(c, http.StatusBadRequest, err.Error())
}
