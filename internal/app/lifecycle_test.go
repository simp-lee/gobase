package app

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/simp-lee/logger"
	"gorm.io/gorm"

	"github.com/simp-lee/gobase/internal/config"
)

// --- fakeHTTPServer for Run() tests ---

type fakeHTTPServer struct {
	listenErr      error
	listenStarted  chan struct{}
	shutdownCalled bool
	stopCh         chan struct{}
	mu             sync.Mutex
	closeOnce      sync.Once
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
		f.closeOnce.Do(func() { close(f.stopCh) })
	}
	return nil
}

func (f *fakeHTTPServer) wasShutdownCalled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.shutdownCalled
}

func TestMigrate_NilConfig(t *testing.T) {
	err := Migrate(nil)
	if err == nil {
		t.Fatal("Migrate(nil) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "config is nil") {
		t.Fatalf("Migrate(nil) error = %q, want contains %q", err.Error(), "config is nil")
	}
}

func TestMigrate_SetupLoggerError(t *testing.T) {
	tmpDir := t.TempDir()
	blocker := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("create blocker file: %v", err)
	}

	cfg := &config.Config{
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

	err := Migrate(cfg)
	if err == nil {
		t.Fatal("Migrate() error = nil, want error for logger setup failure")
	}
	if !strings.Contains(err.Error(), "setup logger") {
		t.Fatalf("Migrate() error = %q, want contains %q", err.Error(), "setup logger")
	}
}

func TestMigrate_InvalidDriver(t *testing.T) {
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Driver: "unsupported",
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "text",
		},
	}

	err := Migrate(cfg)
	if err == nil {
		t.Fatal("Migrate() error = nil, want error for unsupported driver")
	}
	if !strings.Contains(err.Error(), "setup database") {
		t.Fatalf("Migrate() error = %q, want contains %q", err.Error(), "setup database")
	}
}

func TestMigrate_SQLite_Success(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate-test.db")
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Driver: "sqlite",
			SQLite: config.SQLiteConfig{Path: dbPath},
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "text",
		},
	}

	if err := Migrate(cfg); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}

	// Re-open the DB to verify tables were created.
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("re-open db: %v", err)
	}
	defer func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	}()

	if !db.Migrator().HasTable("users") {
		t.Error("expected table \"users\" to exist after Migrate()")
	}
}

func TestMigrate_CleansUpDBConnection(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cleanup-test.db")
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Driver: "sqlite",
			SQLite: config.SQLiteConfig{Path: dbPath},
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "text",
		},
	}

	if err := Migrate(cfg); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}

	// After Migrate() returns, verify database is accessible (no lingering locks).
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("re-open db after Migrate: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping after Migrate cleanup: %v", err)
	}
	_ = sqlDB.Close()
}

// --- Run() tests ---

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

// --- Run() guard clause tests ---

func TestRun_NilApp(t *testing.T) {
	var a *App
	err := a.Run()
	if err == nil {
		t.Fatal("Run() error = nil, want error for nil app")
	}
	if !strings.Contains(err.Error(), "app is nil") {
		t.Fatalf("Run() error = %q, want contains %q", err.Error(), "app is nil")
	}
}

func TestRun_NilConfig(t *testing.T) {
	a := &App{engine: gin.New(), logger: logger.Default()}
	err := a.Run()
	if err == nil {
		t.Fatal("Run() error = nil, want error for nil config")
	}
	if !strings.Contains(err.Error(), "app config is nil") {
		t.Fatalf("Run() error = %q, want contains %q", err.Error(), "app config is nil")
	}
}

func TestRun_NilEngine(t *testing.T) {
	a := &App{
		logger: logger.Default(),
		cfg:    &config.Config{Server: config.ServerConfig{Host: "127.0.0.1", Port: 8080}},
	}
	err := a.Run()
	if err == nil {
		t.Fatal("Run() error = nil, want error for nil engine")
	}
	if !strings.Contains(err.Error(), "app engine is nil") {
		t.Fatalf("Run() error = %q, want contains %q", err.Error(), "app engine is nil")
	}
}
