package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/simp-lee/ginx"
	"github.com/simp-lee/logger"
	"gorm.io/gorm"

	"github.com/simp-lee/gobase/internal/config"
	"github.com/simp-lee/gobase/internal/pkg"
)

type fakeHTTPServer struct {
	listenErr      error
	listenStarted  chan struct{}
	shutdownCalled bool
	stopCh         chan struct{}
	mu             sync.Mutex
}

func (f *fakeHTTPServer) ListenAndServe() error {
	if f.listenStarted != nil {
		close(f.listenStarted)
	}
	if f.listenErr != nil {
		return f.listenErr
	}
	if f.stopCh != nil {
		<-f.stopCh
		return http.ErrServerClosed
	}
	return http.ErrServerClosed
}

func (f *fakeHTTPServer) Shutdown(context.Context) error {
	f.mu.Lock()
	f.shutdownCalled = true
	f.mu.Unlock()
	if f.stopCh != nil {
		close(f.stopCh)
	}
	return nil
}

func (f *fakeHTTPServer) wasShutdownCalled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.shutdownCalled
}

func TestResolveCORSOptions(t *testing.T) {
	tests := []struct {
		name            string
		mode            string
		corsCfg         *config.CORSConfig
		wantOrigins     []string
		wantMethods     []string
		wantHeaders     []string
		wantCredentials bool
		wantMaxAge      time.Duration
	}{
		{
			name:        "debug mode uses permissive default when not configured",
			mode:        gin.DebugMode,
			corsCfg:     &config.CORSConfig{},
			wantOrigins: []string{"*"},
		},
		{
			name:        "release mode denies cross-origin when not configured",
			mode:        gin.ReleaseMode,
			corsCfg:     &config.CORSConfig{},
			wantOrigins: nil,
		},
		{
			name: "release mode uses explicit allowlist",
			mode: gin.ReleaseMode,
			corsCfg: &config.CORSConfig{
				AllowOrigins: []string{"https://admin.example.com"},
			},
			wantOrigins: []string{"https://admin.example.com"},
		},
		{
			name: "config with AllowMethods and AllowHeaders",
			mode: gin.DebugMode,
			corsCfg: &config.CORSConfig{
				AllowMethods: []string{"GET", "POST"},
				AllowHeaders: []string{"Authorization", "Content-Type"},
			},
			wantOrigins: []string{"*"},
			wantMethods: []string{"GET", "POST"},
			wantHeaders: []string{"Authorization", "Content-Type"},
		},
		{
			name: "config with AllowCredentials true",
			mode: gin.ReleaseMode,
			corsCfg: &config.CORSConfig{
				AllowOrigins:     []string{"https://example.com"},
				AllowCredentials: true,
			},
			wantOrigins:     []string{"https://example.com"},
			wantCredentials: true,
		},
		{
			name: "config with MaxAge",
			mode: gin.ReleaseMode,
			corsCfg: &config.CORSConfig{
				AllowOrigins: []string{"https://example.com"},
				MaxAge:       "12h",
			},
			wantOrigins: []string{"https://example.com"},
			wantMaxAge:  12 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := resolveCORSOptions(tt.mode, tt.corsCfg)
			var cfg ginx.CORSConfig
			for _, opt := range opts {
				opt(&cfg)
			}

			// Check AllowOrigins.
			if len(cfg.AllowOrigins) != len(tt.wantOrigins) {
				t.Fatalf("AllowOrigins length = %d, want %d", len(cfg.AllowOrigins), len(tt.wantOrigins))
			}
			for i := range tt.wantOrigins {
				if cfg.AllowOrigins[i] != tt.wantOrigins[i] {
					t.Fatalf("AllowOrigins[%d] = %q, want %q", i, cfg.AllowOrigins[i], tt.wantOrigins[i])
				}
			}

			// Check AllowMethods.
			if len(cfg.AllowMethods) != len(tt.wantMethods) {
				t.Fatalf("AllowMethods length = %d, want %d", len(cfg.AllowMethods), len(tt.wantMethods))
			}
			for i := range tt.wantMethods {
				if cfg.AllowMethods[i] != tt.wantMethods[i] {
					t.Fatalf("AllowMethods[%d] = %q, want %q", i, cfg.AllowMethods[i], tt.wantMethods[i])
				}
			}

			// Check AllowHeaders.
			if len(cfg.AllowHeaders) != len(tt.wantHeaders) {
				t.Fatalf("AllowHeaders length = %d, want %d", len(cfg.AllowHeaders), len(tt.wantHeaders))
			}
			for i := range tt.wantHeaders {
				if cfg.AllowHeaders[i] != tt.wantHeaders[i] {
					t.Fatalf("AllowHeaders[%d] = %q, want %q", i, cfg.AllowHeaders[i], tt.wantHeaders[i])
				}
			}

			// Check AllowCredentials.
			if cfg.AllowCredentials != tt.wantCredentials {
				t.Fatalf("AllowCredentials = %v, want %v", cfg.AllowCredentials, tt.wantCredentials)
			}

			// Check MaxAge.
			if cfg.MaxAge != tt.wantMaxAge {
				t.Fatalf("MaxAge = %v, want %v", cfg.MaxAge, tt.wantMaxAge)
			}
		})
	}
}

func TestValidateGinMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		wantErr bool
	}{
		{name: "debug mode", mode: gin.DebugMode, wantErr: false},
		{name: "release mode", mode: gin.ReleaseMode, wantErr: false},
		{name: "test mode", mode: gin.TestMode, wantErr: false},
		{name: "invalid mode", mode: "staging", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGinMode(tt.mode)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateGinMode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNew_ReturnsError_WhenDatabaseSetupFails(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 8080,
			Mode: gin.TestMode,
		},
		Database: config.DatabaseConfig{
			Driver: "unsupported",
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "text",
		},
	}

	app, err := New(cfg)
	if err == nil {
		t.Fatalf("New() error = nil, want error")
	}
	if app != nil {
		t.Fatalf("New() app = %#v, want nil", app)
	}
	if !strings.Contains(err.Error(), "setup database") {
		t.Fatalf("New() error = %q, want contains %q", err.Error(), "setup database")
	}
}

func TestNew_CSRFSecretValidation(t *testing.T) {
	tests := []struct {
		name            string
		mode            string
		csrfSecret      string
		wantErr         bool
		wantErrContains string
	}{
		{
			name:            "release mode rejects empty csrf secret",
			mode:            gin.ReleaseMode,
			csrfSecret:      "",
			wantErr:         true,
			wantErrContains: "csrf_secret must be a non-placeholder value in release mode",
		},
		{
			name:            "release mode rejects placeholder csrf secret",
			mode:            gin.ReleaseMode,
			csrfSecret:      "change-me-in-env",
			wantErr:         true,
			wantErrContains: "csrf_secret must be a non-placeholder value in release mode",
		},
		{
			name:       "test mode allows empty csrf secret",
			mode:       gin.TestMode,
			csrfSecret: "",
			wantErr:    false,
		},
		{
			name:       "debug mode allows empty csrf secret",
			mode:       gin.DebugMode,
			csrfSecret: " ",
			wantErr:    false,
		},
		{
			name:            "release mode rejects short csrf secret",
			mode:            gin.ReleaseMode,
			csrfSecret:      "Abc123!",
			wantErr:         true,
			wantErrContains: "csrf_secret must be at least 32 characters in release mode",
		},
		{
			name:            "release mode rejects low complexity csrf secret",
			mode:            gin.ReleaseMode,
			csrfSecret:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			wantErr:         true,
			wantErrContains: "csrf_secret must include at least 3 character classes",
		},
		{
			name:       "release mode accepts strong csrf secret",
			mode:       gin.ReleaseMode,
			csrfSecret: "Abcd1234!Abcd1234!Abcd1234!Abcd1234!",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Server: config.ServerConfig{
					Host:       "127.0.0.1",
					Port:       8080,
					Mode:       tt.mode,
					CSRFSecret: tt.csrfSecret,
				},
				Database: config.DatabaseConfig{
					Driver: "sqlite",
					SQLite: config.SQLiteConfig{Path: "file::memory:?cache=shared"},
				},
				Log: config.LogConfig{
					Level:  "info",
					Format: "text",
				},
			}

			app, err := New(cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("New() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				if err == nil {
					t.Fatalf("New() error = nil, want contains %q", tt.wantErrContains)
				}
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("New() error = %q, want contains %q", err.Error(), tt.wantErrContains)
				}
				if app != nil {
					t.Fatalf("New() app = %#v, want nil", app)
				}
				return
			}

			if app == nil {
				t.Fatal("New() app = nil, want non-nil")
			}

			if app.db != nil {
				sqlDB, dbErr := app.db.DB()
				if dbErr == nil {
					_ = sqlDB.Close()
				}
			}
			if app.logger != nil {
				_ = app.logger.Close()
			}
		})
	}
}

