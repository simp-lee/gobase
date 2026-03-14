package user

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/simp-lee/ginx"
)

type adminFieldAuthzKey struct{}

func withAdminFieldAuthorized(ctx context.Context, allowed bool) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, adminFieldAuthzKey{}, allowed)
}

func isAdminFieldAuthorized(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	allowed, ok := ctx.Value(adminFieldAuthzKey{}).(bool)
	return ok && allowed
}

func isRequesterAdmin(c *gin.Context) bool {
	if roles, ok := ginx.GetUserRoles(c); ok {
		for _, role := range roles {
			if strings.EqualFold(strings.TrimSpace(role), "admin") {
				return true
			}
		}
		return false
	}

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

	return strings.EqualFold(strings.TrimSpace(c.GetString("role")), "admin")
}
