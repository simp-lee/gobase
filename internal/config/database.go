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
	"strings"
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
		if cfg.Driver == "postgres" {
			return nil, sanitizePostgresConnectError(&cfg.Postgres, err)
		}
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// ★ M2: Configure connection pool.
	if err := configurePool(db, &cfg.Pool); err != nil {
		// Close the already-opened connection before returning.
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		return nil, err
	}

	logger.Info("database connected",
		slog.String("driver", cfg.Driver),
		slog.Int("max_idle_conns", effectiveMaxIdleConns(cfg.Pool.MaxIdleConns)),
		slog.Int("max_open_conns", effectiveMaxOpenConns(cfg.Pool.MaxOpenConns)),
		slog.String("conn_max_lifetime", effectiveConnMaxLifetime(cfg.Pool.ConnMaxLifetime)),
		slog.String("conn_max_idle_time", effectiveConnMaxIdleTime(cfg.Pool.ConnMaxIdleTime)),
	)

	return db, nil
}

// configurePool sets connection pool parameters on the underlying sql.DB.
// Zero/empty values are replaced with sensible defaults.
// Negative values for connection counts are rejected to fail fast.
func configurePool(db *gorm.DB, pool *PoolConfig) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	if pool.MaxIdleConns < 0 {
		return fmt.Errorf("invalid pool.max_idle_conns %d: must be >= 0", pool.MaxIdleConns)
	}
	if pool.MaxOpenConns < 0 {
		return fmt.Errorf("invalid pool.max_open_conns %d: must be >= 0", pool.MaxOpenConns)
	}

	sqlDB.SetMaxIdleConns(effectiveMaxIdleConns(pool.MaxIdleConns))
	sqlDB.SetMaxOpenConns(effectiveMaxOpenConns(pool.MaxOpenConns))

	lifetime, err := time.ParseDuration(effectiveConnMaxLifetime(pool.ConnMaxLifetime))
	if err != nil {
		return fmt.Errorf("invalid pool.conn_max_lifetime %q: %w", pool.ConnMaxLifetime, err)
	}
	if lifetime <= 0 {
		return fmt.Errorf("invalid pool.conn_max_lifetime %q: must be greater than 0", pool.ConnMaxLifetime)
	}
	sqlDB.SetConnMaxLifetime(lifetime)

	idleTime, err := time.ParseDuration(effectiveConnMaxIdleTime(pool.ConnMaxIdleTime))
	if err != nil {
		return fmt.Errorf("invalid pool.conn_max_idle_time %q: %w", pool.ConnMaxIdleTime, err)
	}
	if idleTime <= 0 {
		return fmt.Errorf("invalid pool.conn_max_idle_time %q: must be greater than 0", pool.ConnMaxIdleTime)
	}
	sqlDB.SetConnMaxIdleTime(idleTime)

	return nil
}

func effectiveMaxIdleConns(v int) int {
	if v == 0 {
		return 10
	}
	return v
}

func effectiveMaxOpenConns(v int) int {
	if v == 0 {
		return 100
	}
	return v
}

func effectiveConnMaxLifetime(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "1h"
	}
	return v
}

func effectiveConnMaxIdleTime(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "10m"
	}
	return v
}

// sanitizePostgresConnectError redacts the password from the error message.
// NOTE: We intentionally use %s instead of %w to avoid wrapping the original
// error. The underlying error's chain may contain the raw password in nested
// error messages, and wrapping with %w would allow callers to extract it via
// errors.Is / errors.As / Unwrap. Callers who need programmatic error
// inspection should check the returned error's message string instead.
func sanitizePostgresConnectError(cfg *PostgresConfig, err error) error {
	if err == nil {
		return nil
	}
	if cfg == nil {
		return fmt.Errorf("failed to connect to postgres database: %s", err.Error())
	}

	msg := err.Error()
	redactionNote := ""
	if cfg.Password != "" {
		msg = strings.ReplaceAll(msg, cfg.Password, "[REDACTED]")
		msg = strings.ReplaceAll(msg, url.QueryEscape(cfg.Password), "[REDACTED]")
		redactionNote = " password=[REDACTED]"
	}

	return fmt.Errorf(
		"failed to connect to postgres database (host=%s port=%d dbname=%s sslmode=%s%s): %s",
		cfg.Host,
		cfg.Port,
		cfg.DBName,
		cfg.SSLMode,
		redactionNote,
		msg,
	)
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
