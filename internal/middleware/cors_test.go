package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupRouter(middleware gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(middleware)
	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	r.POST("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return r
}

func TestCORS_DefaultConfig_SetsHeaders(t *testing.T) {
	r := setupRouter(CORS())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("expected Allow-Origin *, got %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("expected Allow-Methods header to be set")
	}
	if got := w.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Error("expected Allow-Headers header to be set")
	}
	if got := w.Header().Get("Access-Control-Max-Age"); got != "86400" {
		t.Errorf("expected Max-Age 86400, got %q", got)
	}
	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Errorf("expected Vary Origin, got %q", got)
	}
}

func TestCORS_PreflightOptions_Returns204(t *testing.T) {
	r := setupRouter(CORS())

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("expected Allow-Origin *, got %q", got)
	}
}

func TestCORS_NoOriginHeader_SkipsCORSHeaders(t *testing.T) {
	r := setupRouter(CORS())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	// No Origin header set
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no Allow-Origin header, got %q", got)
	}
}

func TestCORS_WithConfig_SpecificOrigins_Allowed(t *testing.T) {
	cfg := CORSConfig{
		AllowOrigins:     []string{"http://example.com", "http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST"},
		AllowHeaders:     []string{"Content-Type"},
		AllowCredentials: false,
		MaxAge:           "3600",
	}
	r := setupRouter(CORSWithConfig(cfg))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://example.com" {
		t.Errorf("expected Allow-Origin http://example.com, got %q", got)
	}
	if got := w.Header().Get("Access-Control-Max-Age"); got != "3600" {
		t.Errorf("expected Max-Age 3600, got %q", got)
	}
}

func TestCORS_WithConfig_SpecificOrigins_Denied(t *testing.T) {
	cfg := CORSConfig{
		AllowOrigins: []string{"http://example.com"},
		AllowMethods: []string{"GET"},
		AllowHeaders: []string{"Content-Type"},
		MaxAge:       "3600",
	}
	r := setupRouter(CORSWithConfig(cfg))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://evil.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no Allow-Origin header for denied origin, got %q", got)
	}
	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Errorf("expected Vary Origin even for denied origin, got %q", got)
	}
}

func TestCORS_WithConfig_EmptyAllowOrigins_Denied(t *testing.T) {
	cfg := CORSConfig{
		AllowOrigins: []string{},
		AllowMethods: []string{"GET"},
		AllowHeaders: []string{"Content-Type"},
		MaxAge:       "3600",
	}
	r := setupRouter(CORSWithConfig(cfg))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no Allow-Origin header for empty allowlist, got %q", got)
	}
	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Errorf("expected Vary Origin even when allowlist empty, got %q", got)
	}
}

func TestCORS_WithCredentials_EchosOrigin(t *testing.T) {
	cfg := DefaultCORSConfig()
	cfg.AllowCredentials = true

	r := setupRouter(CORSWithConfig(cfg))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://example.com" {
		t.Errorf("expected origin echo http://example.com, got %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("expected Allow-Credentials true, got %q", got)
	}
}

func TestOriginAllowed(t *testing.T) {
	tests := []struct {
		name    string
		allowed []string
		origin  string
		want    bool
	}{
		{"wildcard allows any", []string{"*"}, "http://any.com", true},
		{"exact match", []string{"http://a.com"}, "http://a.com", true},
		{"no match", []string{"http://a.com"}, "http://b.com", false},
		{"multiple with match", []string{"http://a.com", "http://b.com"}, "http://b.com", true},
		{"multiple no match", []string{"http://a.com", "http://b.com"}, "http://c.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := originAllowed(tt.allowed, tt.origin); got != tt.want {
				t.Errorf("originAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}