func TestEffectiveRateLimitRPS(t *testing.T) {
	tests := []struct {
		name string
		rps  float64
		want int
	}{
		{name: "sub one rounds up to one", rps: 0.5, want: 1},
		{name: "integer stays integer", rps: 1.0, want: 1},
		{name: "fraction rounds up", rps: 1.2, want: 2},
		{name: "larger fraction rounds up", rps: 9.01, want: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectiveRateLimitRPS(tt.rps)
			if got != tt.want {
				t.Fatalf("effectiveRateLimitRPS(%v) = %d, want %d", tt.rps, got, tt.want)
			}
		})
	}
}

func TestNew_ServerTimeoutWhitespace_TreatedAsUnset(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:       "127.0.0.1",
			Port:       8080,
			Mode:       gin.TestMode,
			CSRFSecret: "Abcd1234!Abcd1234!Abcd1234!Abcd1234!",
			Timeout:    "   ",
		},
		Database: config.DatabaseConfig{
			Driver: "sqlite",
			SQLite: config.SQLiteConfig{Path: "file::memory:?cache=shared"},
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "text",
		},
	}

	app, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if app == nil {
		t.Fatal("New() app = nil, want non-nil")
	}

	if app.db != nil {
		sqlDB, dbErr := app.db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	}
	if app.logger != nil {
		_ = app.logger.Close()
	}
}

func TestMiddlewareErrorFormat_Timeout_ReturnsPkgResponse(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:       "127.0.0.1",
			Port:       8080,
			Mode:       gin.TestMode,
			CSRFSecret: "Abcd1234!Abcd1234!Abcd1234!Abcd1234!",
			Timeout:    "5ms",
		},
		Database: config.DatabaseConfig{
			Driver: "sqlite",
			SQLite: config.SQLiteConfig{Path: "file::memory:?cache=shared"},
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "text",
		},
	}

	app, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	defer cleanupTestApp(t, app)

	app.engine.GET("/api/v1/test-timeout-fast", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	app.engine.GET("/api/v1/test-timeout-slow", func(c *gin.Context) {
		time.Sleep(20 * time.Millisecond)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	tests := []struct {
		name            string
		path            string
		wantStatus      int
		wantPkgResponse bool
	}{
		{name: "happy path within timeout", path: "/api/v1/test-timeout-fast", wantStatus: http.StatusOK, wantPkgResponse: false},
		{name: "timeout returns pkg response", path: "/api/v1/test-timeout-slow", wantStatus: http.StatusRequestTimeout, wantPkgResponse: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Header.Set("Accept", "application/json")
			app.engine.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if !tt.wantPkgResponse {
				return
			}

			var resp pkg.Response
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("json decode error: %v", err)
			}
			if resp.Code != http.StatusRequestTimeout {
				t.Fatalf("resp.Code = %d, want %d", resp.Code, http.StatusRequestTimeout)
			}
			if resp.Message != "request timeout" {
				t.Fatalf("resp.Message = %q, want %q", resp.Message, "request timeout")
			}
			if resp.Data != nil {
				t.Fatalf("resp.Data = %#v, want nil", resp.Data)
			}

			var raw map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
				t.Fatalf("json decode raw error: %v", err)
			}
			if len(raw) != 3 {
				t.Fatalf("response field count = %d, want %d", len(raw), 3)
			}
			if _, ok := raw["code"]; !ok {
				t.Fatal("response missing field: code")
			}
			if _, ok := raw["message"]; !ok {
				t.Fatal("response missing field: message")
			}
			data, ok := raw["data"]
			if !ok {
				t.Fatal("response missing field: data")
			}
			if data != nil {
				t.Fatalf("raw data field = %#v, want nil", data)
			}
		})
	}
}

