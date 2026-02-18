package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/simp-lee/logger"
	"gorm.io/gorm"

	"github.com/simp-lee/gobase/internal/config"
	"github.com/simp-lee/gobase/internal/domain"
	"github.com/simp-lee/gobase/internal/middleware"
	"github.com/simp-lee/gobase/internal/module/user"
	"github.com/simp-lee/gobase/web"
)

// App holds the core application dependencies and the HTTP server.
type App struct {
	engine *gin.Engine
	db     *gorm.DB
	logger *logger.Logger
	cfg    *config.Config
}

type httpServer interface {
	ListenAndServe() error
	Shutdown(ctx context.Context) error
}

var newHTTPServer = func(addr string, handler http.Handler) httpServer {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

var notifyContext = func(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, signals...)
}

// New creates and wires a fully configured App from the given Config.
//
// It sets up logging, database, domain repositories, services, handlers,
// middleware, template rendering, and routes.
func New(cfg *config.Config) (*App, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}

	success := false

	// 1. Setup logger.
	log, err := config.SetupLogger(&cfg.Log)
	if err != nil {
		return nil, fmt.Errorf("setup logger: %w", err)
	}

	if cfg.Server.Mode == gin.DebugMode && cfg.Server.Host == "0.0.0.0" {
		log.Warn("insecure server config: debug mode on 0.0.0.0 may expose debug behavior and permissive CORS")
	}
	defer func() {
		if success {
			return
		}
		if err := log.Close(); err != nil {
			slog.Error("logger close error", slog.Any("error", err))
		}
	}()

	// 2. Setup database (includes M2 connection pool configuration).
	db, err := config.SetupDatabase(&cfg.Database, log.Logger)
	if err != nil {
		return nil, fmt.Errorf("setup database: %w", err)
	}
	defer func() {
		if success {
			return
		}
		sqlDB, err := db.DB()
		if err != nil {
			return
		}
		if err := sqlDB.Close(); err != nil {
			slog.Error("database close error", slog.Any("error", err))
		}
	}()

	// 3. AutoMigrate in debug mode only.
	if cfg.Server.Mode == "debug" {
		if err := db.AutoMigrate(&domain.User{}); err != nil {
			return nil, fmt.Errorf("auto migrate: %w", err)
		}
		log.Info("auto migration completed")
	}

	// 4. Manual dependency injection: repository → service → handler.
	repo := user.NewUserRepository(db)
	svc := user.NewUserService(repo)
	handler := user.NewUserHandler(svc)
	pageHandler := user.NewUserPageHandler(svc)

	// 5. Create Gin engine with custom middleware (not gin.Default()).
	if err := validateGinMode(cfg.Server.Mode); err != nil {
		return nil, err
	}
	gin.SetMode(cfg.Server.Mode)
	engine := gin.New()

	// Build CORS config from application settings.
	// In release mode, when no allowlist is configured, default to deny cross-origin requests.
	corsConfig := resolveCORSConfig(cfg.Server.Mode, cfg.Server.CORS.AllowOrigins)

	engine.Use(
		middleware.Recovery(log.Logger),
		middleware.RequestIDWithConfig(middleware.RequestIDConfig{
			TrustUpstream: false,
		}),
		middleware.Logger(log.Logger),
		middleware.CORSWithConfig(corsConfig),
	)

	// 6. Determine filesystem mode and set up template renderer.
	var fsys fs.FS
	if cfg.Server.Mode == "debug" {
		fsys, err = resolveDebugWebFS()
		if err != nil {
			return nil, fmt.Errorf("resolve debug template fs: %w", err)
		}
	} else {
		fsys = web.EmbeddedFS
	}

	renderer, err := NewTemplateRenderer(fsys, cfg.Server.Mode == "debug")
	if err != nil {
		return nil, fmt.Errorf("setup template renderer: %w", err)
	}
	engine.HTMLRender = renderer

	// 7. Resolve CSRF secret.
	csrfSecret := cfg.Server.CSRFSecret
	if isPlaceholderCSRFSecret(csrfSecret) {
		if cfg.Server.Mode == gin.ReleaseMode {
			return nil, errors.New("csrf_secret must be a non-placeholder value in release mode")
		}

		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return nil, fmt.Errorf("generate csrf secret: %w", err)
		}
		csrfSecret = hex.EncodeToString(b)
		log.Warn("no csrf_secret configured, using random secret in non-release mode (will change on restart)")
	}

	// 8. Register all routes.
	if err := RegisterRoutes(engine, &RouteDeps{
		UserHandler:     handler,
		UserPageHandler: pageHandler,
		DB:              db,
		Mode:            cfg.Server.Mode,
		CSRFSecret:      csrfSecret,
	}); err != nil {
		return nil, fmt.Errorf("register routes: %w", err)
	}

	success = true
	return &App{
		engine: engine,
		db:     db,
		logger: log,
		cfg:    cfg,
	}, nil
}

