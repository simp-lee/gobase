package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testYAML = `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
  csrf_secret: "test-csrf-secret-value"
database:
  driver: "postgres"
  sqlite:
    path: "data/test.db"
  postgres:
    host: "db.example.com"
    port: 5433
    user: "admin"
    password: "secret"
    dbname: "testdb"
    sslmode: "require"
  pool:
    max_idle_conns: 5
    max_open_conns: 50
    conn_max_lifetime: "30m"
log:
  level: "info"
  format: "json"
`

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}

func TestLoad_FullYAML(t *testing.T) {
	path := writeTestConfig(t, testYAML)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Server
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "127.0.0.1")
	}
	if cfg.Server.Port != 3000 {
		t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 3000)
	}
	if cfg.Server.Mode != "release" {
		t.Errorf("Server.Mode = %q, want %q", cfg.Server.Mode, "release")
	}
	if cfg.Server.CSRFSecret != "test-csrf-secret-value" {
		t.Errorf("Server.CSRFSecret = %q, want %q", cfg.Server.CSRFSecret, "test-csrf-secret-value")
	}

	// Database
	if cfg.Database.Driver != "postgres" {
		t.Errorf("Database.Driver = %q, want %q", cfg.Database.Driver, "postgres")
	}
	if cfg.Database.SQLite.Path != "data/test.db" {
		t.Errorf("SQLite.Path = %q, want %q", cfg.Database.SQLite.Path, "data/test.db")
	}
	if cfg.Database.Postgres.Host != "db.example.com" {
		t.Errorf("Postgres.Host = %q, want %q", cfg.Database.Postgres.Host, "db.example.com")
	}
	if cfg.Database.Postgres.Port != 5433 {
		t.Errorf("Postgres.Port = %d, want %d", cfg.Database.Postgres.Port, 5433)
	}
	if cfg.Database.Postgres.User != "admin" {
		t.Errorf("Postgres.User = %q, want %q", cfg.Database.Postgres.User, "admin")
	}
	if cfg.Database.Postgres.Password != "secret" {
		t.Errorf("Postgres.Password = %q, want %q", cfg.Database.Postgres.Password, "secret")
	}
	if cfg.Database.Postgres.DBName != "testdb" {
		t.Errorf("Postgres.DBName = %q, want %q", cfg.Database.Postgres.DBName, "testdb")
	}
	if cfg.Database.Postgres.SSLMode != "require" {
		t.Errorf("Postgres.SSLMode = %q, want %q", cfg.Database.Postgres.SSLMode, "require")
	}

	// Pool (M2)
	if cfg.Database.Pool.MaxIdleConns != 5 {
		t.Errorf("Pool.MaxIdleConns = %d, want %d", cfg.Database.Pool.MaxIdleConns, 5)
	}
	if cfg.Database.Pool.MaxOpenConns != 50 {
		t.Errorf("Pool.MaxOpenConns = %d, want %d", cfg.Database.Pool.MaxOpenConns, 50)
	}
	if cfg.Database.Pool.ConnMaxLifetime != "30m" {
		t.Errorf("Pool.ConnMaxLifetime = %q, want %q", cfg.Database.Pool.ConnMaxLifetime, "30m")
	}

	// Log
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "info")
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Log.Format = %q, want %q", cfg.Log.Format, "json")
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	path := writeTestConfig(t, testYAML)

	t.Setenv("APP__SERVER__PORT", "9090")
	t.Setenv("APP__DATABASE__DRIVER", "sqlite")
	t.Setenv("APP__LOG__LEVEL", "error")

	// PoolConfig fields contain underscores — verify single _ is preserved.
	t.Setenv("APP__DATABASE__POOL__MAX_IDLE_CONNS", "20")
	t.Setenv("APP__DATABASE__POOL__MAX_OPEN_CONNS", "200")
	t.Setenv("APP__DATABASE__POOL__CONN_MAX_LIFETIME", "2h")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want %d (env override)", cfg.Server.Port, 9090)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Errorf("Database.Driver = %q, want %q (env override)", cfg.Database.Driver, "sqlite")
	}
	if cfg.Log.Level != "error" {
		t.Errorf("Log.Level = %q, want %q (env override)", cfg.Log.Level, "error")
	}

	// PoolConfig env overrides.
	if cfg.Database.Pool.MaxIdleConns != 20 {
		t.Errorf("Pool.MaxIdleConns = %d, want %d (env override)", cfg.Database.Pool.MaxIdleConns, 20)
	}
	if cfg.Database.Pool.MaxOpenConns != 200 {
		t.Errorf("Pool.MaxOpenConns = %d, want %d (env override)", cfg.Database.Pool.MaxOpenConns, 200)
	}
	if cfg.Database.Pool.ConnMaxLifetime != "2h" {
		t.Errorf("Pool.ConnMaxLifetime = %q, want %q (env override)", cfg.Database.Pool.ConnMaxLifetime, "2h")
	}

	// Non-overridden values should remain from YAML.
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Server.Host = %q, want %q (unchanged)", cfg.Server.Host, "127.0.0.1")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("Load() expected error for missing file, got nil")
	}
}