func TestMiddlewareErrorFormat_RateLimit_ReturnsPkgResponse(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:       "127.0.0.1",
			Port:       8080,
			Mode:       gin.TestMode,
			CSRFSecret: "Abcd1234!Abcd1234!Abcd1234!Abcd1234!",
			RateLimit: config.RateLimitConfig{
				Enabled: true,
				RPS:     1,
				Burst:   1,
			},
		},
		Database: config.DatabaseConfig{
			Driver: "sqlite",
			SQLite: config.SQLiteConfig{Path: "file::memory:?cache=shared"},
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "text",
		},
	}

	app, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	defer cleanupTestApp(t, app)

	app.engine.GET("/api/v1/test-rate-limit", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	first := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodGet, "/api/v1/test-rate-limit", nil)
	firstReq.Header.Set("Accept", "application/json")
	app.engine.ServeHTTP(first, firstReq)
	if first.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want %d", first.Code, http.StatusOK)
	}

	second := httptest.NewRecorder()
	secondReq := httptest.NewRequest(http.MethodGet, "/api/v1/test-rate-limit", nil)
	secondReq.Header.Set("Accept", "application/json")
	app.engine.ServeHTTP(second, secondReq)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}

	var resp pkg.Response
	if err := json.Unmarshal(second.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json decode error: %v", err)
	}
	if resp.Code != http.StatusTooManyRequests {
		t.Fatalf("resp.Code = %d, want %d", resp.Code, http.StatusTooManyRequests)
	}
	if resp.Message != "rate limit exceeded" {
		t.Fatalf("resp.Message = %q, want %q", resp.Message, "rate limit exceeded")
	}
	if resp.Data != nil {
		t.Fatalf("resp.Data = %#v, want nil", resp.Data)
	}

	var raw map[string]any
	if err := json.Unmarshal(second.Body.Bytes(), &raw); err != nil {
		t.Fatalf("json decode raw error: %v", err)
	}
	if len(raw) != 3 {
		t.Fatalf("response field count = %d, want %d", len(raw), 3)
	}
	if _, ok := raw["code"]; !ok {
		t.Fatal("response missing field: code")
	}
	if _, ok := raw["message"]; !ok {
		t.Fatal("response missing field: message")
	}
	data, ok := raw["data"]
	if !ok {
		t.Fatal("response missing field: data")
	}
	if data != nil {
		t.Fatalf("raw data field = %#v, want nil", data)
	}
}

func TestRun_ReturnsError_WhenListenFails(t *testing.T) {
	originalNewHTTPServer := newHTTPServer
	originalNotifyContext := notifyContext
	defer func() {
		newHTTPServer = originalNewHTTPServer
		notifyContext = originalNotifyContext
	}()

	listenErr := errors.New("listen failed")
	server := &fakeHTTPServer{listenErr: listenErr}
	newHTTPServer = func(string, http.Handler) httpServer {
		return server
	}
	notifyContext = func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
		return context.WithCancel(context.Background())
	}

	a := &App{
		engine: gin.New(),
		logger: logger.Default(),
		cfg:    &config.Config{Server: config.ServerConfig{Host: "127.0.0.1", Port: 8080}},
	}

	err := a.Run()
	if err == nil {
		t.Fatalf("Run() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "server error") {
		t.Fatalf("Run() error = %q, want contains %q", err.Error(), "server error")
	}
	if !errors.Is(err, listenErr) {
		t.Fatalf("Run() error = %v, want wraps %v", err, listenErr)
	}
}