func isPlaceholderCSRFSecret(secret string) bool {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return true
	}

	switch strings.ToLower(trimmed) {
	case "change-me-to-a-random-secret", "change-me-in-env":
		return true
	default:
		return false
	}
}

func resolveCORSConfig(mode string, configuredAllowOrigins []string) middleware.CORSConfig {
	corsConfig := middleware.DefaultCORSConfig()

	if len(configuredAllowOrigins) > 0 {
		corsConfig.AllowOrigins = configuredAllowOrigins
		return corsConfig
	}

	if mode == gin.ReleaseMode {
		corsConfig.AllowOrigins = []string{}
	}

	return corsConfig
}

func validateGinMode(mode string) error {
	switch mode {
	case gin.DebugMode, gin.ReleaseMode, gin.TestMode:
		return nil
	default:
		return fmt.Errorf("invalid server.mode %q: must be one of %q, %q, %q", mode, gin.DebugMode, gin.ReleaseMode, gin.TestMode)
	}
}

func resolveDebugWebFS() (fs.FS, error) {
	if _, file, _, ok := runtime.Caller(0); ok {
		webDir := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "web"))
		if stat, err := os.Stat(webDir); err == nil && stat.IsDir() {
			return os.DirFS(webDir), nil
		}
	}

	exePath, err := os.Executable()
	if err == nil {
		webDir := filepath.Join(filepath.Dir(exePath), "web")
		if stat, err := os.Stat(webDir); err == nil && stat.IsDir() {
			return os.DirFS(webDir), nil
		}
	}

	return nil, errors.New("debug web directory not found")
}

// Run starts the HTTP server and blocks until a shutdown signal is received.
// It performs graceful shutdown with a 5-second timeout and closes the database
// connection (M2).
func (a *App) Run() error {
	if a == nil {
		return errors.New("app is nil")
	}
	if a.cfg == nil {
		return errors.New("app config is nil")
	}
	if a.engine == nil {
		return errors.New("app engine is nil")
	}

	addr := fmt.Sprintf("%s:%d", a.cfg.Server.Host, a.cfg.Server.Port)
	srv := newHTTPServer(addr, a.engine)

	// Listen for SIGINT / SIGTERM.
	ctx, stop := notifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start HTTP server in a goroutine.
	errCh := make(chan error, 1)
	go func() {
		if a.logger != nil {
			a.logger.Info("server started", slog.String("addr", addr))
		} else {
			slog.Info("server started", slog.String("addr", addr))
		}
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	var runErr error

	// Wait for shutdown signal or server error.
	select {
	case <-ctx.Done():
		if a.logger != nil {
			a.logger.Info("shutdown signal received")
		} else {
			slog.Info("shutdown signal received")
		}
	case err := <-errCh:
		runErr = fmt.Errorf("server error: %w", err)
	}

	if runErr == nil {
		// Graceful shutdown with 5-second deadline.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			if a.logger != nil {
				a.logger.Error("server shutdown error", slog.Any("error", err))
			} else {
				slog.Error("server shutdown error", slog.Any("error", err))
			}
		}
	}

	// ★ M2: Close database connection.
	if a.db != nil {
		if sqlDB, err := a.db.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				if a.logger != nil {
					a.logger.Error("database close error", slog.Any("error", err))
				} else {
					slog.Error("database close error", slog.Any("error", err))
				}
			} else {
				if a.logger != nil {
					a.logger.Info("database connection closed")
				} else {
					slog.Info("database connection closed")
				}
			}
		}
	}

	if a.logger != nil {
		a.logger.Info("server stopped")
		if err := a.logger.Close(); err != nil {
			slog.Error("logger close error", slog.Any("error", err))
		}
	} else {
		slog.Info("server stopped")
	}

	if runErr != nil {
		return runErr
	}

	return nil
}
