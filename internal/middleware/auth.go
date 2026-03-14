package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/simp-lee/ginx"

	"github.com/simp-lee/gobase/internal/pkg"
)

// RequireAdmin is a middleware that checks the user has role "admin".
// If the role is not "admin" (including missing role), it returns 403.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isAdmin(c) {
			c.AbortWithStatusJSON(http.StatusForbidden, pkg.Response{
				Code:    http.StatusForbidden,
				Message: "admin access required",
			})
			return
		}
		c.Next()
	}
}

// isAdmin returns true if the user has the "admin" role.
// It checks ginx roles, the "roles" context key ([]string),
// and the "role" context key (string), in that order.
func isAdmin(c *gin.Context) bool {
	// Check ginx context (set by JWT auth middleware)
	if roles, ok := ginx.GetUserRoles(c); ok {
		for _, role := range roles {
			if strings.EqualFold(strings.TrimSpace(role), "admin") {
				return true
			}
		}
		return false
	}

	// Check "roles" context key ([]string)
	if rolesRaw, ok := c.Get("roles"); ok {
		if roles, ok := rolesRaw.([]string); ok {
			for _, role := range roles {
				if strings.EqualFold(strings.TrimSpace(role), "admin") {
					return true
				}
			}
			return false
		}
	}

	// Check "role" context key (string)
	role := strings.TrimSpace(c.GetString("role"))
	return strings.EqualFold(role, "admin")
}
