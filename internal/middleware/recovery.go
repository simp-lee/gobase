package middleware

import (
	"log/slog"
	"runtime/debug"
	"strings"

	"github.com/gin-gonic/gin"
)

// Recovery returns a gin middleware that recovers from panics, logs the error
// with stack trace using slog, and returns an appropriate error response.
//
// For requests that accept HTML (Accept header contains "text/html"), it renders
// the errors/500.html template. For all other requests, it returns a JSON response:
//
//	{"code": 500, "message": "internal server error", "data": null}
//
// This middleware is intended to replace gin.Recovery() for applications that
// need structured logging and HTML error page support.
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				stack := debug.Stack()

				logger.ErrorContext(c.Request.Context(), "panic recovered",
					slog.Any("panic", err),
					slog.String("method", c.Request.Method),
					slog.String("path", c.Request.URL.Path),
					slog.String("stack", string(stack)),
				)

				c.Abort()

				if acceptsHTML(c) {
					renderHTMLError(c)
				} else {
					c.JSON(500, gin.H{
						"code":    500,
						"message": "internal server error",
						"data":    nil,
					})
				}
			}
		}()
		c.Next()
	}
}

// renderHTMLError attempts to render the errors/500.html template.
// If the HTML renderer is not configured or rendering fails, it falls back
// to a plain text 500 response.
func renderHTMLError(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			// HTML rendering failed (e.g., no renderer configured).
			// Fall back to a plain text response.
			c.Data(500, "text/plain; charset=utf-8", []byte("500 Internal Server Error"))
		}
	}()
	c.HTML(500, "errors/500.html", gin.H{})
}

// acceptsHTML returns true if the request's Accept header contains "text/html".
func acceptsHTML(c *gin.Context) bool {
	accept := strings.ToLower(c.GetHeader("Accept"))
	return strings.Contains(accept, "text/html")
}