func TestRun_ShutdownSignal_ClosesDatabase(t *testing.T) {
	originalNewHTTPServer := newHTTPServer
	originalNotifyContext := notifyContext
	defer func() {
		newHTTPServer = originalNewHTTPServer
		notifyContext = originalNotifyContext
	}()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB() error = %v", err)
	}

	server := &fakeHTTPServer{listenStarted: make(chan struct{}), stopCh: make(chan struct{})}
	newHTTPServer = func(string, http.Handler) httpServer {
		return server
	}

	ctx, cancel := context.WithCancel(context.Background())
	notifyContext = func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
		return ctx, cancel
	}

	a := &App{
		engine: gin.New(),
		db:     db,
		logger: logger.Default(),
		cfg:    &config.Config{Server: config.ServerConfig{Host: "127.0.0.1", Port: 8080}},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Run()
	}()

	select {
	case <-server.listenStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not start listening in time")
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return in time after shutdown signal")
	}

	if !server.wasShutdownCalled() {
		t.Fatal("expected server Shutdown() to be called")
	}

	if pingErr := sqlDB.Ping(); pingErr == nil {
		t.Fatal("expected database connection to be closed, but Ping() succeeded")
	}
}

// --- Auth scenario tests ---

func cleanupTestApp(t *testing.T, a *App) {
	t.Helper()
	if a == nil {
		return
	}
	if a.jwtService != nil {
		a.jwtService.Close()
	}
	if a.rbacService != nil {
		_ = a.rbacService.Close()
	}
	if a.db != nil {
		sqlDB, dbErr := a.db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	}
	if a.logger != nil {
		_ = a.logger.Close()
	}
}

func TestNew_AuthDisabled_NoAuthServices(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 8080,
			Mode: gin.TestMode,
		},
		Database: config.DatabaseConfig{
			Driver: "sqlite",
			SQLite: config.SQLiteConfig{Path: "file::memory:?cache=shared"},
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "text",
		},
	}

	app, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	defer cleanupTestApp(t, app)

	if app.jwtService != nil {
		t.Error("expected jwtService to be nil when auth is disabled")
	}
	if app.rbacService != nil {
		t.Error("expected rbacService to be nil when auth is disabled")
	}
}

func TestNew_AuthEnabled_RoutesAndMiddleware(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 8080,
			Mode: gin.TestMode,
		},
		Database: config.DatabaseConfig{
			Driver: "sqlite",
			SQLite: config.SQLiteConfig{Path: "file::memory:?cache=shared"},
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "text",
		},
		Auth: config.AuthConfig{
			Enabled:     true,
			JWTSecret:   "test-secret-key-must-be-at-least-32-chars-long!",
			TokenExpiry: "24h",
			PublicPaths: []string{"/api/v1/auth/login", "/api/v1/auth/register"},
		},
	}

	app, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	defer cleanupTestApp(t, app)

	if app.jwtService == nil {
		t.Fatal("expected jwtService to be non-nil when auth is enabled")
	}
	if app.rbacService != nil {
		t.Error("expected rbacService to be nil when RBAC is not enabled")
	}

	// Protected API route must return 401 without an Authorization header.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	app.engine.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/users without token: status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// Public path (login) must NOT return 401.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	app.engine.ServeHTTP(w, req)
	if w.Code == http.StatusUnauthorized {
		t.Error("POST /api/v1/auth/login should not return 401 (public path)")
	}

	// Public path (register) must NOT return 401.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	app.engine.ServeHTTP(w, req)
	if w.Code == http.StatusUnauthorized {
		t.Error("POST /api/v1/auth/register should not return 401 (public path)")
	}
}

func TestNew_AuthEnabled_WithRBAC(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 8080,
			Mode: gin.TestMode,
		},
		Database: config.DatabaseConfig{
			Driver: "sqlite",
			SQLite: config.SQLiteConfig{Path: "file::memory:?cache=shared"},
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "text",
		},
		Auth: config.AuthConfig{
			Enabled:     true,
			JWTSecret:   "test-secret-key-must-be-at-least-32-chars-long!",
			TokenExpiry: "24h",
			PublicPaths: []string{"/api/v1/auth/login", "/api/v1/auth/register"},
			RBAC: config.RBACConfig{
				Enabled: true,
				Cache: config.RBACCacheConfig{
					RoleTTL:              "5m",
					UserRoleTTL:          "5m",
					PermissionTTL:        "5m",
					MaxRoleEntries:       100,
					MaxUserEntries:       100,
					MaxPermissionEntries: 100,
				},
			},
		},
	}

	app, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	defer cleanupTestApp(t, app)

	if app.jwtService == nil {
		t.Error("expected jwtService to be non-nil when auth is enabled")
	}
	if app.rbacService == nil {
		t.Error("expected rbacService to be non-nil when RBAC is enabled")
	}
}

