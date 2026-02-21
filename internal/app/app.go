package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	cache "github.com/simp-lee/cache"
	"github.com/simp-lee/ginx"
	"github.com/simp-lee/jwt"
	"github.com/simp-lee/logger"
	"github.com/simp-lee/rbac"
	"gorm.io/gorm"

	"github.com/simp-lee/gobase/internal/config"
	"github.com/simp-lee/gobase/internal/domain"
	"github.com/simp-lee/gobase/internal/module/auth"
	"github.com/simp-lee/gobase/internal/module/user"
	"github.com/simp-lee/gobase/internal/pkg"
	"github.com/simp-lee/gobase/web"
)

// App holds the core application dependencies and the HTTP server.
type App struct {
	engine      *gin.Engine
	db          *gorm.DB
	logger      *logger.Logger
	cfg         *config.Config
	cache       cache.CacheInterface
	jwtService  jwt.Service
	rbacService rbac.Service
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
	userModule := user.NewModule(handler, pageHandler)
	modules := []Module{userModule}

	var jwtSvc jwt.Service
	var rbacSvc rbac.Service

	// 5. Create Gin engine with custom middleware (not gin.Default()).
	if err := validateGinMode(cfg.Server.Mode); err != nil {
		return nil, err
	}
	gin.SetMode(cfg.Server.Mode)
	engine := gin.New()

	// Build shared logger options for ginx middlewares.
	loggerOpts := config.BuildLoggerOpts(&cfg.Log)

	// Build CORS options from application settings.
	corsOpts := resolveCORSOptions(cfg.Server.Mode, &cfg.Server.CORS)

	// Parse timeout duration.
	timeoutDuration := 30 * time.Second
	serverTimeout := strings.TrimSpace(cfg.Server.Timeout)
	if serverTimeout != "" {
		parsed, err := time.ParseDuration(serverTimeout)
		if err != nil {
			return nil, fmt.Errorf("parse server.timeout %q: %w", cfg.Server.Timeout, err)
		}
		timeoutDuration = parsed
	}

	// Build ginx middleware chain.
	chain := ginx.NewChain().
		WithErrorFormat(func(status int, message string) any {
			return pkg.Response{Code: status, Message: message}
		}).
		Use(ginx.RecoveryWith(htmlRecoveryHandler, loggerOpts...)).
		Use(ginx.RequestID(
			ginx.WithIgnoreIncoming(),
			ginx.WithContextInjector(func(ctx context.Context, requestID string) context.Context {
				return logger.WithContextAttrs(ctx, slog.String("request_id", requestID))
			}),
		)).
		Use(ginx.Logger(loggerOpts...)).
		Use(ginx.CORS(corsOpts...)).
		Use(ginx.Timeout(ginx.WithTimeout(timeoutDuration)))

	// Conditionally add rate limiting for /api routes.
	// /health lives at root level, so PathHasPrefix("/api") already excludes it.
	if cfg.Server.RateLimit.Enabled {
		rps := effectiveRateLimitRPS(cfg.Server.RateLimit.RPS)
		chain.When(
			ginx.PathHasPrefix("/api"),
			ginx.RateLimit(rps, cfg.Server.RateLimit.Burst),
		)
	}

	// Conditionally add response caching for GET /api/* requests.
	// Cache is disabled by default (controlled by server.cache config).
	// ginx.Cache auto-skips requests with Authorization/Cookie headers.
	var cacheInstance cache.CacheInterface
	if cfg.Server.Cache.Enabled {
		// already validated by config.Validate()
		ttl, _ := time.ParseDuration(cfg.Server.Cache.TTL)
		cacheInstance = cache.NewCache(cache.Options{
			DefaultExpiration: ttl,
			CleanupInterval:   ttl * 2,
			MaxSize:           cfg.Server.Cache.MaxSize,
		})
		chain.When(
			ginx.And(ginx.PathHasPrefix("/api"), ginx.MethodIs("GET")),
			ginx.Cache(cacheInstance),
		)
	}

	// Conditionally assemble Auth + RBAC when auth is enabled.
	if cfg.Auth.Enabled {
		// Parse token expiry duration.
		tokenExpiry, err := time.ParseDuration(cfg.Auth.TokenExpiry)
		if err != nil {
			return nil, fmt.Errorf("parse auth.token_expiry %q: %w", cfg.Auth.TokenExpiry, err)
		}

		// Create jwt.Service.
		jwtSvc, err = jwt.New(cfg.Auth.JWTSecret)
		if err != nil {
			return nil, fmt.Errorf("create jwt service: %w", err)
		}
		defer func() {
			if !success {
				jwtSvc.Close()
			}
		}()

		// Optional: create rbac.Service.
		if cfg.Auth.RBAC.Enabled {
			sqlDB, err := db.DB()
			if err != nil {
				return nil, fmt.Errorf("get sql.DB for rbac: %w", err)
			}

			roleTTL, _ := time.ParseDuration(cfg.Auth.RBAC.Cache.RoleTTL)
			userRoleTTL, _ := time.ParseDuration(cfg.Auth.RBAC.Cache.UserRoleTTL)
			permissionTTL, _ := time.ParseDuration(cfg.Auth.RBAC.Cache.PermissionTTL)

			rbacSvc, err = rbac.New(rbac.WithCachedStorage(sqlDB, &rbac.CacheConfig{
				RoleTTL:      roleTTL,
				UserRoleTTL:  userRoleTTL,
				PermTTL:      permissionTTL,
				MaxRoles:     cfg.Auth.RBAC.Cache.MaxRoleEntries,
				MaxUserRoles: cfg.Auth.RBAC.Cache.MaxUserEntries,
				MaxUserPerms: cfg.Auth.RBAC.Cache.MaxPermissionEntries,
			}))
			if err != nil {
				return nil, fmt.Errorf("create rbac service: %w", err)
			}
			defer func() {
				if !success {
					if err := rbacSvc.Close(); err != nil {
						slog.Error("rbac service close error during init rollback", slog.Any("error", err))
					}
				}
			}()
			log.Info("RBAC service initialized")
		}

		// Create auth module.
		authSvc := auth.NewService(jwtSvc, repo, tokenExpiry)
		authHandler := auth.NewHandler(authSvc)
		authModule := auth.NewModule(authHandler)
		modules = append(modules, authModule)

		// Add Auth middleware (exclude public paths).
		// RBAC permission checks are already wired for users routes below.
		// Extend the same pattern to additional resource route groups as needed.
		// See: ginx.RequirePermission, ginx.RequireRolePermission
		chain.When(
			ginx.And(
				ginx.PathHasPrefix("/api"),
				ginx.Not(ginx.PathIs(cfg.Auth.PublicPaths...)),
			),
			ginx.Auth(jwtSvc),
		)

		if cfg.Auth.RBAC.Enabled {
			usersPath := ginx.PathHasPrefix("/api/v1/users")

			chain.When(
				ginx.And(usersPath, ginx.MethodIs(http.MethodGet)),
				ginx.RequirePermission(rbacSvc, "users", "read"),
			)
			chain.When(
				ginx.And(usersPath, ginx.MethodIs(http.MethodPost)),
				ginx.RequirePermission(rbacSvc, "users", "create"),
			)
			chain.When(
				ginx.And(usersPath, ginx.MethodIs(http.MethodPut)),
				ginx.RequirePermission(rbacSvc, "users", "update"),
			)
			chain.When(
				ginx.And(usersPath, ginx.MethodIs(http.MethodDelete)),
				ginx.RequirePermission(rbacSvc, "users", "delete"),
			)
		}
	}

	// OnError fires only when a handler or middleware calls c.Error().
	// Timeout, RateLimit, and Recovery have self-contained responses and
	// never call c.Error(), so this handler is not involved in those paths.
	chain.OnError(func(c *gin.Context, err error) {
		renderError(c, 500, "internal server error")
	})

	engine.Use(chain.Build())

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

	if cfg.Server.Mode == gin.ReleaseMode {
		if err := validateReleaseCSRFSecret(csrfSecret); err != nil {
			return nil, err
		}
	}

	// 8. Register all routes.
	if err := RegisterRoutes(engine, &RouteDeps{
		Modules:    modules,
		DB:         db,
		Mode:       cfg.Server.Mode,
		CSRFSecret: csrfSecret,
	}); err != nil {
		return nil, fmt.Errorf("register routes: %w", err)
	}

	success = true
	return &App{
		engine:      engine,
		db:          db,
		logger:      log,
		cfg:         cfg,
		cache:       cacheInstance,
		jwtService:  jwtSvc,
		rbacService: rbacSvc,
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

func effectiveRateLimitRPS(rps float64) int {
	effective := int(math.Ceil(rps))
	if effective < 1 {
		return 1
	}
	return effective
}

func validateReleaseCSRFSecret(secret string) error {
	trimmed := strings.TrimSpace(secret)
	if len(trimmed) < 32 {
		return errors.New("csrf_secret must be at least 32 characters in release mode")
	}

	if config.CountSecretClasses(trimmed) < 3 {
		return errors.New("csrf_secret must include at least 3 character classes (lowercase, uppercase, digit, symbol) in release mode")
	}

	return nil
}

func resolveCORSOptions(mode string, corsCfg *config.CORSConfig) []ginx.Option[ginx.CORSConfig] {
	var opts []ginx.Option[ginx.CORSConfig]

	// Handle AllowOrigins.
	if len(corsCfg.AllowOrigins) > 0 {
		opts = append(opts, ginx.WithAllowOrigins(corsCfg.AllowOrigins...))
	} else if mode != gin.ReleaseMode {
		// In non-release mode with no configured origins, default to permissive.
		opts = append(opts, ginx.WithAllowOrigins("*"))
	}
	// In release mode with no configured origins, don't add WithAllowOrigins — ginx defaults to deny all.

	// Apply optional CORS settings from config.
	if len(corsCfg.AllowMethods) > 0 {
		opts = append(opts, ginx.WithAllowMethods(corsCfg.AllowMethods...))
	}
	if len(corsCfg.AllowHeaders) > 0 {
		opts = append(opts, ginx.WithAllowHeaders(corsCfg.AllowHeaders...))
	}
	if corsCfg.AllowCredentials {
		opts = append(opts, ginx.WithAllowCredentials(true))
	}
	if corsCfg.MaxAge != "" {
		d, err := time.ParseDuration(corsCfg.MaxAge)
		if err != nil {
			slog.Warn("ignoring invalid cors.max_age", slog.String("value", corsCfg.MaxAge), slog.Any("error", err))
		} else if d > 0 {
			opts = append(opts, ginx.WithMaxAge(d))
		}
	}

	return opts
}

// htmlRecoveryHandler is the custom panic handler for ginx.RecoveryWith.
// It renders an HTML error page for browser requests and a JSON response for API clients.
func htmlRecoveryHandler(c *gin.Context, err any) {
	renderError(c, 500, "internal server error")
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

	// Clean up rate limiter stores.
	ginx.CleanupRateLimiters()

	// Clean up cache instance.
	if a.cache != nil {
		a.cache.Close()
	}

	// Close JWT service (stops background cleanup goroutine).
	if a.jwtService != nil {
		a.jwtService.Close()
	}

	// Close RBAC service.
	if a.rbacService != nil {
		if err := a.rbacService.Close(); err != nil {
			if a.logger != nil {
				a.logger.Error("rbac service close error", slog.Any("error", err))
			} else {
				slog.Error("rbac service close error", slog.Any("error", err))
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
