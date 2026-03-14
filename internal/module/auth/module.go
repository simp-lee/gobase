package auth

import "github.com/gin-gonic/gin"

// AuthModule implements the app.Module interface for the auth domain.
type AuthModule struct {
	handler *AuthHandler
}

// NewModule creates a new AuthModule with the given handler.
// Panics if h is nil.
func NewModule(h *AuthHandler) *AuthModule {
	if h == nil {
		panic("auth.NewModule: handler must not be nil")
	}
	return &AuthModule{handler: h}
}

// RegisterRoutes registers auth API and page routes.
func (m *AuthModule) RegisterRoutes(api *gin.RouterGroup, pages *gin.RouterGroup) {
	auth := api.Group("/auth")
	auth.POST("/login", m.handler.Login)
	auth.POST("/register", m.handler.Register)
	auth.POST("/logout", m.handler.Logout)
	auth.POST("/refresh", m.handler.RefreshToken)

	if pages != nil {
		pages.GET("/login", m.handler.LoginPage)
		pages.GET("/register", m.handler.RegisterPage)
	}
}
