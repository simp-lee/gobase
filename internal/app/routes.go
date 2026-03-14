package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/simp-lee/gobase/internal/middleware"
	"github.com/simp-lee/gobase/internal/pkg"
	"github.com/simp-lee/gobase/web"
)

// RouteDeps holds all dependencies needed to register routes.
type RouteDeps struct {
	Modules    []Module
	DB         *gorm.DB
	Mode       string // "debug" or "release"
	CSRFSecret string
}

// RegisterRoutes registers all application routes on the given gin.Engine.
func RegisterRoutes(r *gin.Engine, deps *RouteDeps) error {
	if r == nil {
		return errors.New("router is nil")
	}
	if deps == nil {
		return errors.New("route dependencies are nil")
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

	// API routes — no CSRF
	api := r.Group("/api/v1")

	// Page routes — with CSRF
	pages := r.Group("/")
	pages.Use(middleware.CSRF(deps.CSRFSecret))
	registerDefaultPageRoutes(pages)

	// Register module routes with panic protection and duplicate detection
	routeOwners := make(map[string]int)
	for _, routeInfo := range r.Routes() {
		routeOwners[routeSignature(routeInfo.Method, routeInfo.Path)] = -1
	}

	for i, m := range deps.Modules {
		if m == nil {
			return fmt.Errorf("module at index %d is nil", i)
		}

		before := snapshotRouteSignatures(r.Routes())
		if err := registerModuleRoutesSafely(m, api, pages); err != nil {
			return fmt.Errorf("register routes for module at index %d: %w", i, err)
		}

		for _, routeInfo := range r.Routes() {
			signature := routeSignature(routeInfo.Method, routeInfo.Path)
			if _, existedBefore := before[signature]; existedBefore {
				continue
			}
			if owner, exists := routeOwners[signature]; exists {
				return fmt.Errorf("duplicate route %q from module at index %d (already registered by module at index %d)", signature, i, owner)
			}
			routeOwners[signature] = i
		}
	}

	// NoRoute handler (M5)
	r.NoRoute(noRouteHandler())

	return nil
}

func registerModuleRoutesSafely(m Module, api *gin.RouterGroup, pages *gin.RouterGroup) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic while registering routes: %v", rec)
		}
	}()

	m.RegisterRoutes(api, pages)
	return nil
}

func snapshotRouteSignatures(routes gin.RoutesInfo) map[string]struct{} {
	signatures := make(map[string]struct{}, len(routes))
	for _, routeInfo := range routes {
		signatures[routeSignature(routeInfo.Method, routeInfo.Path)] = struct{}{}
	}
	return signatures
}

func routeSignature(method, path string) string {
	normalizedMethod := strings.ToUpper(strings.TrimSpace(method))
	normalizedPath := strings.TrimSpace(path)
	if normalizedPath == "" {
		normalizedPath = "/"
	}
	if !strings.HasPrefix(normalizedPath, "/") {
		normalizedPath = "/" + normalizedPath
	}
	if len(normalizedPath) > 1 {
		normalizedPath = strings.TrimSuffix(normalizedPath, "/")
	}
	return normalizedMethod + " " + normalizedPath
}

func registerDefaultPageRoutes(pages *gin.RouterGroup) {
	pages.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "home.html", gin.H{
			"CSRFToken": middleware.GetCSRFToken(c),
		})
	})
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
		} else {
			ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
			defer cancel()
			err = sqlDB.PingContext(ctx)
			if err != nil {
				dbStatus = "error"
				status = "degraded"
				code = http.StatusServiceUnavailable
			}
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
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") {
			c.JSON(http.StatusNotFound, pkg.Response{Code: http.StatusNotFound, Message: "not found"})
			return
		}

		renderError(c, http.StatusNotFound, "not found")
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
			requestedPath := c.Param("filepath")
			if strings.HasSuffix(requestedPath, "/sw.js") || strings.HasSuffix(requestedPath, "/manifest.json") {
				c.Header("Cache-Control", "no-cache, max-age=0, must-revalidate")
			} else {
				c.Header("Cache-Control", "no-store")
			}
			if strings.HasSuffix(requestedPath, "/sw.js") {
				c.Header("Service-Worker-Allowed", "/")
			}
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
		requestedPath := c.Param("filepath")
		if strings.HasSuffix(requestedPath, "/sw.js") || strings.HasSuffix(requestedPath, "/manifest.json") {
			c.Header("Cache-Control", "no-cache, max-age=0, must-revalidate")
		} else {
			c.Header("Cache-Control", "public, max-age=86400")
		}
		if strings.HasSuffix(requestedPath, "/sw.js") {
			c.Header("Service-Worker-Allowed", "/")
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}