func TestAutoMigrate_AddsPasswordHashColumnInDebug(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 8080,
			Mode: gin.DebugMode,
		},
		Database: config.DatabaseConfig{
			Driver: "sqlite",
			SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "debug-migrate.db")},
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "text",
		},
	}

	app, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	defer cleanupTestApp(t, app)

	type tableColumn struct {
		Name string `gorm:"column:name"`
	}
	var columns []tableColumn
	if err := app.db.Raw("PRAGMA table_info(users)").Scan(&columns).Error; err != nil {
		t.Fatalf("query users columns: %v", err)
	}

	foundPasswordHash := false
	for _, col := range columns {
		if strings.EqualFold(col.Name, "password_hash") {
			foundPasswordHash = true
			break
		}
	}
	if !foundPasswordHash {
		t.Fatalf("expected users table to include password_hash column, columns=%v", columns)
	}
}

func TestAutoMigrate_DoesNotRunOutsideDebug(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 8080,
			Mode: gin.TestMode,
		},
		Database: config.DatabaseConfig{
			Driver: "sqlite",
			SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "no-migrate.db")},
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "text",
		},
	}

	app, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	defer cleanupTestApp(t, app)

	var userTableCount int
	if err := app.db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'").Scan(&userTableCount).Error; err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if userTableCount != 0 {
		t.Fatalf("expected users table to be absent outside debug mode, count=%d", userTableCount)
	}
}

func TestRun_Shutdown_ClosesAuthServices(t *testing.T) {
	originalNewHTTPServer := newHTTPServer
	originalNotifyContext := notifyContext
	defer func() {
		newHTTPServer = originalNewHTTPServer
		notifyContext = originalNotifyContext
	}()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 8080,
			Mode: gin.TestMode,
		},
		Database: config.DatabaseConfig{
			Driver: "sqlite",
			SQLite: config.SQLiteConfig{Path: "file::memory:?cache=shared"},
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "text",
		},
		Auth: config.AuthConfig{
			Enabled:     true,
			JWTSecret:   "test-secret-key-must-be-at-least-32-chars-long!",
			TokenExpiry: "24h",
			PublicPaths: []string{"/api/v1/auth/login", "/api/v1/auth/register"},
			RBAC: config.RBACConfig{
				Enabled: true,
				Cache: config.RBACCacheConfig{
					RoleTTL:              "5m",
					UserRoleTTL:          "5m",
					PermissionTTL:        "5m",
					MaxRoleEntries:       100,
					MaxUserEntries:       100,
					MaxPermissionEntries: 100,
				},
			},
		},
	}

	app, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	// Verify both services were created before shutdown.
	if app.jwtService == nil {
		t.Fatal("expected jwtService to be non-nil")
	}
	if app.rbacService == nil {
		t.Fatal("expected rbacService to be non-nil")
	}

	server := &fakeHTTPServer{listenStarted: make(chan struct{}), stopCh: make(chan struct{})}
	newHTTPServer = func(string, http.Handler) httpServer {
		return server
	}

	ctx, cancel := context.WithCancel(context.Background())
	notifyContext = func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
		return ctx, cancel
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run()
	}()

	select {
	case <-server.listenStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not start listening in time")
	}

	cancel()

	select {
	case runErr := <-errCh:
		if runErr != nil {
			t.Fatalf("Run() error = %v, want nil", runErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return in time after shutdown signal")
	}

	if !server.wasShutdownCalled() {
		t.Error("expected server Shutdown() to be called")
	}
}