func TestLoad_InvalidServerMode(t *testing.T) {
	path := writeTestConfig(t, `server:
  host: "127.0.0.1"
  port: 3000
  mode: "invalid"
database:
  driver: "sqlite"
  sqlite:
    path: "data/test.db"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for invalid server mode, got nil")
	}
	if !strings.Contains(err.Error(), "server.mode") {
		t.Fatalf("Load() error = %v, want contains %q", err, "server.mode")
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	path := writeTestConfig(t, `server:
  host: "127.0.0.1"
  port: 0
  mode: "release"
database:
  driver: "sqlite"
  sqlite:
    path: "data/test.db"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for port 0, got nil")
	}

	path = writeTestConfig(t, `server:
  host: "127.0.0.1"
  port: 70000
  mode: "release"
database:
  driver: "sqlite"
  sqlite:
    path: "data/test.db"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
`)
	_, err = Load(path)
	if err == nil {
		t.Fatal("Load() expected error for port 70000, got nil")
	}
}

func TestLoad_InvalidServerHost(t *testing.T) {
	path := writeTestConfig(t, `server:
  host: ""
  port: 3000
  mode: "release"
database:
  driver: "sqlite"
  sqlite:
    path: "data/test.db"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for empty server host, got nil")
	}
	if !strings.Contains(err.Error(), "server.host") {
		t.Fatalf("Load() error = %v, want contains %q", err, "server.host")
	}

	path = writeTestConfig(t, `server:
  host: "   "
  port: 3000
  mode: "release"
database:
  driver: "sqlite"
  sqlite:
    path: "data/test.db"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
`)

	_, err = Load(path)
	if err == nil {
		t.Fatal("Load() expected error for whitespace-only server host, got nil")
	}
	if !strings.Contains(err.Error(), "server.host") {
		t.Fatalf("Load() error = %v, want contains %q", err, "server.host")
	}
}

func TestLoad_InvalidDatabaseDriver(t *testing.T) {
	path := writeTestConfig(t, `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
database:
  driver: "mysql"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for unsupported driver 'mysql', got nil")
	}
}

func TestLoad_PostgresMissingFields(t *testing.T) {
	path := writeTestConfig(t, `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
database:
  driver: "postgres"
  postgres:
    host: ""
    user: "admin"
    dbname: "testdb"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for empty postgres host, got nil")
	}

	path = writeTestConfig(t, `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
database:
  driver: "postgres"
  postgres:
    host: "localhost"
    user: ""
    dbname: "testdb"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
`)
	_, err = Load(path)
	if err == nil {
		t.Fatal("Load() expected error for empty postgres user, got nil")
	}

	path = writeTestConfig(t, `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
database:
  driver: "postgres"
  postgres:
    host: "localhost"
    user: "admin"
    dbname: ""
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
`)
	_, err = Load(path)
	if err == nil {
		t.Fatal("Load() expected error for empty postgres dbname, got nil")
	}
}

func TestLoad_SQLiteMissingPath(t *testing.T) {
	path := writeTestConfig(t, `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
database:
  driver: "sqlite"
  sqlite:
    path: ""
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for empty sqlite path, got nil")
	}
	if !strings.Contains(err.Error(), "database.sqlite.path") {
		t.Fatalf("Load() error = %v, want contains %q", err, "database.sqlite.path")
	}
}

