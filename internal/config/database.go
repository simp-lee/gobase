package config

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// SetupDatabase initializes a GORM database connection based on the provided
// DatabaseConfig. It supports "sqlite" and "postgres" drivers, configures the
// GORM logger mode based on the slog level, and sets connection pool parameters.
func SetupDatabase(cfg *DatabaseConfig, logger *slog.Logger) (*gorm.DB, error) {
	if cfg == nil {
		return nil, errors.New("database config is nil")
	}
	if logger == nil {
		return nil, errors.New("logger is nil")
	}

	var dialector gorm.Dialector

	switch cfg.Driver {
	case "sqlite":
		dir := filepath.Dir(cfg.SQLite.Path)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("failed to create sqlite directory %q: %w", dir, err)
			}
		}
		dialector = sqlite.Open(cfg.SQLite.Path)
	case "postgres":
		dsn := buildPostgresDSN(&cfg.Postgres)
		dialector = postgres.Open(dsn)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}

	// Determine GORM log level based on slog level.
	// Debug mode → Info (logs all SQL); otherwise → Warn (slow SQL and errors only).
	logMode := gormlogger.Warn
	if logger.Enabled(context.Background(), slog.LevelDebug) {
		logMode = gormlogger.Info
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: gormlogger.Default.LogMode(logMode),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// ★ M2: Configure connection pool.
	if err := configurePool(db, &cfg.Pool); err != nil {
		// Close the already-opened connection before returning.
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			sqlDB.Close()
		}
		return nil, err
	}

	logger.Info("database connected",
		slog.String("driver", cfg.Driver),
		slog.Int("max_idle_conns", effectiveMaxIdleConns(cfg.Pool.MaxIdleConns)),
		slog.Int("max_open_conns", effectiveMaxOpenConns(cfg.Pool.MaxOpenConns)),
		slog.String("conn_max_lifetime", effectiveConnMaxLifetime(cfg.Pool.ConnMaxLifetime)),
	)

	return db, nil
}

// configurePool sets connection pool parameters on the underlying sql.DB.
// Zero/empty values are replaced with sensible defaults.
func configurePool(db *gorm.DB, pool *PoolConfig) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxIdleConns(effectiveMaxIdleConns(pool.MaxIdleConns))
	sqlDB.SetMaxOpenConns(effectiveMaxOpenConns(pool.MaxOpenConns))

	lifetime, err := time.ParseDuration(effectiveConnMaxLifetime(pool.ConnMaxLifetime))
	if err != nil {
		return fmt.Errorf("invalid pool.conn_max_lifetime %q: %w", pool.ConnMaxLifetime, err)
	}
	sqlDB.SetConnMaxLifetime(lifetime)

	return nil
}

func effectiveMaxIdleConns(v int) int {
	if v <= 0 {
		return 10
	}
	return v
}

func effectiveMaxOpenConns(v int) int {
	if v <= 0 {
		return 100
	}
	return v
}

func effectiveConnMaxLifetime(v string) string {
	if v == "" {
		return "1h"
	}
	return v
}

func buildPostgresDSN(cfg *PostgresConfig) string {
	if cfg == nil {
		return ""
	}

	u := &url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Path:   cfg.DBName,
	}

	if cfg.User != "" || cfg.Password != "" {
		u.User = url.UserPassword(cfg.User, cfg.Password)
	}

	query := url.Values{}
	if cfg.SSLMode != "" {
		query.Set("sslmode", cfg.SSLMode)
	}
	u.RawQuery = query.Encode()

	return u.String()
}
