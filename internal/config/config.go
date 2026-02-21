package config

import (
	"fmt"
	"strings"
	"time"
	"unicode"

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
	Auth     AuthConfig     `koanf:"auth"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host       string          `koanf:"host"`
	Port       int             `koanf:"port"`
	Mode       string          `koanf:"mode"`
	CSRFSecret string          `koanf:"csrf_secret"`
	Timeout    string          `koanf:"timeout"`
	CORS       CORSConfig      `koanf:"cors"`
	RateLimit  RateLimitConfig `koanf:"rate_limit"`
	Cache      CacheConfig     `koanf:"cache"`
}

// CORSConfig holds CORS middleware settings.
type CORSConfig struct {
	AllowOrigins     []string `koanf:"allow_origins"`
	AllowMethods     []string `koanf:"allow_methods"`
	AllowHeaders     []string `koanf:"allow_headers"`
	AllowCredentials bool     `koanf:"allow_credentials"`
	MaxAge           string   `koanf:"max_age"`
}

// RateLimitConfig holds rate limiting settings.
type RateLimitConfig struct {
	Enabled bool    `koanf:"enabled"`
	RPS     float64 `koanf:"rps"`
	Burst   int     `koanf:"burst"`
}

// CacheConfig holds HTTP response caching settings.
type CacheConfig struct {
	Enabled bool   `koanf:"enabled"`
	TTL     string `koanf:"ttl"`
	MaxSize int    `koanf:"max_size"`
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

// AuthConfig holds authentication and authorization settings.
type AuthConfig struct {
	Enabled     bool       `koanf:"enabled"`
	JWTSecret   string     `koanf:"jwt_secret"`
	TokenExpiry string     `koanf:"token_expiry"`
	PublicPaths []string   `koanf:"public_paths"`
	RBAC        RBACConfig `koanf:"rbac"`
}

// RBACConfig holds role-based access control settings.
type RBACConfig struct {
	Enabled bool            `koanf:"enabled"`
	Cache   RBACCacheConfig `koanf:"cache"`
}

// RBACCacheConfig holds RBAC cache tuning parameters.
type RBACCacheConfig struct {
	RoleTTL              string `koanf:"role_ttl"`
	UserRoleTTL          string `koanf:"user_role_ttl"`
	PermissionTTL        string `koanf:"permission_ttl"`
	MaxRoleEntries       int    `koanf:"max_role_entries"`
	MaxUserEntries       int    `koanf:"max_user_entries"`
	MaxPermissionEntries int    `koanf:"max_permission_entries"`
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

	// Validate server.host.
	host := strings.TrimSpace(c.Server.Host)
	if host == "" {
		return fmt.Errorf("server.host is required")
	}
	c.Server.Host = host

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
		if c.Server.Mode == gin.ReleaseMode {
			switch sslMode {
			case "require", "verify-ca", "verify-full":
				// ok
			default:
				return fmt.Errorf("invalid database.postgres.sslmode %q for server.mode %q: must be one of %q, %q, %q", c.Database.Postgres.SSLMode, gin.ReleaseMode, "require", "verify-ca", "verify-full")
			}
		}

		c.Database.Postgres.Host = host
		c.Database.Postgres.User = user
		c.Database.Postgres.DBName = dbName
		c.Database.Postgres.SSLMode = sslMode
	}

	// Normalize optional duration fields: whitespace-only means unset.
	c.Server.Timeout = strings.TrimSpace(c.Server.Timeout)
	c.Server.CORS.MaxAge = strings.TrimSpace(c.Server.CORS.MaxAge)
	c.Database.Pool.ConnMaxLifetime = strings.TrimSpace(c.Database.Pool.ConnMaxLifetime)
	c.Server.Cache.TTL = strings.TrimSpace(c.Server.Cache.TTL)

	// Validate server.timeout (optional; must be a valid Go duration if set).
	if t := c.Server.Timeout; t != "" {
		d, err := time.ParseDuration(t)
		if err != nil {
			return fmt.Errorf("invalid server.timeout %q: %w", c.Server.Timeout, err)
		}
		if d <= 0 {
			return fmt.Errorf("invalid server.timeout %q: must be greater than 0", c.Server.Timeout)
		}
	}

	// Validate server.cors.max_age (optional; must be a valid Go duration if set).
	if ma := c.Server.CORS.MaxAge; ma != "" {
		d, err := time.ParseDuration(ma)
		if err != nil {
			return fmt.Errorf("invalid server.cors.max_age %q: must be a valid duration (e.g. \"24h\", \"3600s\"): %w", c.Server.CORS.MaxAge, err)
		}
		if d <= 0 {
			return fmt.Errorf("invalid server.cors.max_age %q: must be greater than 0", c.Server.CORS.MaxAge)
		}
	}

	// Validate database.pool.conn_max_lifetime (optional; must be positive if set).
	if lm := c.Database.Pool.ConnMaxLifetime; lm != "" {
		d, err := time.ParseDuration(lm)
		if err != nil {
			return fmt.Errorf("invalid database.pool.conn_max_lifetime %q: %w", c.Database.Pool.ConnMaxLifetime, err)
		}
		if d <= 0 {
			return fmt.Errorf("invalid database.pool.conn_max_lifetime %q: must be greater than 0", c.Database.Pool.ConnMaxLifetime)
		}
	}

	// Validate server.rate_limit (when enabled, rps and burst must be positive).
	if c.Server.RateLimit.Enabled {
		if c.Server.RateLimit.RPS <= 0 {
			return fmt.Errorf("invalid server.rate_limit.rps %v: must be positive when rate limiting is enabled", c.Server.RateLimit.RPS)
		}
		if c.Server.RateLimit.Burst <= 0 {
			return fmt.Errorf("invalid server.rate_limit.burst %d: must be positive when rate limiting is enabled", c.Server.RateLimit.Burst)
		}
	}

	// Validate server.cache (when enabled, ttl must be a valid positive duration, max_size > 0).
	if c.Server.Cache.Enabled {
		d, err := time.ParseDuration(c.Server.Cache.TTL)
		if err != nil {
			return fmt.Errorf("invalid server.cache.ttl %q: %w", c.Server.Cache.TTL, err)
		}
		if d <= 0 {
			return fmt.Errorf("invalid server.cache.ttl %q: must be greater than 0", c.Server.Cache.TTL)
		}
		if c.Server.Cache.MaxSize <= 0 {
			return fmt.Errorf("invalid server.cache.max_size %d: must be positive when caching is enabled", c.Server.Cache.MaxSize)
		}
	}

	// Validate auth config (when enabled).
	if c.Auth.RBAC.Enabled && !c.Auth.Enabled {
		return fmt.Errorf("auth.rbac.enabled requires auth.enabled to be true")
	}

	if c.Auth.Enabled {
		jwtSecret := strings.TrimSpace(c.Auth.JWTSecret)
		if jwtSecret == "" {
			return fmt.Errorf("auth.jwt_secret is required when auth is enabled")
		}
		if len(jwtSecret) < 32 {
			return fmt.Errorf("invalid auth.jwt_secret: must be at least 32 characters")
		}
		c.Auth.JWTSecret = jwtSecret

		tokenExpiry := strings.TrimSpace(c.Auth.TokenExpiry)
		if tokenExpiry == "" {
			return fmt.Errorf("auth.token_expiry is required when auth is enabled")
		}
		td, err := time.ParseDuration(tokenExpiry)
		if err != nil {
			return fmt.Errorf("invalid auth.token_expiry %q: %w", c.Auth.TokenExpiry, err)
		}
		if td <= 0 {
			return fmt.Errorf("invalid auth.token_expiry %q: must be greater than 0", c.Auth.TokenExpiry)
		}
		c.Auth.TokenExpiry = tokenExpiry

		publicPaths := make([]string, 0, len(c.Auth.PublicPaths))
		seenPublicPaths := make(map[string]struct{}, len(c.Auth.PublicPaths))
		for idx, p := range c.Auth.PublicPaths {
			normalizedPath := strings.TrimSpace(p)
			if normalizedPath == "" {
				return fmt.Errorf("auth.public_paths[%d] cannot be empty when auth is enabled", idx)
			}
			if !strings.HasPrefix(normalizedPath, "/") {
				return fmt.Errorf("invalid auth.public_paths[%d] %q: must start with '/'", idx, p)
			}
			if _, exists := seenPublicPaths[normalizedPath]; exists {
				continue
			}
			seenPublicPaths[normalizedPath] = struct{}{}
			publicPaths = append(publicPaths, normalizedPath)
		}
		if len(publicPaths) == 0 {
			return fmt.Errorf("auth.public_paths is required when auth is enabled")
		}

		requiredPublicPaths := []string{"/api/v1/auth/login", "/api/v1/auth/register"}
		for _, requiredPath := range requiredPublicPaths {
			if _, exists := seenPublicPaths[requiredPath]; !exists {
				return fmt.Errorf("auth.public_paths must include %q when auth is enabled", requiredPath)
			}
		}
		c.Auth.PublicPaths = publicPaths

		if c.Server.Mode == gin.ReleaseMode {
			if CountSecretClasses(jwtSecret) < 3 {
				return fmt.Errorf("auth.jwt_secret must include at least 3 character classes (lowercase, uppercase, digit, symbol) in release mode")
			}
		}
	}

	// Validate RBAC cache config (when RBAC is enabled).
	if c.Auth.RBAC.Enabled {
		cacheCfg := &c.Auth.RBAC.Cache

		// Validate cache TTL fields.
		ttlFields := []struct {
			name  string
			value *string
		}{
			{"auth.rbac.cache.role_ttl", &cacheCfg.RoleTTL},
			{"auth.rbac.cache.user_role_ttl", &cacheCfg.UserRoleTTL},
			{"auth.rbac.cache.permission_ttl", &cacheCfg.PermissionTTL},
		}
		for _, f := range ttlFields {
			v := strings.TrimSpace(*f.value)
			if v == "" {
				return fmt.Errorf("%s is required when RBAC is enabled", f.name)
			}
			d, err := time.ParseDuration(v)
			if err != nil {
				return fmt.Errorf("invalid %s %q: %w", f.name, *f.value, err)
			}
			if d <= 0 {
				return fmt.Errorf("invalid %s %q: must be greater than 0", f.name, *f.value)
			}
			*f.value = v
		}

		// Validate max entries fields.
		entryFields := []struct {
			name  string
			value int
		}{
			{"auth.rbac.cache.max_role_entries", cacheCfg.MaxRoleEntries},
			{"auth.rbac.cache.max_user_entries", cacheCfg.MaxUserEntries},
			{"auth.rbac.cache.max_permission_entries", cacheCfg.MaxPermissionEntries},
		}
		for _, f := range entryFields {
			if f.value <= 0 {
				return fmt.Errorf("invalid %s %d: must be positive when RBAC is enabled", f.name, f.value)
			}
		}
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

// CountSecretClasses counts how many character classes (lowercase, uppercase,
// digit, symbol) are present in the given secret string.
func CountSecretClasses(secret string) int {
	hasLower := false
	hasUpper := false
	hasDigit := false
	hasSymbol := false

	for _, r := range secret {
		switch {
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		default:
			hasSymbol = true
		}
	}

	classes := 0
	if hasLower {
		classes++
	}
	if hasUpper {
		classes++
	}
	if hasDigit {
		classes++
	}
	if hasSymbol {
		classes++
	}

	return classes
}
