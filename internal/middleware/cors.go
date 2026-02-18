package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORSConfig holds the configuration for the CORS middleware.
type CORSConfig struct {
	// AllowOrigins is a list of origins that are allowed to make cross-origin requests.
	// Use ["*"] to allow all origins (default in debug mode).
	AllowOrigins []string

	// AllowMethods is a list of HTTP methods allowed for cross-origin requests.
	AllowMethods []string

	// AllowHeaders is a list of headers allowed in cross-origin requests.
	AllowHeaders []string

	// AllowCredentials indicates whether the request can include credentials like cookies.
	AllowCredentials bool

	// MaxAge indicates how long (in seconds) the results of a preflight request can be cached.
	MaxAge string
}

// DefaultCORSConfig returns a permissive CORS configuration suitable for development.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With", "X-CSRF-Token", "HX-Request", "HX-Current-URL", "HX-Target", "HX-Trigger"},
		AllowCredentials: false,
		MaxAge:           "86400",
	}
}

// CORS returns a gin middleware that handles Cross-Origin Resource Sharing.
// It uses DefaultCORSConfig which is permissive for development.
func CORS() gin.HandlerFunc {
	return CORSWithConfig(DefaultCORSConfig())
}

// CORSWithConfig returns a gin middleware that handles Cross-Origin Resource Sharing
// using the provided configuration.
func CORSWithConfig(cfg CORSConfig) gin.HandlerFunc {
	allowOrigins := strings.Join(cfg.AllowOrigins, ", ")
	allowMethods := strings.Join(cfg.AllowMethods, ", ")
	allowHeaders := strings.Join(cfg.AllowHeaders, ", ")

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		// Always set Vary when CORS processing is active, so caches
		// differentiate responses by Origin.
		c.Writer.Header().Add("Vary", "Origin")

		// Determine which origin to reflect back.
		if allowOrigins == "*" {
			// When credentials are enabled, we must echo the specific origin
			// instead of using the wildcard "*".
			if cfg.AllowCredentials {
				c.Header("Access-Control-Allow-Origin", origin)
			} else {
				c.Header("Access-Control-Allow-Origin", "*")
			}
		} else if originAllowed(cfg.AllowOrigins, origin) {
			c.Header("Access-Control-Allow-Origin", origin)
		} else {
			// Origin not allowed — skip CORS headers entirely.
			c.Next()
			return
		}

		c.Header("Access-Control-Allow-Methods", allowMethods)
		c.Header("Access-Control-Allow-Headers", allowHeaders)
		c.Header("Access-Control-Max-Age", cfg.MaxAge)

		if cfg.AllowCredentials {
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		// Handle preflight OPTIONS requests.
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// originAllowed checks whether the given origin is in the allowed list.
func originAllowed(allowed []string, origin string) bool {
	for _, a := range allowed {
		if a == "*" || a == origin {
			return true
		}
	}
	return false
}
