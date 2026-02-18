package app

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/simp-lee/gobase/internal/middleware"
	"github.com/simp-lee/gobase/internal/module/user"
	"github.com/simp-lee/gobase/web"
)

// RouteDeps holds all dependencies needed to register routes.
type RouteDeps struct {
	UserHandler     *user.UserHandler
	UserPageHandler *user.UserPageHandler
	DB              *gorm.DB
	Mode            string // "debug" or "release"
	CSRFSecret      string
}

// RegisterRoutes registers all application routes on the given gin.Engine.
func RegisterRoutes(r *gin.Engine, deps *RouteDeps) error {
	if r == nil {
		return errors.New("router is nil")
	}
	if deps == nil {
		return errors.New("route dependencies are nil")
	}
	if deps.UserHandler == nil {
		return errors.New("user handler is nil")
	}
	if deps.UserPageHandler == nil {
		return errors.New("user page handler is nil")
	}
	if strings.TrimSpace(deps.CSRFSecret) == "" {
		return errors.New("csrf secret is required")
	}

	// Static assets
	if err := registerStaticRoutesWithError(r, deps.Mode); err != nil {
		return fmt.Errorf("register static routes: %w", err)
	}

	// Health check (M3)
	r.GET("/health", healthHandler(deps.DB))

	// Home page (with CSRF so templates have a token)
	r.GET("/", middleware.CSRF(deps.CSRFSecret), func(c *gin.Context) {
		c.HTML(http.StatusOK, "home.html", gin.H{
			"CSRFToken": middleware.GetCSRFToken(c),
		})
	})

	// API routes — no CSRF
	api := r.Group("/api")
	{
		h := deps.UserHandler
		api.POST("/users", h.Create)
		api.GET("/users/:id", h.Get)
		api.GET("/users", h.List)
		api.PUT("/users/:id", h.Update)
		api.DELETE("/users/:id", h.Delete)
	}

	// Page routes — with CSRF
	pages := r.Group("/")
	pages.Use(middleware.CSRF(deps.CSRFSecret))
	{
		ph := deps.UserPageHandler
		pages.GET("/users", ph.ListPage)
		pages.GET("/users/new", ph.NewPage)
		pages.GET("/users/:id/edit", ph.EditPage)
		pages.POST("/users", ph.CreateHTMX)
		pages.PUT("/users/:id", ph.UpdateHTMX)
		pages.DELETE("/users/:id", ph.DeleteHTMX)
	}

	// NoRoute handler (M5)
	r.NoRoute(noRouteHandler())

	return nil
}

// healthHandler returns a handler that pings the database and reports status.
func healthHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		dbStatus := "ok"
		status := "ok"
		code := http.StatusOK

		if db == nil {
			dbStatus = "error"
			status = "degraded"
			code = http.StatusServiceUnavailable
			c.JSON(code, gin.H{
				"status": status,
				"components": gin.H{
					"database": dbStatus,
				},
			})
			return
		}

		sqlDB, err := db.DB()
		if err != nil {
			dbStatus = "error"
			status = "degraded"
			code = http.StatusServiceUnavailable
		} else if err = sqlDB.Ping(); err != nil {
			dbStatus = "error"
			status = "degraded"
			code = http.StatusServiceUnavailable
		}

		c.JSON(code, gin.H{
			"status": status,
			"components": gin.H{
				"database": dbStatus,
			},
		})
	}
}

// noRouteHandler returns a handler that renders a 404 HTML page for browser
// requests or a JSON response for API clients.
func noRouteHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		accept := strings.ToLower(c.GetHeader("Accept"))
		if strings.Contains(accept, "text/html") {
			c.HTML(http.StatusNotFound, "errors/404.html", gin.H{})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{
			"code":    http.StatusNotFound,
			"message": "not found",
			"data":    nil,
		})
	}
}

func registerStaticRoutesWithError(r *gin.Engine, mode string) error {
	if mode == "debug" {
		debugStaticFS, err := resolveDebugStaticFS()
		if err != nil {
			return fmt.Errorf("resolve debug static filesystem: %w", err)
		}
		fileServer := http.StripPrefix("/static", http.FileServer(http.FS(debugStaticFS)))
		r.GET("/static/*filepath", func(c *gin.Context) {
			fileServer.ServeHTTP(c.Writer, c.Request)
		})
		return nil
	}

	// Release mode: serve from embed.FS with cache headers.
	staticFS, err := fs.Sub(web.EmbeddedFS, "static")
	if err != nil {
		return fmt.Errorf("create sub filesystem for static assets: %w", err)
	}
	r.GET("/static/*filepath", cacheStaticHandler(http.FS(staticFS)))
	return nil
}

func resolveDebugStaticFS() (fs.FS, error) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return nil, errors.New("resolve current file path")
	}

	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	staticDir := filepath.Join(projectRoot, "web", "static")
	if _, err := os.Stat(staticDir); err != nil {
		return nil, fmt.Errorf("stat static directory %q: %w", staticDir, err)
	}

	return os.DirFS(staticDir), nil
}

// cacheStaticHandler wraps an http.FileSystem handler and sets a Cache-Control header
// for release mode static assets (M8).
func cacheStaticHandler(fsys http.FileSystem) gin.HandlerFunc {
	fileServer := http.StripPrefix("/static", http.FileServer(fsys))
	return func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=86400")
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}
