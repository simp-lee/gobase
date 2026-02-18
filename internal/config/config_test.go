package config

import (
	"os"
	"path/filepath"
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
