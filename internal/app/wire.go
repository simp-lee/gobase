package app

import (
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/simp-lee/gobase/internal/config"
	"github.com/simp-lee/gobase/internal/domain"
	"github.com/simp-lee/gobase/internal/module/user"
)

// wireDeps initialises the core infrastructure dependencies: logger, database
// connection, and auto-migration (debug mode only).
//
// On success the caller owns the returned resources and must close them on
// failure (the standard pattern is a success-guarded defer in New).
// On failure all partially-created resources are cleaned up internally.
func wireDeps(cfg *config.Config) (*App, error) {
	// 1. Setup logger.
	log, err := config.SetupLogger(&cfg.Log)
	if err != nil {
		return nil, fmt.Errorf("setup logger: %w", err)
	}

	if cfg.Server.Mode == gin.DebugMode && cfg.Server.Host == "0.0.0.0" {
		log.Warn("insecure server config: debug mode on 0.0.0.0 may expose debug behavior and permissive CORS")
	}

	// 2. Setup database (includes connection pool configuration).
	db, err := config.SetupDatabase(&cfg.Database, log.Logger)
	if err != nil {
		if closeErr := log.Close(); closeErr != nil {
			slog.Error("logger close error", slog.Any("error", closeErr))
		}
		return nil, fmt.Errorf("setup database: %w", err)
	}

	// 3. AutoMigrate in debug mode only.
	if cfg.Server.Mode == "debug" {
		if err := db.AutoMigrate(migrateModels()...); err != nil {
			sqlDB, dbErr := db.DB()
			if dbErr == nil {
				if closeErr := sqlDB.Close(); closeErr != nil {
					slog.Error("database close error", slog.Any("error", closeErr))
				}
			}
			if closeErr := log.Close(); closeErr != nil {
				slog.Error("logger close error", slog.Any("error", closeErr))
			}
			return nil, fmt.Errorf("auto migrate: %w", err)
		}
		log.Info("auto migration completed")
	}

	return &App{db: db, logger: log, cfg: cfg}, nil
}

func validateGinMode(mode string) error {
	switch mode {
	case gin.DebugMode, gin.ReleaseMode, gin.TestMode:
		return nil
	default:
		return fmt.Errorf("invalid server.mode %q: must be one of %q, %q, %q", mode, gin.DebugMode, gin.ReleaseMode, gin.TestMode)
	}
}

// wireModules creates all business modules with their dependency chains
// (repository → service → handler → module) and returns them as a slice.
//
// It also returns the user repository, which is required by setupAuth to
// look up users during JWT validation.
//
// NOTE: The auth module is intentionally NOT wired here. It is assembled in
// setupAuth() (auth.go) and merged with these business modules in New(),
// because auth wiring is conditional on cfg.Auth.Enabled and interleaves
// with middleware chain configuration.
func wireModules(db *gorm.DB) ([]Module, domain.UserRepository) {
	repo := user.NewUserRepository(db)
	svc := user.NewUserService(repo)
	handler := user.NewUserHandler(svc)
	pageHandler := user.NewUserPageHandler(svc)
	userModule := user.NewModule(handler, pageHandler)

	return []Module{userModule}, repo
}
