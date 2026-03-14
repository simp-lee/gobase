package app

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"
	cache "github.com/simp-lee/cache"
	"github.com/simp-lee/jwt"
	"github.com/simp-lee/logger"
	"github.com/simp-lee/rbac"
	"gorm.io/gorm"

	"github.com/simp-lee/gobase/internal/config"
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

// New creates and wires a fully configured App from the given Config.
//
// It sets up logging, database, domain repositories, services, handlers,
// middleware, template rendering, and routes.
func New(cfg *config.Config) (*App, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}

	// 1. Initialise core infrastructure: logger, database, auto-migration.
	app, err := wireDeps(cfg)
	if err != nil {
		return nil, err
	}

	success := false
	defer func() {
		if success {
			return
		}
		if err := app.logger.Close(); err != nil {
			slog.Error("logger close error", slog.Any("error", err))
		}
	}()
	defer func() {
		if success {
			return
		}
		sqlDB, err := app.db.DB()
		if err != nil {
			return
		}
		if err := sqlDB.Close(); err != nil {
			slog.Error("database close error", slog.Any("error", err))
		}
	}()

	if err := ensureSeedAdmin(cfg, app.db, app.logger); err != nil {
		return nil, err
	}

	// 2. Wire business modules (repository → service → handler → module).
	modules, userRepo := wireModules(app.db)

	// 3. Create Gin engine with custom middleware (not gin.Default()).
	if err := validateGinMode(cfg.Server.Mode); err != nil {
		return nil, err
	}
	gin.SetMode(cfg.Server.Mode)
	engine := gin.New()

	// 3a. Build global middleware chain (recovery, CORS, timeout, rate-limit, cache).
	chain, cacheInstance, err := buildMiddlewareChain(cfg)
	if err != nil {
		return nil, err
	}

	// 4. Resolve CSRF secret.
	csrfSecret, err := resolveCSRFSecret(cfg, app.logger)
	if err != nil {
		return nil, err
	}

	// 3b. Conditionally assemble Auth + RBAC when auth is enabled.
	authModules, jwtSvc, rbacSvc, err := setupAuth(cfg, app.db, userRepo, chain, app.logger, csrfSecret)
	if err != nil {
		return nil, err
	}
	modules = append(modules, authModules...)
	// setupAuth succeeded → we own jwtSvc/rbacSvc → clean up if a later step fails.
	defer func() {
		if !success {
			if jwtSvc != nil {
				jwtSvc.Close()
			}
			if rbacSvc != nil {
				if err := rbacSvc.Close(); err != nil {
					slog.Error("rbac service close error during init rollback", slog.Any("error", err))
				}
			}
		}
	}()

	// OnError fires only when a handler or middleware calls c.Error().
	// Timeout, RateLimit, and Recovery have self-contained responses and
	// never call c.Error(), so this handler is not involved in those paths.
	chain.OnError(func(c *gin.Context, err error) {
		renderError(c, 500, "internal server error")
	})

	engine.Use(chain.Build())

	// 4. Set up template renderer.
	if err := setupTemplateRenderer(engine, cfg.Server.Mode); err != nil {
		return nil, err
	}

	// 6. Register all routes.
	if err := RegisterRoutes(engine, &RouteDeps{
		Modules:    modules,
		DB:         app.db,
		Mode:       cfg.Server.Mode,
		CSRFSecret: csrfSecret,
	}); err != nil {
		return nil, fmt.Errorf("register routes: %w", err)
	}

	app.engine = engine
	app.cache = cacheInstance
	app.jwtService = jwtSvc
	app.rbacService = rbacSvc

	success = true
	return app, nil
}
