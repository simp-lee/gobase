package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/simp-lee/ginx"

	"github.com/simp-lee/gobase/internal/config"
	"github.com/simp-lee/gobase/internal/domain"
)

// migrateModels returns all domain models for AutoMigrate.
// Shared by New() (debug mode) and Migrate() (production).
// Returns a fresh slice each call to prevent accidental mutation.
func migrateModels() []any {
	return []any{
		&domain.User{},
	}
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

// Migrate sets up the database connection, runs AutoMigrate for all domain
// models, and then closes the connection. It is intended for production
// deployments where the server runs in release mode and does not auto-migrate
// on startup.
func Migrate(cfg *config.Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}

	log, err := config.SetupLogger(&cfg.Log)
	if err != nil {
		return fmt.Errorf("setup logger: %w", err)
	}
	defer func() {
		if err := log.Close(); err != nil {
			slog.Error("logger close error", slog.Any("error", err))
		}
	}()

	db, err := config.SetupDatabase(&cfg.Database, log.Logger)
	if err != nil {
		return fmt.Errorf("setup database: %w", err)
	}
	defer func() {
		sqlDB, dbErr := db.DB()
		if dbErr != nil {
			return
		}
		if err := sqlDB.Close(); err != nil {
			slog.Error("database close error", slog.Any("error", err))
		}
	}()

	if err := db.AutoMigrate(migrateModels()...); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}

	log.Info("migration completed")
	return nil
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

	if a.logger != nil {
		a.logger.Info("server stopped")
	} else {
		slog.Info("server stopped")
	}

	a.close()

	return runErr
}

// close releases all resources owned by the App in reverse-init order:
// rate-limiter → cache → jwt → rbac → db → logger.
func (a *App) close() {
	ginx.CleanupRateLimiters()

	if a.cache != nil {
		a.cache.Close()
	}

	if a.jwtService != nil {
		a.jwtService.Close()
	}

	if a.rbacService != nil {
		if err := a.rbacService.Close(); err != nil {
			if a.logger != nil {
				a.logger.Error("rbac service close error", slog.Any("error", err))
			} else {
				slog.Error("rbac service close error", slog.Any("error", err))
			}
		}
	}

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
		if err := a.logger.Close(); err != nil {
			slog.Error("logger close error", slog.Any("error", err))
		}
	}
}
