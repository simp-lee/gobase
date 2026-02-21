package app

import "github.com/gin-gonic/gin"

// Module defines the contract for a self-registering business module.
// Each module registers its own API and page routes.
type Module interface {
	RegisterRoutes(api *gin.RouterGroup, pages *gin.RouterGroup)
}
