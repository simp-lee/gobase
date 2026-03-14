package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testYAML = `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
  csrf_secret: "TestCsrf!Secret#2024xHereValid!!"
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
    conn_max_idle_time: "10m"
log:
  level: "info"
  format: "json"
`

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	clearAPPEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}

func clearAPPEnv(t *testing.T) {
	t.Helper()

	type envVar struct {
		key   string
		value string
	}

	vars := make([]envVar, 0)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(key, "APP__") {
			continue
		}
		vars = append(vars, envVar{key: key, value: value})
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("Unsetenv(%q): %v", key, err)
		}
	}

	t.Cleanup(func() {
		for _, envVar := range vars {
			_ = os.Setenv(envVar.key, envVar.value)
		}
	})
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
	if cfg.Server.CSRFSecret != "TestCsrf!Secret#2024xHereValid!!" {
		t.Errorf("Server.CSRFSecret = %q, want %q", cfg.Server.CSRFSecret, "TestCsrf!Secret#2024xHereValid!!")
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
	if cfg.Database.Pool.ConnMaxIdleTime != "10m" {
		t.Errorf("Pool.ConnMaxIdleTime = %q, want %q", cfg.Database.Pool.ConnMaxIdleTime, "10m")
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
	t.Setenv("APP__DATABASE__POOL__CONN_MAX_IDLE_TIME", "15m")

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
	if cfg.Database.Pool.ConnMaxIdleTime != "15m" {
		t.Errorf("Pool.ConnMaxIdleTime = %q, want %q (env override)", cfg.Database.Pool.ConnMaxIdleTime, "15m")
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

func TestLoad_ServerModeTestAllowed(t *testing.T) {
	path := writeTestConfig(t, "server:\n"+
		"  host: \"127.0.0.1\"\n"+
		"  port: 3000\n"+
		"  mode: \"test\"\n"+
		"database:\n"+
		"  driver: \"sqlite\"\n"+
		"  sqlite:\n"+
		"    path: \"data/test.db\"\n"+
		"  pool:\n"+
		"    max_idle_conns: 1\n"+
		"    max_open_conns: 1\n"+
		"    conn_max_lifetime: \"1m\"\n"+
		"log:\n"+
		"  level: \"info\"\n"+
		"  format: \"json\"\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error for server mode test: %v", err)
	}
	if cfg.Server.Mode != "test" {
		t.Fatalf("Server.Mode = %q, want %q", cfg.Server.Mode, "test")
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	path := writeTestConfig(t, "server:\n"+
		"  host: \"127.0.0.1\"\n"+
		"  port: 0\n"+
		"  mode: \"release\"\n"+
		"database:\n"+
		"  driver: \"sqlite\"\n"+
		"  sqlite:\n"+
		"    path: \"data/test.db\"\n"+
		"  pool:\n"+
		"    max_idle_conns: 1\n"+
		"    max_open_conns: 1\n"+
		"    conn_max_lifetime: \"1m\"\n"+
		"log:\n"+
		"  level: \"info\"\n"+
		"  format: \"json\"\n")
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for port 0, got nil")
	}
	if !strings.Contains(err.Error(), "server.port") {
		t.Fatalf("Load() error = %v, want contains %q", err, "server.port")
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
	if !strings.Contains(err.Error(), "server.port") {
		t.Fatalf("Load() error = %v, want contains %q", err, "server.port")
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
	if !strings.Contains(err.Error(), "database.driver") {
		t.Fatalf("Load() error = %v, want contains %q", err, "database.driver")
	}
}

func TestLoad_PostgresMissingFields(t *testing.T) {
	postgresYAML := func(host, user, dbname string) string {
		return "server:\n" +
			"  host: \"127.0.0.1\"\n" +
			"  port: 3000\n" +
			"  mode: \"release\"\n" +
			"database:\n" +
			"  driver: \"postgres\"\n" +
			"  postgres:\n" +
			fmt.Sprintf("    host: %q\n", host) +
			"    port: 5432\n" +
			fmt.Sprintf("    user: %q\n", user) +
			fmt.Sprintf("    dbname: %q\n", dbname) +
			"  pool:\n" +
			"    max_idle_conns: 1\n" +
			"    max_open_conns: 1\n" +
			"    conn_max_lifetime: \"1m\"\n" +
			"log:\n" +
			"  level: \"info\"\n" +
			"  format: \"json\"\n"
	}

	path := writeTestConfig(t, postgresYAML("", "admin", "testdb"))
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for empty postgres host, got nil")
	}
	if !strings.Contains(err.Error(), "database.postgres.host") {
		t.Fatalf("Load() error = %v, want contains %q", err, "database.postgres.host")
	}

	path = writeTestConfig(t, postgresYAML("localhost", "", "testdb"))
	_, err = Load(path)
	if err == nil {
		t.Fatal("Load() expected error for empty postgres user, got nil")
	}
	if !strings.Contains(err.Error(), "database.postgres.user") {
		t.Fatalf("Load() error = %v, want contains %q", err, "database.postgres.user")
	}

	path = writeTestConfig(t, postgresYAML("localhost", "admin", ""))
	_, err = Load(path)
	if err == nil {
		t.Fatal("Load() expected error for empty postgres dbname, got nil")
	}
	if !strings.Contains(err.Error(), "database.postgres.dbname") {
		t.Fatalf("Load() error = %v, want contains %q", err, "database.postgres.dbname")
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
	postgresYAML := func(mode, sslmode string) string {
		return fmt.Sprintf("server:\n"+
			"  host: \"127.0.0.1\"\n"+
			"  port: 3000\n"+
			"  mode: %q\n"+
			"  csrf_secret: \"TestCsrf!Secret#2024xHereValid!!\"\n"+
			"database:\n"+
			"  driver: \"postgres\"\n"+
			"  postgres:\n"+
			"    host: \"localhost\"\n"+
			"    port: 5432\n"+
			"    user: \"admin\"\n"+
			"    password: \"secret\"\n"+
			"    dbname: \"testdb\"\n"+
			"    sslmode: %q\n"+
			"  pool:\n"+
			"    max_idle_conns: 1\n"+
			"    max_open_conns: 1\n"+
			"    conn_max_lifetime: \"1m\"\n"+
			"log:\n"+
			"  level: \"info\"\n"+
			"  format: \"json\"\n", mode, sslmode)
	}

	tests := []struct {
		name        string
		mode        string
		sslmode     string
		wantErr     bool
		wantContain string
	}{
		{
			name:        "release rejects insecure sslmode disable",
			mode:        "release",
			sslmode:     "disable",
			wantErr:     true,
			wantContain: "database.postgres.sslmode",
		},
		{
			name:    "release allows require",
			mode:    "release",
			sslmode: "require",
		},
		{
			name:    "release allows verify-ca",
			mode:    "release",
			sslmode: "verify-ca",
		},
		{
			name:    "release allows verify-full",
			mode:    "release",
			sslmode: "verify-full",
		},
		{
			name:    "debug allows disable",
			mode:    "debug",
			sslmode: "disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestConfig(t, postgresYAML(tt.mode, tt.sslmode))
			_, err := Load(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Load() expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantContain) {
					t.Fatalf("Load() error = %v, want contains %q", err, tt.wantContain)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
		})
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
			name: "server timeout must be valid duration",
			yaml: `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
  timeout: "not-a-duration"
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
			name: "cors max age must be valid duration",
			yaml: `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
  cors:
    max_age: "oops"
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
		{
			name: "pool lifetime rejects negative",
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
    conn_max_lifetime: "-5m"
log:
  level: "info"
  format: "json"
`,
			wantContain: "database.pool.conn_max_lifetime",
		},
		{
			name: "pool lifetime must be valid duration",
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
    conn_max_lifetime: "not-a-duration"
log:
  level: "info"
  format: "json"
`,
			wantContain: "database.pool.conn_max_lifetime",
		},
		{
			name: "pool idle time must be positive",
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
    conn_max_lifetime: "1m"
    conn_max_idle_time: "0s"
log:
  level: "info"
  format: "json"
`,
			wantContain: "database.pool.conn_max_idle_time",
		},
		{
			name: "pool idle time rejects negative",
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
    conn_max_lifetime: "1m"
    conn_max_idle_time: "-5m"
log:
  level: "info"
  format: "json"
`,
			wantContain: "database.pool.conn_max_idle_time",
		},
		{
			name: "pool idle time rejects invalid",
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
    conn_max_lifetime: "1m"
    conn_max_idle_time: "abc"
log:
  level: "info"
  format: "json"
`,
			wantContain: "database.pool.conn_max_idle_time",
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

func TestLoad_InvalidLogConfig(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		wantContain string
	}{
		{
			name: "invalid log level",
			yaml: `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
  csrf_secret: "TestCsrf!Secret#2024xHereValid!!"
database:
  driver: "sqlite"
  sqlite:
    path: "data/test.db"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "verbose"
  format: "json"
`,
			wantContain: "log.level",
		},
		{
			name: "invalid log format",
			yaml: `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
  csrf_secret: "TestCsrf!Secret#2024xHereValid!!"
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
  format: "yaml"
`,
			wantContain: "log.format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestConfig(t, tt.yaml)
			_, err := Load(path)
			if err == nil {
				t.Fatal("Load() expected error for invalid log config, got nil")
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
  csrf_secret: "TestCsrf!Secret#2024xHereValid!!"
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
    conn_max_idle_time: "   "
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
	if cfg.Database.Pool.ConnMaxIdleTime != "" {
		t.Errorf("Database.Pool.ConnMaxIdleTime = %q, want empty string", cfg.Database.Pool.ConnMaxIdleTime)
	}
}

func TestLoad_PoolValidation(t *testing.T) {
	poolYAML := func(idle, open int) string {
		return fmt.Sprintf(`server:
  host: "127.0.0.1"
  port: 3000
  mode: "debug"
database:
  driver: "sqlite"
  sqlite:
    path: "data/test.db"
  pool:
    max_idle_conns: %d
    max_open_conns: %d
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
`, idle, open)
	}

	releasePoolYAML := func(idle, open int) string {
		return fmt.Sprintf(`server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
  csrf_secret: "TestCsrf!Secret#2024xHereValid!!"
database:
  driver: "sqlite"
  sqlite:
    path: "data/test.db"
  pool:
    max_idle_conns: %d
    max_open_conns: %d
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
`, idle, open)
	}

	tests := []struct {
		name        string
		yaml        string
		wantErr     bool
		wantContain string
	}{
		{
			name:        "negative max_idle_conns",
			yaml:        poolYAML(-1, 50),
			wantErr:     true,
			wantContain: "max_idle_conns",
		},
		{
			name:        "negative max_open_conns",
			yaml:        poolYAML(5, -10),
			wantErr:     true,
			wantContain: "max_open_conns",
		},
		{
			name:        "idle exceeds open",
			yaml:        poolYAML(20, 10),
			wantErr:     true,
			wantContain: "must not exceed",
		},
		{
			name:    "zero values use defaults (valid)",
			yaml:    poolYAML(0, 0), // zero → defaults (idle=10, open=100)
			wantErr: false,
		},
		{
			name:        "zero idle with low open (effective idle exceeds open)",
			yaml:        poolYAML(0, 5), // zero idle → default 10, explicit open 5 → 10 > 5
			wantErr:     true,
			wantContain: "must not exceed",
		},
		{
			name:        "release mode zero idle with low open (effective idle exceeds open)",
			yaml:        releasePoolYAML(0, 5),
			wantErr:     true,
			wantContain: "must not exceed",
		},
		{
			name:    "idle equals open (valid)",
			yaml:    poolYAML(5, 5),
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

func TestLoad_CacheConfig(t *testing.T) {
	base := func(cacheBlock string) string {
		return `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
  csrf_secret: "TestCsrf!Secret#2024xHereValid!!"
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

func loadProjectDefaultConfig(t *testing.T) *Config {
	t.Helper()
	clearAPPEnv(t)
	cfg, err := Load("../../configs/config.yaml")
	if err != nil {
		t.Fatalf("Load() error on project config: %v", err)
	}
	return cfg
}

func TestLoad_DefaultConfig(t *testing.T) {
	t.Run("server and database defaults", func(t *testing.T) {
		cfg := loadProjectDefaultConfig(t)

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
	})

	t.Run("auth defaults are populated and accessible", func(t *testing.T) {
		cfg := loadProjectDefaultConfig(t)

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
		if cfg.Auth.CookieSecure == nil {
			t.Fatal("Auth.CookieSecure is nil, want non-nil")
		}
		if *cfg.Auth.CookieSecure {
			t.Errorf("Auth.CookieSecure = %v, want false (debug mode default)", *cfg.Auth.CookieSecure)
		}
	})

	t.Run("missing auth public_paths is rejected by parser+validator", func(t *testing.T) {
		path := writeTestConfig(t, `server:
  host: "127.0.0.1"
  port: 8080
  mode: "debug"
database:
  driver: "sqlite"
  sqlite:
    path: "data/app.db"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "debug"
  format: "text"
auth:
  enabled: true
  jwt_secret: "abcdefghijklmnopqrstuvwxyz123456"
  token_expiry: "24h"
`)

		_, err := Load(path)
		if err == nil {
			t.Fatal("Load() expected error for missing auth.public_paths, got nil")
		}
		if !strings.Contains(err.Error(), "auth.public_paths") {
			t.Fatalf("Load() error = %v, want contains %q", err, "auth.public_paths")
		}
	})
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
  csrf_secret: "TestCsrf!Secret#2024xHereValid!!"
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

func TestLoad_AuthCookieSecure(t *testing.T) {
	validAuthDebug := func(extra string) string {
		return validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n" + extra)
	}
	validAuthRelease := func(extra string) string {
		return validReleaseBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"Abcd1234!Abcd1234!Abcd1234!Abcd1234!\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n" + extra)
	}

	t.Run("debug mode nil defaults to false", func(t *testing.T) {
		path := writeTestConfig(t, validAuthDebug(""))
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if cfg.Auth.CookieSecure == nil {
			t.Fatal("CookieSecure is nil, want non-nil")
		}
		if *cfg.Auth.CookieSecure {
			t.Errorf("CookieSecure = true, want false in debug mode")
		}
	})

	t.Run("release mode nil defaults to true", func(t *testing.T) {
		path := writeTestConfig(t, validAuthRelease(""))
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if cfg.Auth.CookieSecure == nil {
			t.Fatal("CookieSecure is nil, want non-nil")
		}
		if !*cfg.Auth.CookieSecure {
			t.Errorf("CookieSecure = false, want true in release mode")
		}
	})

	t.Run("debug mode explicit false accepted", func(t *testing.T) {
		path := writeTestConfig(t, validAuthDebug("  cookie_secure: false\n"))
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if cfg.Auth.CookieSecure == nil || *cfg.Auth.CookieSecure {
			t.Errorf("CookieSecure = %v, want false", cfg.Auth.CookieSecure)
		}
	})

	t.Run("debug mode explicit true accepted", func(t *testing.T) {
		path := writeTestConfig(t, validAuthDebug("  cookie_secure: true\n"))
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if cfg.Auth.CookieSecure == nil || !*cfg.Auth.CookieSecure {
			t.Errorf("CookieSecure = %v, want true", cfg.Auth.CookieSecure)
		}
	})

	t.Run("release mode explicit true accepted", func(t *testing.T) {
		path := writeTestConfig(t, validAuthRelease("  cookie_secure: true\n"))
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if cfg.Auth.CookieSecure == nil || !*cfg.Auth.CookieSecure {
			t.Errorf("CookieSecure = %v, want true", cfg.Auth.CookieSecure)
		}
	})

	t.Run("release mode explicit false rejected", func(t *testing.T) {
		path := writeTestConfig(t, validAuthRelease("  cookie_secure: false\n"))
		_, err := Load(path)
		if err == nil {
			t.Fatal("Load() expected error, got nil")
		}
		if !strings.Contains(err.Error(), "auth.cookie_secure") {
			t.Fatalf("Load() error = %v, want contains %q", err, "auth.cookie_secure")
		}
	})

	t.Run("auth disabled leaves CookieSecure nil", func(t *testing.T) {
		path := writeTestConfig(t, validBaseYAML("auth:\n  enabled: false\n"))
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if cfg.Auth.CookieSecure != nil {
			t.Errorf("CookieSecure = %v, want nil when auth disabled", cfg.Auth.CookieSecure)
		}
	})
}

func TestLoad_AuthConfig(t *testing.T) {
	tests := []struct {
		name          string
		yaml          string
		wantErr       bool
		wantContain   string
		wantPathCount int // if >0, assert len(cfg.Auth.PublicPaths)
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
			name:        "auth enabled with empty public_paths entry",
			yaml:        validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"\"\n    - \"/api/v1/auth/register\"\n"),
			wantErr:     true,
			wantContain: "auth.public_paths[0]",
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
			name:          "auth enabled with duplicate public_paths deduplicates",
			yaml:          validBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"abcdefghijklmnopqrstuvwxyz123456\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n"),
			wantPathCount: 2,
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
			cfg, err := Load(path)
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
				if tt.wantPathCount > 0 {
					if len(cfg.Auth.PublicPaths) != tt.wantPathCount {
						t.Errorf("len(PublicPaths) = %d, want %d", len(cfg.Auth.PublicPaths), tt.wantPathCount)
					}
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

func TestLoad_CSRFSecretValidation(t *testing.T) {
	releaseNoCSRF := func(csrfLine string) string {
		return fmt.Sprintf(`server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
%sdatabase:
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
`, csrfLine)
	}

	tests := []struct {
		name         string
		yaml         string
		wantErr      bool
		wantContains []string
	}{
		{
			name:         "release mode rejects empty csrf_secret",
			yaml:         releaseNoCSRF("  csrf_secret: \"\"\n"),
			wantErr:      true,
			wantContains: []string{"server.csrf_secret", "is required when server.mode"},
		},
		{
			name:         "release mode rejects missing csrf_secret",
			yaml:         releaseNoCSRF(""),
			wantErr:      true,
			wantContains: []string{"server.csrf_secret", "is required when server.mode"},
		},
		{
			name:         "release mode rejects short csrf_secret",
			yaml:         releaseNoCSRF("  csrf_secret: \"tooshort\"\n"),
			wantErr:      true,
			wantContains: []string{"server.csrf_secret", "at least 32 characters"},
		},
		{
			name:         "release mode rejects low complexity csrf_secret",
			yaml:         releaseNoCSRF("  csrf_secret: \"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\"\n"),
			wantErr:      true,
			wantContains: []string{"server.csrf_secret", "at least 3 character classes"},
		},
		{
			name:         "release mode rejects placeholder csrf_secret",
			yaml:         releaseNoCSRF("  csrf_secret: \"change-me-to-a-random-secret\"\n"),
			wantErr:      true,
			wantContains: []string{"server.csrf_secret", "placeholder/default values"},
		},
		{
			name:    "release mode accepts valid csrf_secret",
			yaml:    validReleaseBaseYAML(""),
			wantErr: false,
		},
		{
			name:    "debug mode allows empty csrf_secret",
			yaml:    validBaseYAML(""),
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
				for _, want := range tt.wantContains {
					if !strings.Contains(err.Error(), want) {
						t.Fatalf("Load() error = %v, want contains %q", err, want)
					}
				}
			} else {
				if err != nil {
					t.Fatalf("Load() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestLoad_ServerRateLimitValidation(t *testing.T) {
	base := func(rateLimitBlock string) string {
		return `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
  csrf_secret: "TestCsrf!Secret#2024xHereValid!!"
` + rateLimitBlock + `
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
		name           string
		rateLimitBlock string
		wantErr        bool
		wantContain    string
	}{
		{
			name: "enabled with non-positive rps",
			rateLimitBlock: `  rate_limit:
    enabled: true
    rps: 0
    burst: 10`,
			wantErr:     true,
			wantContain: "server.rate_limit.rps",
		},
		{
			name: "enabled with non-positive burst",
			rateLimitBlock: `  rate_limit:
    enabled: true
    rps: 5
    burst: 0`,
			wantErr:     true,
			wantContain: "server.rate_limit.burst",
		},
		{
			name: "enabled with valid settings",
			rateLimitBlock: `  rate_limit:
    enabled: true
    rps: 5
    burst: 10`,
			wantErr: false,
		},
		{
			name: "disabled skips validation",
			rateLimitBlock: `  rate_limit:
    enabled: false
    rps: 0
    burst: 0`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestConfig(t, base(tt.rateLimitBlock))
			_, err := Load(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Load() expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantContain) {
					t.Fatalf("Load() error = %v, want contains %q", err, tt.wantContain)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
		})
	}
}

func TestLoad_ReleaseSecurityChecks(t *testing.T) {
	releasePostgres := func(password string) string {
		return `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
  csrf_secret: "TestCsrf!Secret#2024xHereValid!!"
database:
  driver: "postgres"
  postgres:
    host: "localhost"
    port: 5432
    user: "admin"
    password: "` + password + `"
    dbname: "testdb"
    sslmode: "require"
  pool:
    max_idle_conns: 1
    max_open_conns: 1
    conn_max_lifetime: "1m"
log:
  level: "info"
  format: "json"
`
	}

	releaseAuthJWT := func(jwtSecret string) string {
		return validReleaseBaseYAML("auth:\n  enabled: true\n  jwt_secret: \"" + jwtSecret + "\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n")
	}

	tests := []struct {
		name        string
		yaml        string
		wantErr     bool
		wantContain []string
	}{
		{
			name:        "release postgres rejects empty password",
			yaml:        releasePostgres(""),
			wantErr:     true,
			wantContain: []string{"database.postgres.password"},
		},
		{
			name:        "release postgres rejects placeholder password",
			yaml:        releasePostgres("password"),
			wantErr:     true,
			wantContain: []string{"database.postgres.password", "placeholder/default values"},
		},
		{
			name:        "release postgres rejects placeholder password gobase",
			yaml:        releasePostgres("gobase"),
			wantErr:     true,
			wantContain: []string{"database.postgres.password", "placeholder/default values"},
		},
		{
			name:    "release postgres accepts real password",
			yaml:    releasePostgres("my-S3cur3-Pr0d-P@ss!"),
			wantErr: false,
		},
		{
			name:        "release auth rejects placeholder jwt_secret",
			yaml:        releaseAuthJWT("gobase-dev-jwt-secret-key-change-me"),
			wantErr:     true,
			wantContain: []string{"auth.jwt_secret", "placeholder/default values"},
		},
		{
			name:        "release auth rejects short jwt_secret",
			yaml:        releaseAuthJWT("change-me-in-env"),
			wantErr:     true,
			wantContain: []string{"auth.jwt_secret", "must be at least 32 characters"},
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
				for _, wantContain := range tt.wantContain {
					if !strings.Contains(err.Error(), wantContain) {
						t.Fatalf("Load() error = %v, want contains %q", err, wantContain)
					}
				}
			} else {
				if err != nil {
					t.Fatalf("Load() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestLoad_ReleaseErrors_DoNotLeakSecrets(t *testing.T) {
	t.Run("postgres placeholder password not in error message", func(t *testing.T) {
		password := "gobase"
		path := writeTestConfig(t, `server:
  host: "127.0.0.1"
  port: 3000
  mode: "release"
  csrf_secret: "TestCsrf!Secret#2024xHereValid!!"
database:
  driver: "postgres"
  postgres:
    host: "localhost"
    port: 5432
    user: "admin"
    password: "`+password+`"
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
			t.Fatal("Load() expected error for placeholder postgres password in release mode, got nil")
		}
		if !strings.Contains(err.Error(), "placeholder/default values") {
			t.Fatalf("Load() error = %v, want contains %q", err, "placeholder/default values")
		}
		if strings.Contains(err.Error(), password) {
			t.Fatalf("Load() error leaked sensitive password value: %v", err)
		}
	})

	t.Run("jwt_secret placeholder not in error message", func(t *testing.T) {
		secret := "gobase-dev-jwt-secret-key-change-me"
		path := writeTestConfig(t, validReleaseBaseYAML("auth:\n  enabled: true\n  jwt_secret: \""+secret+"\"\n  token_expiry: \"24h\"\n  public_paths:\n    - \"/api/v1/auth/login\"\n    - \"/api/v1/auth/register\"\n"))

		_, err := Load(path)
		if err == nil {
			t.Fatal("Load() expected error for placeholder jwt_secret in release mode, got nil")
		}
		if !strings.Contains(err.Error(), "auth.jwt_secret") {
			t.Fatalf("Load() error = %v, want contains %q", err, "auth.jwt_secret")
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("Load() error leaked sensitive jwt_secret value: %v", err)
		}
	})
}