func TestLoad_PostgresInvalidPortOrSSLMode(t *testing.T) {
	path := writeTestConfig(t, `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
database:
  driver: "postgres"
  postgres:
    host: "localhost"
    port: 0
    user: "admin"
    password: "secret"
    dbname: "testdb"
    sslmode: "require"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for postgres port 0, got nil")
	}
	if !strings.Contains(err.Error(), "database.postgres.port") {
		t.Fatalf("Load() error = %v, want contains %q", err, "database.postgres.port")
	}

	path = writeTestConfig(t, `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
database:
  driver: "postgres"
  postgres:
    host: "localhost"
    port: 5432
    user: "admin"
    password: "secret"
    dbname: "testdb"
    sslmode: "invalid"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
`)

	_, err = Load(path)
	if err == nil {
		t.Fatal("Load() expected error for invalid postgres sslmode, got nil")
	}
	if !strings.Contains(err.Error(), "database.postgres.sslmode") {
		t.Fatalf("Load() error = %v, want contains %q", err, "database.postgres.sslmode")
	}
}

func TestLoad_PostgresSSLMode_ReleaseRestriction(t *testing.T) {
	path := writeTestConfig(t, `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
database:
  driver: "postgres"
  postgres:
    host: "localhost"
    port: 5432
    user: "admin"
    password: "secret"
    dbname: "testdb"
    sslmode: "disable"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for insecure postgres sslmode in release mode, got nil")
	}
	if !strings.Contains(err.Error(), "database.postgres.sslmode") {
		t.Fatalf("Load() error = %v, want contains %q", err, "database.postgres.sslmode")
	}

	path = writeTestConfig(t, `server:
  host: "127.0.0.1"
  port: 3000
  mode: "debug"
database:
  driver: "postgres"
  postgres:
    host: "localhost"
    port: 5432
    user: "admin"
    password: "secret"
    dbname: "testdb"
    sslmode: "disable"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
`)

	if _, err = Load(path); err != nil {
		t.Fatalf("Load() expected debug mode to allow postgres sslmode disable, got error: %v", err)
	}
}

func TestLoad_NonPositiveDurations(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		wantContain string
	}{
		{
			name: "server timeout must be positive",
			yaml: `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
  timeout: "0s"
database:
  driver: "sqlite"
  sqlite:
    path: "data/test.db"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
`,
			wantContain: "server.timeout",
		},
		{
			name: "cors max age must be positive",
			yaml: `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
  cors:
    max_age: "-1s"
database:
  driver: "sqlite"
  sqlite:
    path: "data/test.db"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
`,
			wantContain: "server.cors.max_age",
		},
		{
			name: "pool lifetime must be positive",
			yaml: `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
database:
  driver: "sqlite"
  sqlite:
    path: "data/test.db"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "0s"
log:
  level: "info"
  format: "json"
`,
			wantContain: "database.pool.conn_max_lifetime",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestConfig(t, tt.yaml)
			_, err := Load(path)
			if err == nil {
				t.Fatal("Load() expected error for non-positive duration, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantContain) {
				t.Fatalf("Load() error = %v, want contains %q", err, tt.wantContain)
			}
		})
	}
}

