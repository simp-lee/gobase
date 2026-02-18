package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/simp-lee/logger"
)

func setupLoggerRouter(logger *slog.Logger) *gin.Engine {
	return setupLoggerRouterWithRequestID(logger, RequestID())
}

func setupLoggerRouterWithRequestID(logger *slog.Logger, requestID gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(requestID)
	r.Use(Logger(logger))

	r.GET("/ok", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	r.GET("/not-found", func(c *gin.Context) {
		c.String(http.StatusNotFound, "not found")
	})
	r.GET("/error", func(c *gin.Context) {
		c.String(http.StatusInternalServerError, "error")
	})
	r.POST("/create", func(c *gin.Context) {
		c.String(http.StatusCreated, "created")
	})
	return r
}

func TestLogger_LogsInfoForSuccess(t *testing.T) {
	var logBuf bytes.Buffer
	r := setupLoggerRouter(newTestLogger(&logBuf))

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "level=INFO") {
		t.Errorf("expected INFO level log, got:\n%s", logOutput)
	}
	if !strings.Contains(logOutput, "request") {
		t.Errorf("expected log message 'request', got:\n%s", logOutput)
	}
}

func TestLogger_LogsWarnFor4xx(t *testing.T) {
	var logBuf bytes.Buffer
	r := setupLoggerRouter(newTestLogger(&logBuf))

	req := httptest.NewRequest(http.MethodGet, "/not-found", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "level=WARN") {
		t.Errorf("expected WARN level log, got:\n%s", logOutput)
	}
}

func TestLogger_LogsErrorFor5xx(t *testing.T) {
	var logBuf bytes.Buffer
	r := setupLoggerRouter(newTestLogger(&logBuf))

	req := httptest.NewRequest(http.MethodGet, "/error", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "level=ERROR") {
		t.Errorf("expected ERROR level log, got:\n%s", logOutput)
	}
}

func TestLogger_ContainsExpectedFields(t *testing.T) {
	var logBuf bytes.Buffer
	r := setupLoggerRouter(newTestLogger(&logBuf))

	req := httptest.NewRequest(http.MethodPost, "/create", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", w.Code)
	}

	logOutput := logBuf.String()
	for _, field := range []string{"method=POST", "path=/create", "status=201", "latency=", "client_ip="} {
		if !strings.Contains(logOutput, field) {
			t.Errorf("expected log to contain %q, got:\n%s", field, logOutput)
		}
	}
}

func TestLogger_IncludesRequestIDFromContext(t *testing.T) {
	var logBuf bytes.Buffer
	log, err := logger.New(
		logger.WithConsoleWriter(&logBuf),
		logger.WithConsoleFormat(logger.FormatText),
		logger.WithConsoleColor(false),
		logger.WithLevel(slog.LevelDebug),
		logger.WithMiddleware(logger.ContextMiddleware()),
	)
	if err != nil {
		t.Fatalf("logger.New error: %v", err)
	}
	defer log.Close()

	r := setupLoggerRouterWithRequestID(log.Logger, RequestIDWithConfig(RequestIDConfig{TrustUpstream: true}))

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req.Header.Set("X-Request-ID", "test-req-id-789")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "test-req-id-789") {
		t.Errorf("expected log to contain request_id 'test-req-id-789', got:\n%s", logOutput)
	}
}
