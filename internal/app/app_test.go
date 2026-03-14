package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	// Release-mode CSRF rejection (placeholder, length, complexity) is now
	// validated by config.Validate() and tested in config_test.go
	// TestLoad_CSRFSecretValidation. App layer only tests the non-release
	// random-generation fallback.
	tests := []struct {
		name            string
		mode            string
		csrfSecret      string
		wantErr         bool
		wantErrContains string
	}{
		{
			name:       "test mode generates random secret for empty value",
			mode:       gin.TestMode,
			csrfSecret: "",
			wantErr:    false,
		},
		{
			name:       "debug mode generates random secret for whitespace",
			mode:       gin.DebugMode,
			csrfSecret: " ",
			wantErr:    false,
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
			defer cleanupTestApp(t, app)
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
	defer cleanupTestApp(t, app)
}

func TestMiddlewareErrorFormat_Timeout_ReturnsPkgResponse(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:       "127.0.0.1",
			Port:       8080,
			Mode:       gin.TestMode,
			CSRFSecret: "Abcd1234!Abcd1234!Abcd1234!Abcd1234!",
			Timeout:    "5s",
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
		time.Sleep(10 * time.Second)
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

// --- Auth scenario tests ---

func cleanupTestApp(t *testing.T, a *App) {
	t.Helper()
	if a == nil {
		return
	}
	a.close()
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

	// Protected users route: authenticated request without permission should be denied by RBAC.
	token, tokenErr := app.jwtService.GenerateToken("9999", nil, time.Hour)
	if tokenErr != nil {
		t.Fatalf("GenerateToken() error = %v, want nil", tokenErr)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	app.engine.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("GET /api/v1/users with token but without permission: status = %d, want %d", w.Code, http.StatusForbidden)
	}

	// Public auth route should not be intercepted by auth/RBAC middleware.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"bad","password":"short"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://example.com")
	app.engine.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("POST /api/v1/auth/login should bypass auth/RBAC middleware: status = %d, want %d", w.Code, http.StatusBadRequest)
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

// --- wire.go tests ---

func TestWireModules_ReturnsModuleAndRepo(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	}()

	modules, repo := wireModules(db)
	if len(modules) != 1 {
		t.Fatalf("wireModules() returned %d modules, want 1", len(modules))
	}
	if repo == nil {
		t.Fatal("wireModules() returned nil repo")
	}
}

func TestWireDeps_Success(t *testing.T) {
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
			// Valid config, so wireDeps succeeds.
		},
	}

	app, err := wireDeps(cfg)
	if err != nil {
		t.Fatalf("wireDeps() error = %v, want nil", err)
	}
	if app == nil {
		t.Fatal("wireDeps() app = nil, want non-nil")
	}
	if app.db == nil {
		t.Error("wireDeps() app.db = nil")
	}
	if app.logger == nil {
		t.Error("wireDeps() app.logger = nil")
	}
	if app.cfg != cfg {
		t.Error("wireDeps() app.cfg mismatch")
	}

	// Clean up.
	sqlDB, dbErr := app.db.DB()
	if dbErr == nil {
		_ = sqlDB.Close()
	}
	_ = app.logger.Close()
}

func TestWireDeps_SetupLoggerError(t *testing.T) {
	tmpDir := t.TempDir()
	blocker := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("create blocker file: %v", err)
	}

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
			Level:    "info",
			Format:   "text",
			FilePath: filepath.Join(blocker, "subdir", "app.log"),
		},
	}

	app, err := wireDeps(cfg)
	if err == nil {
		t.Fatal("wireDeps() error = nil, want error")
	}
	if app != nil {
		t.Fatalf("wireDeps() app = %v, want nil", app)
	}
	if !strings.Contains(err.Error(), "setup logger") {
		t.Fatalf("wireDeps() error = %q, want contains %q", err.Error(), "setup logger")
	}
}

func TestWireDeps_DatabaseError(t *testing.T) {
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

	app, err := wireDeps(cfg)
	if err == nil {
		t.Fatal("wireDeps() error = nil, want error")
	}
	if app != nil {
		t.Fatalf("wireDeps() app = %v, want nil", app)
	}
	if !strings.Contains(err.Error(), "setup database") {
		t.Fatalf("wireDeps() error = %q, want contains %q", err.Error(), "setup database")
	}
}

// --- Tests for nil guards and edge cases in the refactored structure ---

func TestNew_NilConfig(t *testing.T) {
	app, err := New(nil)
	if err == nil {
		t.Fatal("New(nil) error = nil, want error")
	}
	if app != nil {
		t.Fatalf("New(nil) app = %v, want nil", app)
	}
	if !strings.Contains(err.Error(), "config is nil") {
		t.Fatalf("New(nil) error = %q, want contains %q", err.Error(), "config is nil")
	}
}

func TestNew_InvalidGinMode(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 8080,
			Mode: "staging", // invalid
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
	if err == nil {
		t.Fatal("New() error = nil, want error for invalid gin mode")
	}
	if app != nil {
		t.Fatalf("New() app = %v, want nil", app)
	}
	if !strings.Contains(err.Error(), "invalid server.mode") {
		t.Fatalf("New() error = %q, want contains %q", err.Error(), "invalid server.mode")
	}
}

