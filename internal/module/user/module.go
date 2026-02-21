package user

import "github.com/gin-gonic/gin"

// UserModule implements the app.Module interface for the user domain.
type UserModule struct {
	handler     *UserHandler
	pageHandler *UserPageHandler
}

// NewModule creates a new UserModule with the given handlers.
// Panics if h or ph is nil.
func NewModule(h *UserHandler, ph *UserPageHandler) *UserModule {
	if h == nil {
		panic("user.NewModule: handler must not be nil")
	}
	if ph == nil {
		panic("user.NewModule: pageHandler must not be nil")
	}
	return &UserModule{handler: h, pageHandler: ph}
}

// RegisterRoutes registers user API and page routes.
func (m *UserModule) RegisterRoutes(api *gin.RouterGroup, pages *gin.RouterGroup) {
	// API routes
	api.POST("/users", m.handler.Create)
	api.GET("/users/:id", m.handler.Get)
	api.GET("/users", m.handler.List)
	api.PUT("/users/:id", m.handler.Update)
	api.DELETE("/users/:id", m.handler.Delete)

	// Page routes
	pages.GET("/users", m.pageHandler.ListPage)
	pages.GET("/users/new", m.pageHandler.NewPage)
	pages.GET("/users/:id/edit", m.pageHandler.EditPage)
	pages.POST("/users", m.pageHandler.CreateHTMX)
	pages.PUT("/users/:id", m.pageHandler.UpdateHTMX)
	pages.DELETE("/users/:id", m.pageHandler.DeleteHTMX)
}
