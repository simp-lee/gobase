package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/simp-lee/ginx"
	"github.com/simp-lee/jwt"
	"github.com/simp-lee/logger"
	"github.com/simp-lee/rbac"
	"gorm.io/gorm"

	"github.com/simp-lee/gobase/internal/config"
	"github.com/simp-lee/gobase/internal/domain"
	"github.com/simp-lee/gobase/internal/middleware"
	"github.com/simp-lee/gobase/internal/module/auth"
)

const authHeaderPromotedFromCookieKey = "_auth_header_promoted_from_cookie"

// setupAuth conditionally creates JWT, optional RBAC services, the auth
// module, and registers the auth middleware on the given chain. When auth
// is disabled, all returned values are nil.
func setupAuth(cfg *config.Config, db *gorm.DB, repo domain.UserRepository, chain *ginx.Chain, log *logger.Logger, csrfSecret string) (authModules []Module, jwtSvc jwt.Service, rbacSvc rbac.Service, err error) {
	if !cfg.Auth.Enabled {
		return nil, nil, nil, nil
	}

	// Parse token expiry duration.
	tokenExpiry, err := time.ParseDuration(cfg.Auth.TokenExpiry)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse auth.token_expiry %q: %w", cfg.Auth.TokenExpiry, err)
	}

	// Create jwt.Service.
	jwtSvc, err = jwt.New(cfg.Auth.JWTSecret)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create jwt service: %w", err)
	}
	// Keep a local reference for cleanup in case a later step fails.
	jwtToClose := jwtSvc
	defer func() {
		if err != nil && jwtToClose != nil {
			jwtToClose.Close()
		}
	}()

	// Optional: create rbac.Service.
	if cfg.Auth.RBAC.Enabled {
		sqlDB, sqlErr := db.DB()
		if sqlErr != nil {
			return nil, nil, nil, fmt.Errorf("get sql.DB for rbac: %w", sqlErr)
		}
		// safe: durations validated by config.Validate() before reaching here.
		roleTTL, _ := time.ParseDuration(cfg.Auth.RBAC.Cache.RoleTTL)
		userRoleTTL, _ := time.ParseDuration(cfg.Auth.RBAC.Cache.UserRoleTTL)
		permissionTTL, _ := time.ParseDuration(cfg.Auth.RBAC.Cache.PermissionTTL)

		rbacSvc, err = rbac.New(rbac.WithCachedStorage(sqlDB, &rbac.CacheConfig{
			RoleTTL:      roleTTL,
			UserRoleTTL:  userRoleTTL,
			PermTTL:      permissionTTL,
			MaxRoles:     cfg.Auth.RBAC.Cache.MaxRoleEntries,
			MaxUserRoles: cfg.Auth.RBAC.Cache.MaxUserEntries,
			MaxUserPerms: cfg.Auth.RBAC.Cache.MaxPermissionEntries,
		}))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("create rbac service: %w", err)
		}
		// Keep a local reference for cleanup in case a later step fails.
		rbacToClose := rbacSvc
		defer func() {
			if err != nil && rbacToClose != nil {
				if closeErr := rbacToClose.Close(); closeErr != nil {
					slog.Error("rbac service close error during init rollback", slog.Any("error", closeErr))
				}
			}
		}()
		log.Info("RBAC service initialized")
	}

	// Create auth module.
	authSvc := auth.NewService(jwtSvc, repo, tokenExpiry)
	cookieSecure := false
	if cfg.Auth.CookieSecure != nil {
		cookieSecure = *cfg.Auth.CookieSecure
	}
	authHandler := auth.NewHandlerWithCookieSecure(authSvc, cookieSecure)
	authModule := auth.NewModule(authHandler)

	// Add Auth middleware (exclude public paths).
	// RBAC permission checks are already wired for users routes below.
	// Extend the same pattern to additional resource route groups as needed.
	// See: ginx.RequirePermission, ginx.RequireRolePermission
	protectedPagePaths := ginx.PathHasPrefix("/users")

	// (1) Cookie→Bearer promotion for all non-API routes (before ginx.Auth).
	chain.When(
		ginx.Not(ginx.PathHasPrefix("/api")),
		optionalBearerAuthorizationHeaderMiddleware(),
	)

	// (2a) Keep existing API auth rule.
	chain.When(
		ginx.And(
			ginx.PathHasPrefix("/api"),
			ginx.Not(ginx.PathIs(cfg.Auth.PublicPaths...)),
		),
		ginx.Auth(jwtSvc),
	)

	// (2b) Enforce explicit bearer auth on protected page write routes.
	chain.When(
		ginx.And(
			protectedPagePaths,
			ginx.MethodIs(http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete),
		),
		requireBearerAuthorizationHeaderMiddleware(csrfSecret),
	)

	// (2c) Add auth for non-API non-public page routes.
	chain.When(
		protectedPagePaths,
		ginx.Auth(jwtSvc),
	)

	// (3) Bridge auth context for non-API non-public page routes.
	chain.When(
		protectedPagePaths,
		bridgeAuthContextMiddleware(),
	)

	// (4) Optional auth context for public page routes (at least "/").
	chain.When(
		ginx.And(
			ginx.Not(ginx.PathHasPrefix("/api")),
			ginx.PathIs("/"),
		),
		optionalAuthContextMiddleware(jwtSvc),
	)

	if cfg.Auth.RBAC.Enabled {
		usersPath := ginx.PathHasPrefix("/api/v1/users")

		chain.When(
			ginx.And(usersPath, ginx.MethodIs(http.MethodGet)),
			ginx.RequirePermission(rbacSvc, "users", "read"),
		)
		chain.When(
			ginx.And(usersPath, ginx.MethodIs(http.MethodPost)),
			ginx.RequirePermission(rbacSvc, "users", "create"),
		)
		chain.When(
			ginx.And(usersPath, ginx.MethodIs(http.MethodPut)),
			ginx.RequirePermission(rbacSvc, "users", "update"),
		)
		chain.When(
			ginx.And(usersPath, ginx.MethodIs(http.MethodDelete)),
			ginx.RequirePermission(rbacSvc, "users", "delete"),
		)
	}

	return []Module{authModule}, jwtSvc, rbacSvc, nil
}

