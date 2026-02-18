package middleware

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"regexp"
	"sync/atomic"
	"time"

	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/simp-lee/logger"
)

const (
	requestIDHeader     = "X-Request-ID"
	requestIDContextKey = "request_id"
	requestIDLength     = 16 // 16 bytes = 32 hex chars
)

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

var requestIDFallbackCounter atomic.Uint64

// RequestIDConfig controls request-id reuse behavior.
type RequestIDConfig struct {
	TrustUpstream bool
}

// RequestID returns a gin middleware that assigns a unique request ID to each request.
//
// By default, upstream X-Request-ID values are not trusted and a new ID is generated
// for every request.
//
// The request ID is:
//   - Stored in gin.Context under the key "request_id"
//   - Set as the X-Request-ID response header
//   - Stored in the Go context via logger.WithContextAttrs for structured logging
func RequestID() gin.HandlerFunc {
	return RequestIDWithConfig(RequestIDConfig{})
}

// RequestIDWithConfig returns a gin middleware that assigns request IDs based on config.
//
// When TrustUpstream is enabled, a valid incoming X-Request-ID is reused.
// Otherwise, a new random ID is generated.
func RequestIDWithConfig(cfg RequestIDConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := ""
		if cfg.TrustUpstream {
			upstreamID := c.GetHeader(requestIDHeader)
			if isValidRequestID(upstreamID) {
				id = upstreamID
			}
		}

		if id == "" {
			id = generateRequestID()
		}

		c.Set(requestIDContextKey, id)
		c.Header(requestIDHeader, id)

		ctx := logger.WithContextAttrs(c.Request.Context(), slog.String("request_id", id))
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

func isValidRequestID(id string) bool {
	return requestIDPattern.MatchString(id)
}

// GetRequestID extracts the request ID from the gin.Context.
// Returns an empty string if no request ID is set.
func GetRequestID(c *gin.Context) string {
	if id, exists := c.Get(requestIDContextKey); exists {
		if s, ok := id.(string); ok {
			return s
		}
	}
	return ""
}

// generateRequestID produces a random hex string of requestIDLength*2 characters.
func generateRequestID() string {
	b := make([]byte, requestIDLength)
	_, err := rand.Read(b)
	if err != nil {
		binary.BigEndian.PutUint64(b[:8], uint64(time.Now().UnixNano()))
		binary.BigEndian.PutUint64(b[8:], requestIDFallbackCounter.Add(1))
		return hex.EncodeToString(b)
	}
	return hex.EncodeToString(b)
}
