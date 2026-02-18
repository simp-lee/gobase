package config

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Config is the top-level application configuration.
type Config struct {
	Server   ServerConfig   `koanf:"server"`
	Database DatabaseConfig `koanf:"database"`
	Log      LogConfig      `koanf:"log"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host       string     `koanf:"host"`
	Port       int        `koanf:"port"`
	Mode       string     `koanf:"mode"`
	CSRFSecret string     `koanf:"csrf_secret"`
	CORS       CORSConfig `koanf:"cors"`
}

// CORSConfig holds CORS middleware settings.
type CORSConfig struct {
	AllowOrigins []string `koanf:"allow_origins"`
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	Driver   string         `koanf:"driver"`
	SQLite   SQLiteConfig   `koanf:"sqlite"`
	Postgres PostgresConfig `koanf:"postgres"`
	Pool     PoolConfig     `koanf:"pool"`
}

// SQLiteConfig holds SQLite-specific settings.
type SQLiteConfig struct {
	Path string `koanf:"path"`
}

// PostgresConfig holds PostgreSQL-specific settings.
type PostgresConfig struct {
	Host     string `koanf:"host"`
	Port     int    `koanf:"port"`
	User     string `koanf:"user"`
	Password string `koanf:"password"`
	DBName   string `koanf:"dbname"`
	SSLMode  string `koanf:"sslmode"`
}

// PoolConfig holds database connection pool settings.
type PoolConfig struct {
	MaxIdleConns    int    `koanf:"max_idle_conns"`
	MaxOpenConns    int    `koanf:"max_open_conns"`
	ConnMaxLifetime string `koanf:"conn_max_lifetime"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level           string `koanf:"level"`
	Format          string `koanf:"format"`
	Color           *bool  `koanf:"color"`
	FilePath        string `koanf:"file_path"`
	MaxSizeMB       int    `koanf:"max_size_mb"`
	RetentionDays   int    `koanf:"retention_days"`
	MaxBackups      int    `koanf:"max_backups"`
	CompressRotated *bool  `koanf:"compress_rotated"`
}

// Load reads configuration from a YAML file and overlays environment variables.
// Environment variables use the prefix "APP__" and double-underscore as the
// hierarchy separator. Single underscores are preserved as part of the key name.
// For example, APP__SERVER__PORT=9090 overrides server.port and
// APP__DATABASE__POOL__MAX_IDLE_CONNS=20 overrides database.pool.max_idle_conns.
func Load(configPath string) (*Config, error) {
	k := koanf.New(".")

	// Load YAML config file.
	if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("failed to load config file %s: %w", configPath, err)
	}

	// Overlay environment variables with prefix APP__.
	// APP__SERVER__PORT -> server.port
	// APP__DATABASE__POOL__MAX_IDLE_CONNS -> database.pool.max_idle_conns
	if err := k.Load(env.Provider("APP__", ".", func(s string) string {
		key := strings.TrimPrefix(s, "APP__")
		key = strings.ToLower(key)
		key = strings.ReplaceAll(key, "__", ".")
		return key
	}), nil); err != nil {
		return nil, fmt.Errorf("failed to load env variables: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Validate checks cross-field constraints and supported values.
func (c *Config) Validate() error {
	// Validate server.mode.
	mode := strings.TrimSpace(c.Server.Mode)
	switch mode {
	case gin.DebugMode, gin.ReleaseMode, gin.TestMode:
		c.Server.Mode = mode
	default:
		return fmt.Errorf("invalid server.mode %q: must be one of %q, %q, %q", c.Server.Mode, gin.DebugMode, gin.ReleaseMode, gin.TestMode)
	}

	// Validate server.port range.
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server.port %d: must be between 1 and 65535", c.Server.Port)
	}

	// Validate database.driver.
	switch c.Database.Driver {
	case "sqlite", "postgres":
		// ok
	default:
		return fmt.Errorf("invalid database.driver %q: must be one of %q, %q", c.Database.Driver, "sqlite", "postgres")
	}

	if c.Database.Driver == "sqlite" {
		sqlitePath := strings.TrimSpace(c.Database.SQLite.Path)
		if sqlitePath == "" {
			return fmt.Errorf("database.sqlite.path is required when driver is sqlite")
		}
		c.Database.SQLite.Path = sqlitePath
	}

	// When driver is postgres, required connection fields must be valid.
	if c.Database.Driver == "postgres" {
		host := strings.TrimSpace(c.Database.Postgres.Host)
		if host == "" {
			return fmt.Errorf("database.postgres.host is required when driver is postgres")
		}
		if c.Database.Postgres.Port < 1 || c.Database.Postgres.Port > 65535 {
			return fmt.Errorf("invalid database.postgres.port %d: must be between 1 and 65535", c.Database.Postgres.Port)
		}
		user := strings.TrimSpace(c.Database.Postgres.User)
		if user == "" {
			return fmt.Errorf("database.postgres.user is required when driver is postgres")
		}
		dbName := strings.TrimSpace(c.Database.Postgres.DBName)
		if dbName == "" {
			return fmt.Errorf("database.postgres.dbname is required when driver is postgres")
		}
		sslMode := strings.TrimSpace(c.Database.Postgres.SSLMode)

		switch sslMode {
		case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
			// ok
		default:
			return fmt.Errorf("invalid database.postgres.sslmode %q: must be one of %q, %q, %q, %q, %q, %q", c.Database.Postgres.SSLMode, "disable", "allow", "prefer", "require", "verify-ca", "verify-full")
		}

		c.Database.Postgres.Host = host
		c.Database.Postgres.User = user
		c.Database.Postgres.DBName = dbName
		c.Database.Postgres.SSLMode = sslMode
	}

	// Validate log.level.
	level := strings.ToLower(strings.TrimSpace(c.Log.Level))
	switch level {
	case "debug", "info", "warn", "error":
		c.Log.Level = level
	default:
		return fmt.Errorf("invalid log.level %q: must be one of %q, %q, %q, %q", c.Log.Level, "debug", "info", "warn", "error")
	}

	// Validate log.format.
	format := strings.ToLower(strings.TrimSpace(c.Log.Format))
	switch format {
	case "text", "json":
		c.Log.Format = format
	default:
		return fmt.Errorf("invalid log.format %q: must be one of %q, %q", c.Log.Format, "text", "json")
	}

	return nil
}