// resolveCSRFSecret returns the configured CSRF secret or generates a random
// one for non-release modes when a placeholder is configured.
// Release-mode validation (placeholder, length, complexity) is handled by
// config.Validate(), so only the non-release fallback remains here.
func resolveCSRFSecret(cfg *config.Config, log *logger.Logger) (string, error) {
	secret := cfg.Server.CSRFSecret
	if config.IsPlaceholderCSRFSecret(secret) {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return "", fmt.Errorf("generate csrf secret: %w", err)
		}
		secret = hex.EncodeToString(b)
		log.Warn("no csrf_secret configured, using random secret in non-release mode (will change on restart)")
	}
	return secret, nil
}

func promoteTokenFromCookie(c *gin.Context) (promoted bool) {
	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	if authorization != "" {
		parts := strings.Fields(authorization)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") && parts[1] != "" {
			return false
		}
	}

	token, err := c.Cookie(auth.AccessTokenCookieName)
	if err != nil || token == "" {
		return false
	}

	c.Request.Header.Set("Authorization", "Bearer "+token)
	c.Set(authHeaderPromotedFromCookieKey, true)
	return true
}

func wasAuthHeaderPromotedFromCookie(c *gin.Context) bool {
	value, ok := c.Get(authHeaderPromotedFromCookieKey)
	if !ok {
		return false
	}
	promoted, ok := value.(bool)
	return ok && promoted
}

func optionalBearerAuthorizationHeader() gin.HandlerFunc {
	return middlewareToHandler(optionalBearerAuthorizationHeaderMiddleware())
}

func optionalBearerAuthorizationHeaderMiddleware() ginx.Middleware {
	return func(next gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			promoteTokenFromCookie(c)
			next(c)
		}
	}
}

func requireBearerAuthorizationHeader(csrfSecret string) gin.HandlerFunc {
	return middlewareToHandler(requireBearerAuthorizationHeaderMiddleware(csrfSecret))
}

func requireBearerAuthorizationHeaderMiddleware(csrfSecret string) ginx.Middleware {
	return func(next gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			promoted := promoteTokenFromCookie(c) || wasAuthHeaderPromotedFromCookie(c)

			authorization := strings.TrimSpace(c.GetHeader("Authorization"))
			parts := strings.Fields(authorization)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}

			isWriteMethod := c.Request.Method == http.MethodPost ||
				c.Request.Method == http.MethodPut ||
				c.Request.Method == http.MethodPatch ||
				c.Request.Method == http.MethodDelete

			if promoted && isWriteMethod {
				cookieToken, err := c.Cookie("_csrf_token")
				if err != nil || cookieToken == "" {
					c.AbortWithStatus(http.StatusForbidden)
					return
				}

				requestToken := c.PostForm("_csrf_token")
				if requestToken == "" {
					requestToken = c.GetHeader("X-CSRF-Token")
				}
				if requestToken == "" || !middleware.ValidateCSRFTokenPair(cookieToken, requestToken, csrfSecret) {
					c.AbortWithStatus(http.StatusForbidden)
					return
				}
			}

			next(c)
		}
	}
}

func bridgeAuthContext() gin.HandlerFunc {
	return middlewareToHandler(bridgeAuthContextMiddleware())
}

func bridgeAuthContextMiddleware() ginx.Middleware {
	return func(next gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			userID, ok := ginx.GetUserID(c)
			if ok {
				if parsedUserID, err := strconv.ParseUint(strings.TrimSpace(userID), 10, strconv.IntSize); err == nil {
					c.Set("userID", uint(parsedUserID))
				}

				if roles, rolesOK := ginx.GetUserRoles(c); rolesOK && len(roles) > 0 {
					c.Set("role", roles[0])
				}
			}

			next(c)
		}
	}
}

func optionalAuthContext(jwtSvc jwt.Service) gin.HandlerFunc {
	return middlewareToHandler(optionalAuthContextMiddleware(jwtSvc))
}

func optionalAuthContextMiddleware(jwtSvc jwt.Service) ginx.Middleware {
	return func(next gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			promoteTokenFromCookie(c)

			authorization := strings.TrimSpace(c.GetHeader("Authorization"))
			parts := strings.Fields(authorization)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") && parts[1] != "" {
				if token, err := jwtSvc.ValidateToken(parts[1]); err == nil && token != nil {
					ginx.SetUserID(c, token.UserID)
					ginx.SetUserRoles(c, token.Roles)

					if parsedUserID, parseErr := strconv.ParseUint(strings.TrimSpace(token.UserID), 10, strconv.IntSize); parseErr == nil {
						c.Set("userID", uint(parsedUserID))
					}

					if len(token.Roles) > 0 {
						c.Set("role", token.Roles[0])
					}
				}
			}

			next(c)
		}
	}
}

func middlewareToHandler(m ginx.Middleware) gin.HandlerFunc {
	return m(func(c *gin.Context) {
		c.Next()
	})
}
