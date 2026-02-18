package app

import (
	"context"
	"errors"
	"net/http"
	"os"
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

func TestResolveCORSConfig(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		configured []string
		want       []string
	}{
		{
			name: "debug mode uses permissive default when not configured",
			mode: gin.DebugMode,
			want: []string{"*"},
		},
		{
			name: "release mode denies cross-origin when not configured",
			mode: gin.ReleaseMode,
			want: []string{},
		},
		{
			name:       "release mode uses explicit allowlist",
			mode:       gin.ReleaseMode,
			configured: []string{"https://admin.example.com"},
			want:       []string{"https://admin.example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := resolveCORSConfig(tt.mode, tt.configured)
			if len(cfg.AllowOrigins) != len(tt.want) {
				t.Fatalf("AllowOrigins length = %d, want %d", len(cfg.AllowOrigins), len(tt.want))
			}
			for i := range tt.want {
				if cfg.AllowOrigins[i] != tt.want[i] {
					t.Fatalf("AllowOrigins[%d] = %q, want %q", i, cfg.AllowOrigins[i], tt.want[i])
				}
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
