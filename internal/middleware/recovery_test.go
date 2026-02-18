package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
)

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func setupRecoveryRouter(logger *slog.Logger, htmlRenderer ...render.HTMLRender) *gin.Engine {
	r := gin.New()
	r.Use(Recovery(logger))
	if len(htmlRenderer) > 0 {
		r.HTMLRender = htmlRenderer[0]
	}
	r.GET("/panic", func(c *gin.Context) {
		panic("test panic")
	})
	r.GET("/ok", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return r
}

func TestRecovery_NoPanic_PassesThrough(t *testing.T) {
	var logBuf bytes.Buffer
	r := setupRecoveryRouter(newTestLogger(&logBuf))

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %q", w.Body.String())
	}
}

func TestRecovery_Panic_JSONResponse(t *testing.T) {
	var logBuf bytes.Buffer
	r := setupRecoveryRouter(newTestLogger(&logBuf))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}
	if code, ok := body["code"].(float64); !ok || int(code) != 500 {
		t.Errorf("expected code 500, got %v", body["code"])
	}
	if msg, ok := body["message"].(string); !ok || msg != "internal server error" {
		t.Errorf("expected message 'internal server error', got %v", body["message"])
	}
	if val, exists := body["data"]; !exists {
		t.Error("expected 'data' field in response")
	} else if val != nil {
		t.Errorf("expected 'data' to be null, got %v", val)
	}
}

func TestRecovery_Panic_HTMLResponse_FallbackWithoutRenderer(t *testing.T) {
	var logBuf bytes.Buffer
	r := setupRecoveryRouter(newTestLogger(&logBuf))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}

	// Without a real template renderer, renderHTMLError falls back to plain text.
	body := w.Body.String()
	if !strings.Contains(body, "500") {
		t.Errorf("expected body to contain '500', got %q", body)
	}
}

func TestRecovery_Panic_LogsDetails(t *testing.T) {
	var logBuf bytes.Buffer
	r := setupRecoveryRouter(newTestLogger(&logBuf))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "test panic") {
		t.Errorf("expected log to contain panic value 'test panic', got:\n%s", logOutput)
	}
	if !strings.Contains(logOutput, "panic recovered") {
		t.Errorf("expected log to contain 'panic recovered', got:\n%s", logOutput)
	}
}

func TestRecovery_Panic_AbortsFurtherHandlers(t *testing.T) {
	var logBuf bytes.Buffer
	logger := newTestLogger(&logBuf)

	handlerCalled := false
	r := gin.New()
	r.Use(Recovery(logger))
	r.Use(func(c *gin.Context) {
		c.Next()
		// This runs after the panic handler in the chain
		// but the response should already be written
	})
	r.GET("/panic", func(c *gin.Context) {
		panic("abort test")
	})
	r.GET("/after", func(c *gin.Context) {
		handlerCalled = true
		c.String(http.StatusOK, "should not reach")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}
	if handlerCalled {
		t.Error("expected subsequent handler NOT to be called after panic recovery")
	}
}

func TestRecovery_Panic_DefaultAcceptIsJSON(t *testing.T) {
	// When Accept header is empty or unrecognized, default to JSON
	var logBuf bytes.Buffer
	r := setupRecoveryRouter(newTestLogger(&logBuf))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	// No Accept header set
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON response when no Accept header, got: %s", w.Body.String())
	}
	if code, ok := body["code"].(float64); !ok || int(code) != 500 {
		t.Errorf("expected code 500, got %v", body["code"])
	}
}