func TestLoad_OptionalDurationWhitespace_NormalizedAsUnset(t *testing.T) {
	path := writeTestConfig(t, `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
  timeout: "   "
  cors:
    max_age: "   "
database:
  driver: "sqlite"
  sqlite:
    path: "data/test.db"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "   "
log:
  level: "info"
  format: "json"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.Timeout != "" {
		t.Errorf("Server.Timeout = %q, want empty string", cfg.Server.Timeout)
	}
	if cfg.Server.CORS.MaxAge != "" {
		t.Errorf("Server.CORS.MaxAge = %q, want empty string", cfg.Server.CORS.MaxAge)
	}
	if cfg.Database.Pool.ConnMaxLifetime != "" {
		t.Errorf("Database.Pool.ConnMaxLifetime = %q, want empty string", cfg.Database.Pool.ConnMaxLifetime)
	}
}

func TestLoad_CacheConfig(t *testing.T) {
	base := func(cacheBlock string) string {
		return `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
` + cacheBlock + `
database:
  driver: "sqlite"
  sqlite:
    path: "data/test.db"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
`
	}

	tests := []struct {
		name        string
		cacheBlock  string
		wantErr     bool
		wantContain string
	}{
		{
			name: "enabled with invalid TTL",
			cacheBlock: `  cache:
    enabled: true
    ttl: "not-a-duration"
    max_size: 100`,
			wantErr:     true,
			wantContain: "server.cache.ttl",
		},
		{
			name: "enabled with zero TTL",
			cacheBlock: `  cache:
    enabled: true
    ttl: "0s"
    max_size: 100`,
			wantErr:     true,
			wantContain: "server.cache.ttl",
		},
		{
			name: "enabled with negative TTL",
			cacheBlock: `  cache:
    enabled: true
    ttl: "-5m"
    max_size: 100`,
			wantErr:     true,
			wantContain: "server.cache.ttl",
		},
		{
			name: "enabled with max_size zero",
			cacheBlock: `  cache:
    enabled: true
    ttl: "5m"
    max_size: 0`,
			wantErr:     true,
			wantContain: "server.cache.max_size",
		},
		{
			name: "enabled with negative max_size",
			cacheBlock: `  cache:
    enabled: true
    ttl: "5m"
    max_size: -1`,
			wantErr:     true,
			wantContain: "server.cache.max_size",
		},
		{
			name: "enabled with valid settings",
			cacheBlock: `  cache:
    enabled: true
    ttl: "5m"
    max_size: 1000`,
			wantErr: false,
		},
		{
			name: "disabled skips validation",
			cacheBlock: `  cache:
    enabled: false
    ttl: "bad"
    max_size: -1`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestConfig(t, base(tt.cacheBlock))
			_, err := Load(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Load() expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantContain) {
					t.Fatalf("Load() error = %v, want contains %q", err, tt.wantContain)
				}
			} else {
				if err != nil {
					t.Fatalf("Load() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestLoad_DefaultConfig(t *testing.T) {
	// Verify loading the actual project config.yaml works.
	cfg, err := Load("../../configs/config.yaml")
	if err != nil {
		t.Fatalf("Load() error on project config: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 8080)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Errorf("Database.Driver = %q, want %q", cfg.Database.Driver, "sqlite")
	}
	if cfg.Database.Pool.MaxIdleConns != 10 {
		t.Errorf("Pool.MaxIdleConns = %d, want %d", cfg.Database.Pool.MaxIdleConns, 10)
	}
	if cfg.Database.Pool.MaxOpenConns != 100 {
		t.Errorf("Pool.MaxOpenConns = %d, want %d", cfg.Database.Pool.MaxOpenConns, 100)
	}
	if cfg.Database.Pool.ConnMaxLifetime != "1h" {
		t.Errorf("Pool.ConnMaxLifetime = %q, want %q", cfg.Database.Pool.ConnMaxLifetime, "1h")
	}
}

func TestDefaultConfigYAML_ContainsAuthSection(t *testing.T) {
	requiredAuthKeys := []string{
		"auth:",
		"enabled:",
		"jwt_secret:",
		"token_expiry:",
		"public_paths:",
		"rbac:",
	}

	missingAuthKeys := func(content string, required []string) []string {
		missing := make([]string, 0)
		for _, key := range required {
			if !strings.Contains(content, key) {
				missing = append(missing, key)
			}
		}
		return missing
	}

	tests := []struct {
		name           string
		content        string
		wantNoMissing  bool
		expectContains string
	}{
		{
			name: "default config contains all auth keys",
			content: func() string {
				b, err := os.ReadFile("../../configs/config.yaml")
				if err != nil {
					t.Fatalf("read ../../configs/config.yaml: %v", err)
				}
				return string(b)
			}(),
			wantNoMissing: true,
		},
		{
			name: "missing auth key is detected",
			content: `server:
  host: "127.0.0.1"
  port: 8080
  mode: "debug"
database:
  driver: "sqlite"
  sqlite:
    path: "data/app.db"
log:
  level: "debug"
  format: "text"
auth:
  enabled: false
  jwt_secret: ""
  token_expiry: "24h"
  rbac:
    enabled: false
`,
			wantNoMissing:  false,
			expectContains: "public_paths:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			missing := missingAuthKeys(tt.content, requiredAuthKeys)
			if tt.wantNoMissing {
				if len(missing) != 0 {
					t.Fatalf("missing auth keys: %v", missing)
				}
				return
			}

			if len(missing) == 0 {
				t.Fatal("expected missing auth keys, got none")
			}
			if tt.expectContains != "" {
				found := false
				for _, key := range missing {
					if key == tt.expectContains {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("missing auth keys = %v, want include %q", missing, tt.expectContains)
				}
			}
		})
	}
}

func TestLoad_DefaultConfig_AuthFieldsAccessible(t *testing.T) {
	cfg, err := Load("../../configs/config.yaml")
	if err != nil {
		t.Fatalf("Load() error on project config: %v", err)
	}

	if cfg.Auth.TokenExpiry != "24h" {
		t.Errorf("Auth.TokenExpiry = %q, want %q", cfg.Auth.TokenExpiry, "24h")
	}
	if len(cfg.Auth.PublicPaths) == 0 {
		t.Fatal("Auth.PublicPaths is empty, want non-empty")
	}
	if cfg.Auth.PublicPaths[0] != "/api/v1/auth/login" {
		t.Errorf("Auth.PublicPaths[0] = %q, want %q", cfg.Auth.PublicPaths[0], "/api/v1/auth/login")
	}
	if cfg.Auth.RBAC.Cache.RoleTTL != "5m" {
		t.Errorf("Auth.RBAC.Cache.RoleTTL = %q, want %q", cfg.Auth.RBAC.Cache.RoleTTL, "5m")
	}
}

func TestCountSecretClasses(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		want   int
	}{
		{name: "empty string", secret: "", want: 0},
		{name: "lowercase only", secret: "abcdef", want: 1},
		{name: "uppercase only", secret: "ABCDEF", want: 1},
		{name: "digits only", secret: "123456", want: 1},
		{name: "symbols only", secret: "!@#$%^", want: 1},
		{name: "lower and upper", secret: "abcDEF", want: 2},
		{name: "lower upper digit", secret: "abcDEF123", want: 3},
		{name: "all four classes", secret: "abcDEF123!", want: 4},
		{name: "mixed with spaces", secret: "aA1 ", want: 4}, // space counts as symbol
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CountSecretClasses(tt.secret)
			if got != tt.want {
				t.Errorf("CountSecretClasses(%q) = %d, want %d", tt.secret, got, tt.want)
			}
		})
	}
}

// validBaseYAML returns a minimal valid YAML config string (sqlite, debug mode).
func validBaseYAML(extras string) string {
	return `server:
  host: "127.0.0.1"
  port: 3000
  mode: "debug"
database:
  driver: "sqlite"
  sqlite:
    path: "data/test.db"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
` + extras
}

// validReleaseBaseYAML returns a minimal valid YAML config string (sqlite, release mode).
func validReleaseBaseYAML(extras string) string {
	return `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
database:
  driver: "sqlite"
  sqlite:
    path: "data/test.db"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
` + extras
}

func TestLoad_AuthConfig(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		wantErr     bool
		wantContain string
	}{
		{
			name:    "auth disabled skips validation",
			yaml:    validBaseYAML("auth:\n  enabled: false\n  jwt_secret: \"\"\n  token_expiry: \"bad\"\n"),
			wantErr: false,
		},
		{
			name:        "auth enabled with empty jwt_secret",
			yaml:        validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n"),
			wantErr:     true,
			wantContain: "auth.jwt_secret",
		},
		{
			name:        "auth enabled with short jwt_secret",
			yaml:        validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"tooshort\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n"),
			wantErr:     true,
			wantContain: "auth.jwt_secret",
		},
		{
			name:    "auth enabled with jwt_secret exactly 32 chars passes",
			yaml:    validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n"),
			wantErr: false,
		},
		{
			name:        "auth enabled with empty token_expiry",
			yaml:        validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n"),
			wantErr:     true,
			wantContain: "auth.token_expiry",
		},
		{
			name:        "auth enabled with invalid token_expiry",
			yaml:        validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"not-a-duration\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n"),
			wantErr:     true,
			wantContain: "auth.token_expiry",
		},
		{
			name:        "auth enabled with zero token_expiry",
			yaml:        validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"0s\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n"),
			wantErr:     true,
			wantContain: "auth.token_expiry",
		},
		{
			name:        "auth enabled with negative token_expiry",
			yaml:        validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"-1h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n"),
			wantErr:     true,
			wantContain: "auth.token_expiry",
		},
		{
			name:    "auth enabled with valid settings in debug mode",
			yaml:    validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n"),
			wantErr: false,
		},
		{
			name:        "auth enabled with empty public_paths",
			yaml:        validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"24h\"\n  public_paths: []\n"),
			wantErr:     true,
			wantContain: "auth.public_paths",
		},
		{
			name:        "auth enabled with invalid public_paths format",
			yaml:        validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n"),
			wantErr:     true,
			wantContain: "auth.public_paths",
		},
		{
			name:        "auth enabled requires login in public_paths",
			yaml:        validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/register\"\n"),
			wantErr:     true,
			wantContain: "/api/v1/auth/login",
		},
		{
			name:        "auth enabled requires register in public_paths",
			yaml:        validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n"),
			wantErr:     true,
			wantContain: "/api/v1/auth/register",
		},
		{
			name:    "auth enabled with explicit valid public_paths",
			yaml:    validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \" /api/v1/auth/login \"\n    - \"/api/v1/auth/register\"\n"),
			wantErr: false,
		},
		{
			name:        "release mode rejects jwt_secret with low complexity",
			yaml:        validReleaseBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n"),
			wantErr:     true,
			wantContain: "auth.jwt_secret",
		},
		{
			name:    "release mode accepts jwt_secret with high complexity",
			yaml:    validReleaseBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"Abcd1234!Abcd1234!Abcd1234!Abcd1234!\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestConfig(t, tt.yaml)
			_, err := Load(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Load() expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantContain) {
					t.Fatalf("Load() error = %v, want contains %q", err, tt.wantContain)
				}
			} else {
				if err != nil {
					t.Fatalf("Load() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestLoad_RBACConfig(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		wantErr     bool
		wantContain string
	}{
		{
			name:        "rbac enabled but auth disabled",
			yaml:        validBaseYAML("auth:\n  enabled: false\n  rbac:\n    enabled: true\n"),
			wantErr:     true,
			wantContain: "auth.rbac.enabled",
		},
		{
			name:        "rbac enabled with missing cache ttls",
			yaml:        validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n  rbac:\n    enabled: true\n    cache:\n      role_ttl: \"\"\n      user_role_ttl: \"5m\"\n      permission_ttl: \"5m\"\n      max_role_entries: 100\n      max_user_entries: 500\n      max_permission_entries: 200\n"),
			wantErr:     true,
			wantContain: "auth.rbac.cache.role_ttl",
		},
		{
			name:        "rbac enabled with invalid user_role_ttl",
			yaml:        validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n  rbac:\n    enabled: true\n    cache:\n      role_ttl: \"5m\"\n      user_role_ttl: \"bad\"\n      permission_ttl: \"5m\"\n      max_role_entries: 100\n      max_user_entries: 500\n      max_permission_entries: 200\n"),
			wantErr:     true,
			wantContain: "auth.rbac.cache.user_role_ttl",
		},
		{
			name:        "rbac enabled with zero permission_ttl",
			yaml:        validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n  rbac:\n    enabled: true\n    cache:\n      role_ttl: \"5m\"\n      user_role_ttl: \"5m\"\n      permission_ttl: \"0s\"\n      max_role_entries: 100\n      max_user_entries: 500\n      max_permission_entries: 200\n"),
			wantErr:     true,
			wantContain: "auth.rbac.cache.permission_ttl",
		},
		{
			name:        "rbac enabled with zero max_role_entries",
			yaml:        validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n  rbac:\n    enabled: true\n    cache:\n      role_ttl: \"5m\"\n      user_role_ttl: \"5m\"\n      permission_ttl: \"5m\"\n      max_role_entries: 0\n      max_user_entries: 500\n      max_permission_entries: 200\n"),
			wantErr:     true,
			wantContain: "auth.rbac.cache.max_role_entries",
		},
		{
			name:        "rbac enabled with negative max_user_entries",
			yaml:        validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n  rbac:\n    enabled: true\n    cache:\n      role_ttl: \"5m\"\n      user_role_ttl: \"5m\"\n      permission_ttl: \"5m\"\n      max_role_entries: 100\n      max_user_entries: -1\n      max_permission_entries: 200\n"),
			wantErr:     true,
			wantContain: "auth.rbac.cache.max_user_entries",
		},
		{
			name:        "rbac enabled with zero max_permission_entries",
			yaml:        validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n  rbac:\n    enabled: true\n    cache:\n      role_ttl: \"5m\"\n      user_role_ttl: \"5m\"\n      permission_ttl: \"5m\"\n      max_role_entries: 100\n      max_user_entries: 500\n      max_permission_entries: 0\n"),
			wantErr:     true,
			wantContain: "auth.rbac.cache.max_permission_entries",
		},
		{
			name:    "rbac enabled with valid settings",
			yaml:    validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n  rbac:\n    enabled: true\n    cache:\n      role_ttl: \"5m\"\n      user_role_ttl: \"5m\"\n      permission_ttl: \"10m\"\n      max_role_entries: 100\n      max_user_entries: 500\n      max_permission_entries: 200\n"),
			wantErr: false,
		},
		{
			name:    "rbac disabled skips cache validation",
			yaml:    validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n  rbac:\n    enabled: false\n    cache:\n      role_ttl: \"\"\n      max_role_entries: 0\n"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestConfig(t, tt.yaml)
			_, err := Load(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Load() expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantContain) {
					t.Fatalf("Load() error = %v, want contains %q", err, tt.wantContain)
				}
			} else {
				if err != nil {
					t.Fatalf("Load() unexpected error: %v", err)
				}
			}
		})
	}
}