func TestNew_InvalidServerTimeout(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:    "127.0.0.1",
			Port:    8080,
			Mode:    gin.TestMode,
			Timeout: "not_a_duration",
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
	if err == nil {
		t.Fatal("New() error = nil, want error for invalid timeout")
	}
	if app != nil {
		t.Fatalf("New() app = %v, want nil", app)
	}
	if !strings.Contains(err.Error(), "parse server.timeout") {
		t.Fatalf("New() error = %q, want contains %q", err.Error(), "parse server.timeout")
	}
}

// --- htmlRecoveryHandler tests ---

func TestHtmlRecoveryHandler_JSON(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	c.Request.Header.Set("Accept", "application/json")

	htmlRecoveryHandler(c, "test panic")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	var resp pkg.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json decode error: %v", err)
	}
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("resp.Code = %d, want %d", resp.Code, http.StatusInternalServerError)
	}
	if resp.Message != "internal server error" {
		t.Fatalf("resp.Message = %q, want %q", resp.Message, "internal server error")
	}
}

func TestHtmlRecoveryHandler_HTML(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/page", nil)
	c.Request.Header.Set("Accept", "text/html")

	htmlRecoveryHandler(c, "test panic")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	// No HTML renderer configured so renderError falls back to plain text.
	body := w.Body.String()
	if !strings.Contains(body, "500") {
		t.Fatalf("body = %q, want contains %q", body, "500")
	}
}

// --- effectiveRateLimitRPS edge cases ---

func TestEffectiveRateLimitRPS_ZeroAndNegative(t *testing.T) {
	tests := []struct {
		name string
		rps  float64
		want int
	}{
		{name: "zero clamps to one", rps: 0, want: 1},
		{name: "negative clamps to one", rps: -5, want: 1},
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

// --- resolveCSRFSecret unit tests ---

func TestResolveCSRFSecret_PlaceholderGeneratesRandom(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			CSRFSecret: "change-me-to-a-random-secret",
		},
	}
	log := logger.Default()

	secret, err := resolveCSRFSecret(cfg, log)
	if err != nil {
		t.Fatalf("resolveCSRFSecret() error = %v", err)
	}
	if secret == cfg.Server.CSRFSecret {
		t.Fatal("resolveCSRFSecret() should replace placeholder with a random secret")
	}
	if len(secret) < 32 {
		t.Fatalf("resolveCSRFSecret() generated secret len = %d, want >= 32", len(secret))
	}
}

func TestResolveCSRFSecret_ExplicitSecretPassedThrough(t *testing.T) {
	explicit := "Abcd1234!Abcd1234!Abcd1234!Abcd1234!"
	cfg := &config.Config{
		Server: config.ServerConfig{
			CSRFSecret: explicit,
		},
	}
	log := logger.Default()

	secret, err := resolveCSRFSecret(cfg, log)
	if err != nil {
		t.Fatalf("resolveCSRFSecret() error = %v", err)
	}
	if secret != explicit {
		t.Fatalf("resolveCSRFSecret() = %q, want %q", secret, explicit)
	}
}

// --- buildMiddlewareChain unit tests ---

func TestBuildMiddlewareChain_InvalidTimeout(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Mode:    gin.TestMode,
			Timeout: "invalid",
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "text",
		},
	}

	chain, cacheInstance, err := buildMiddlewareChain(cfg)
	if err == nil {
		t.Fatal("buildMiddlewareChain() error = nil, want error for invalid timeout")
	}
	if chain != nil {
		t.Fatal("buildMiddlewareChain() chain should be nil on error")
	}
	if cacheInstance != nil {
		t.Fatal("buildMiddlewareChain() cacheInstance should be nil on error")
	}
	if !strings.Contains(err.Error(), "parse server.timeout") {
		t.Fatalf("buildMiddlewareChain() error = %q, want contains %q", err.Error(), "parse server.timeout")
	}
}

func TestBuildMiddlewareChain_DefaultTimeout(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Mode: gin.TestMode,
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "text",
		},
	}

	chain, _, err := buildMiddlewareChain(cfg)
	if err != nil {
		t.Fatalf("buildMiddlewareChain() error = %v", err)
	}
	if chain == nil {
		t.Fatal("buildMiddlewareChain() chain = nil")
	}
}

// --- setupAuth unit tests ---

func TestSetupAuth_Disabled(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{Enabled: false},
	}
	chain := ginx.NewChain()

	modules, jwtSvc, rbacSvc, err := setupAuth(cfg, nil, nil, chain, logger.Default(), "")
	if err != nil {
		t.Fatalf("setupAuth() error = %v", err)
	}
	if modules != nil {
		t.Errorf("setupAuth() modules = %v, want nil when disabled", modules)
	}
	if jwtSvc != nil {
		t.Errorf("setupAuth() jwtSvc = %v, want nil when disabled", jwtSvc)
	}
	if rbacSvc != nil {
		t.Errorf("setupAuth() rbacSvc = %v, want nil when disabled", rbacSvc)
	}
}

func TestSetupAuth_InvalidTokenExpiry(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			Enabled:     true,
			JWTSecret:   "test-secret-key-must-be-at-least-32-chars-long!",
			TokenExpiry: "not_a_duration",
		},
	}
	chain := ginx.NewChain()

	_, _, _, err := setupAuth(cfg, nil, nil, chain, logger.Default(), "")
	if err == nil {
		t.Fatal("setupAuth() error = nil, want error for invalid token expiry")
	}
	if !strings.Contains(err.Error(), "parse auth.token_expiry") {
		t.Fatalf("setupAuth() error = %q, want contains %q", err.Error(), "parse auth.token_expiry")
	}
}
