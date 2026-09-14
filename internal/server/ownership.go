package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *Server) ownershipMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		identity := currentIdentity(c)
		accountIDs := []string{}
		if id := strings.TrimSpace(c.Query("account_id")); id != "" {
			accountIDs = append(accountIDs, id)
		}
		parts := strings.Split(strings.Trim(c.Request.URL.Path, "/"), "/")
		if len(parts) >= 3 && parts[0] == "api" && parts[1] == "accounts" && parts[2] != "batch" {
			accountIDs = append(accountIDs, parts[2])
		}
		if c.Request.Body != nil && c.Request.Method != "GET" {
			raw, _ := io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewReader(raw))
			var body struct {
				AccountID string   `json:"account_id"`
				IDs       []string `json:"ids"`
			}
			if json.Unmarshal(raw, &body) == nil {
				if body.AccountID != "" {
					accountIDs = append(accountIDs, body.AccountID)
				}
				if len(parts) >= 3 && parts[1] == "accounts" && parts[2] == "batch" {
					accountIDs = append(accountIDs, body.IDs...)
				}
			}
		}
		for _, id := range accountIDs {
			owner, ok := s.mgr.Owner(id)
			// Leave unknown IDs to the handler so normal parameter/not-found
			// validation remains consistent. Existing accounts, however, must
			// always belong to the current user.
			if ok && owner != identity.ID {
				fail(c, http.StatusNotFound, "账号不存在")
				c.Abort()
				return
			}
		}
		c.Next()
	}
}
