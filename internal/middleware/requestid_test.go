package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/simp-lee/logger"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupRequestIDRouter() *gin.Engine {
	r := gin.New()
	r.Use(RequestID())
	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, GetRequestID(c))
	})
	r.GET("/ctx", func(c *gin.Context) {
		// Verify the request ID is available in the Go context via logger.WithContextAttrs
		attrs := logger.FromContext(c.Request.Context())
		id := findAttrValue(attrs, "request_id")
		c.String(http.StatusOK, id)
	})
	return r
}

func setupRequestIDRouterWithConfig(cfg RequestIDConfig) *gin.Engine {
	r := gin.New()
	r.Use(RequestIDWithConfig(cfg))
	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, GetRequestID(c))
	})
	r.GET("/ctx", func(c *gin.Context) {
		// Verify the request ID is available in the Go context via logger.WithContextAttrs
		attrs := logger.FromContext(c.Request.Context())
		id := findAttrValue(attrs, "request_id")
		c.String(http.StatusOK, id)
	})
	return r
}

func findAttrValue(attrs []slog.Attr, key string) string {
	for _, a := range attrs {
		if a.Key == key {
			return a.Value.String()
		}
	}
	return ""
}

func TestRequestID_GeneratesID(t *testing.T) {
	r := setupRequestIDRouter()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if len(body) != requestIDLength*2 {
		t.Errorf("expected request ID of length %d, got %d (%q)", requestIDLength*2, len(body), body)
	}

	header := w.Header().Get(requestIDHeader)
	if header != body {
		t.Errorf("response header %q = %q; want %q", requestIDHeader, header, body)
	}
}

func TestRequestID_ReusesUpstreamHeader(t *testing.T) {
	r := setupRequestIDRouterWithConfig(RequestIDConfig{TrustUpstream: true})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(requestIDHeader, "upstream-id-123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if body != "upstream-id-123" {
		t.Errorf("expected request ID %q, got %q", "upstream-id-123", body)
	}

	header := w.Header().Get(requestIDHeader)
	if header != "upstream-id-123" {
		t.Errorf("response header %q = %q; want %q", requestIDHeader, header, "upstream-id-123")
	}
}

func TestRequestID_StoredInGoContext(t *testing.T) {
	r := setupRequestIDRouterWithConfig(RequestIDConfig{TrustUpstream: true})

	req := httptest.NewRequest(http.MethodGet, "/ctx", nil)
	req.Header.Set(requestIDHeader, "ctx-test-456")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if body != "ctx-test-456" {
		t.Errorf("expected request ID in context %q, got %q", "ctx-test-456", body)
	}
}

func TestRequestID_UniquePerRequest(t *testing.T) {
	r := setupRequestIDRouter()

	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		id := w.Body.String()
		if ids[id] {
			t.Fatalf("duplicate request ID generated: %q", id)
		}
		ids[id] = true
	}
}

func TestRequestID_InvalidUpstreamHeaderTooLong_GeneratesNew(t *testing.T) {
	r := setupRequestIDRouterWithConfig(RequestIDConfig{TrustUpstream: true})

	invalid := strings.Repeat("a", 65)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(requestIDHeader, invalid)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if body == invalid {
		t.Fatalf("expected middleware to reject invalid upstream id and generate a new one")
	}
	if len(body) != requestIDLength*2 {
		t.Fatalf("expected generated request ID length %d, got %d", requestIDLength*2, len(body))
	}
}

func TestRequestID_InvalidUpstreamHeaderCharset_GeneratesNew(t *testing.T) {
	r := setupRequestIDRouterWithConfig(RequestIDConfig{TrustUpstream: true})

	invalid := "bad_id"
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(requestIDHeader, invalid)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if body == invalid {
		t.Fatalf("expected middleware to reject invalid upstream id and generate a new one")
	}
	if len(body) != requestIDLength*2 {
		t.Fatalf("expected generated request ID length %d, got %d", requestIDLength*2, len(body))
	}
}

func TestRequestID_ValidUpstreamHeaderBoundary64_Reused(t *testing.T) {
	r := setupRequestIDRouterWithConfig(RequestIDConfig{TrustUpstream: true})

	valid := strings.Repeat("a", 64)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(requestIDHeader, valid)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if body != valid {
		t.Fatalf("expected valid upstream id to be reused, got %q", body)
	}
}

func TestGetRequestID_Empty(t *testing.T) {
	r := gin.New()
	// No RequestID middleware
	r.GET("/no-id", func(c *gin.Context) {
		c.String(http.StatusOK, GetRequestID(c))
	})

	req := httptest.NewRequest(http.MethodGet, "/no-id", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Body.String() != "" {
		t.Errorf("expected empty request ID, got %q", w.Body.String())
	}
}
