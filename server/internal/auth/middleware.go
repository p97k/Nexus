package auth

import (
	"net/http"
	"strings"

	"nexus/internal/domain"

	"github.com/gin-gonic/gin"
)

const (
	ContextUserID = "user_id"
	ContextRole   = "user_role"
)

func Middleware(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Allow token via Authorization header or ?token= query param (for SSE EventSource)
		var token string
		header := c.GetHeader("Authorization")
		if strings.HasPrefix(header, "Bearer ") {
			token = strings.TrimPrefix(header, "Bearer ")
		} else if q := c.Query("token"); q != "" {
			token = q
		}
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}
		claims, err := svc.ParseToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set(ContextUserID, claims.UserID)
		c.Set(ContextRole, string(claims.Role))
		c.Next()
	}
}

func RequireRole(roles ...domain.Role) gin.HandlerFunc {
	allowed := make(map[domain.Role]bool)
	for _, r := range roles {
		allowed[r] = true
	}
	return func(c *gin.Context) {
		role := domain.Role(c.GetString(ContextRole))
		if !allowed[role] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}
		c.Next()
	}
}
