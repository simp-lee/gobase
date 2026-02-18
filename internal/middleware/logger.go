package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger returns a gin middleware that logs each HTTP request using the provided
// slog.Logger. It records the method, path, status code, latency, and client IP.
//
// The log level is chosen based on the response status code:
//   - 2xx/3xx: Info
//   - 4xx: Warn
//   - 5xx: Error
//
// It uses slog's Context-aware methods (InfoContext, WarnContext, ErrorContext)
// so that the ContextHandler automatically attaches the request_id from context.
func Logger(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		attrs := []slog.Attr{
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", status),
			slog.Duration("latency", latency),
			slog.String("client_ip", c.ClientIP()),
		}

		ctx := c.Request.Context()
		msg := "request"

		switch {
		case status >= 500:
			logger.LogAttrs(ctx, slog.LevelError, msg, attrs...)
		case status >= 400:
			logger.LogAttrs(ctx, slog.LevelWarn, msg, attrs...)
		default:
			logger.LogAttrs(ctx, slog.LevelInfo, msg, attrs...)
		}
	}
}
