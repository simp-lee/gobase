package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupDatabase_SQLite(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := &DatabaseConfig{
		Driver: "sqlite",
		SQLite: SQLiteConfig{Path: dbPath},
		Pool: PoolConfig{
			MaxIdleConns:    5,
			MaxOpenConns:    50,
			ConnMaxLifetime: "30m",
		},
	}

	db, err := SetupDatabase(cfg, logger)
	if err != nil {
		t.Fatalf("SetupDatabase() error = %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB() error = %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}

	stats := sqlDB.Stats()
	if stats.MaxOpenConnections != 50 {
		t.Errorf("MaxOpenConnections = %d; want 50", stats.MaxOpenConnections)
	}
}

func TestSetupDatabase_PoolDefaults(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg := &DatabaseConfig{
		Driver: "sqlite",
		SQLite: SQLiteConfig{Path: dbPath},
		Pool:   PoolConfig{}, // all zeros → defaults
	}

	db, err := SetupDatabase(cfg, logger)
	if err != nil {
		t.Fatalf("SetupDatabase() error = %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB() error = %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	stats := sqlDB.Stats()
	if stats.MaxOpenConnections != 100 {
		t.Errorf("MaxOpenConnections = %d; want 100 (default)", stats.MaxOpenConnections)
	}
}

func TestSetupDatabase_UnsupportedDriver(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := &DatabaseConfig{Driver: "mysql"}

	_, err := SetupDatabase(cfg, logger)
	if err == nil {
		t.Fatal("SetupDatabase() expected error for unsupported driver, got nil")
	}

	want := `unsupported database driver: mysql`
	if err.Error() != want {
		t.Errorf("error = %q; want %q", err.Error(), want)
	}
}

func TestSetupDatabase_InvalidConnMaxLifetime(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := &DatabaseConfig{
		Driver: "sqlite",
		SQLite: SQLiteConfig{Path: dbPath},
		Pool: PoolConfig{
			MaxIdleConns:    5,
			MaxOpenConns:    50,
			ConnMaxLifetime: "not-a-duration",
		},
	}

	_, err := SetupDatabase(cfg, logger)
	if err == nil {
		t.Fatal("SetupDatabase() expected error for invalid duration, got nil")
	}
	if !strings.Contains(err.Error(), "pool.conn_max_lifetime") {
		t.Fatalf("SetupDatabase() error = %v, want contains %q", err, "pool.conn_max_lifetime")
	}
	if !strings.Contains(err.Error(), "invalid duration") {
		t.Fatalf("SetupDatabase() error = %v, want contains %q", err, "invalid duration")
	}
}

func TestSetupDatabase_NonPositiveConnMaxLifetime(t *testing.T) {
	tests := []struct {
		name       string
		connMaxTTL string
	}{
		{name: "negative duration", connMaxTTL: "-1s"},
		{name: "zero duration", connMaxTTL: "0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			dbPath := filepath.Join(dir, "test.db")
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

			cfg := &DatabaseConfig{
				Driver: "sqlite",
				SQLite: SQLiteConfig{Path: dbPath},
				Pool: PoolConfig{
					MaxIdleConns:    5,
					MaxOpenConns:    50,
					ConnMaxLifetime: tt.connMaxTTL,
				},
			}

			_, err := SetupDatabase(cfg, logger)
			if err == nil {
				t.Fatalf("SetupDatabase() expected error for conn_max_lifetime=%q, got nil", tt.connMaxTTL)
			}
			if !strings.Contains(err.Error(), "pool.conn_max_lifetime") {
				t.Fatalf("SetupDatabase() error = %v, want contains %q", err, "pool.conn_max_lifetime")
			}
			if !strings.Contains(err.Error(), "must be greater than 0") {
				t.Fatalf("SetupDatabase() error = %v, want contains %q", err, "must be greater than 0")
			}
		})
	}
}

func TestSetupDatabase_DebugLoggerConnection(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := &DatabaseConfig{
		Driver: "sqlite",
		SQLite: SQLiteConfig{Path: dbPath},
		Pool: PoolConfig{
			MaxIdleConns:    5,
			MaxOpenConns:    20,
			ConnMaxLifetime: "10m",
		},
	}

	db, err := SetupDatabase(cfg, logger)
	if err != nil {
		t.Fatalf("SetupDatabase() error = %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB() error = %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	// This test validates that setup succeeds with a debug-level logger.
	// SQL log verbosity mapping is internal to GORM logger construction.
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
}

func TestEffectiveDefaults(t *testing.T) {
	if got := effectiveMaxIdleConns(0); got != 10 {
		t.Errorf("effectiveMaxIdleConns(0) = %d; want 10", got)
	}
	if got := effectiveMaxIdleConns(5); got != 5 {
		t.Errorf("effectiveMaxIdleConns(5) = %d; want 5", got)
	}
	// Negative values pass through (rejected by configurePool/Validate, not here).
	if got := effectiveMaxIdleConns(-1); got != -1 {
		t.Errorf("effectiveMaxIdleConns(-1) = %d; want -1", got)
	}
	if got := effectiveMaxOpenConns(0); got != 100 {
		t.Errorf("effectiveMaxOpenConns(0) = %d; want 100", got)
	}
	if got := effectiveMaxOpenConns(50); got != 50 {
		t.Errorf("effectiveMaxOpenConns(50) = %d; want 50", got)
	}
	if got := effectiveMaxOpenConns(-5); got != -5 {
		t.Errorf("effectiveMaxOpenConns(-5) = %d; want -5", got)
	}
	if got := effectiveConnMaxLifetime(""); got != "1h" {
		t.Errorf("effectiveConnMaxLifetime(\"\") = %q; want \"1h\"", got)
	}
	if got := effectiveConnMaxLifetime("   "); got != "1h" {
		t.Errorf("effectiveConnMaxLifetime(\"   \") = %q; want \"1h\"", got)
	}
	if got := effectiveConnMaxLifetime("30m"); got != "30m" {
		t.Errorf("effectiveConnMaxLifetime(\"30m\") = %q; want \"30m\"", got)
	}
	if got := effectiveConnMaxIdleTime(""); got != "10m" {
		t.Errorf("effectiveConnMaxIdleTime(\"\") = %q; want \"10m\"", got)
	}
	if got := effectiveConnMaxIdleTime("   "); got != "10m" {
		t.Errorf("effectiveConnMaxIdleTime(\"   \") = %q; want \"10m\"", got)
	}
	if got := effectiveConnMaxIdleTime("5m"); got != "5m" {
		t.Errorf("effectiveConnMaxIdleTime(\"5m\") = %q; want \"5m\"", got)
	}
}

func TestSetupDatabase_NegativeMaxIdleConns(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := &DatabaseConfig{
		Driver: "sqlite",
		SQLite: SQLiteConfig{Path: dbPath},
		Pool: PoolConfig{
			MaxIdleConns:    -1,
			MaxOpenConns:    50,
			ConnMaxLifetime: "30m",
		},
	}

	_, err := SetupDatabase(cfg, logger)
	if err == nil {
		t.Fatal("SetupDatabase() expected error for negative MaxIdleConns, got nil")
	}
	if !strings.Contains(err.Error(), "pool.max_idle_conns") {
		t.Errorf("error = %v, want contains %q", err, "pool.max_idle_conns")
	}
}

func TestSetupDatabase_InvalidConnMaxIdleTime(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := &DatabaseConfig{
		Driver: "sqlite",
		SQLite: SQLiteConfig{Path: dbPath},
		Pool: PoolConfig{
			MaxIdleConns:    5,
			MaxOpenConns:    50,
			ConnMaxLifetime: "30m",
			ConnMaxIdleTime: "not-a-duration",
		},
	}

	_, err := SetupDatabase(cfg, logger)
	if err == nil {
		t.Fatal("SetupDatabase() expected error for invalid ConnMaxIdleTime, got nil")
	}
	if !strings.Contains(err.Error(), "pool.conn_max_idle_time") {
		t.Errorf("error = %v, want contains %q", err, "pool.conn_max_idle_time")
	}
	if !strings.Contains(err.Error(), "invalid duration") {
		t.Fatalf("SetupDatabase() error = %v, want contains %q", err, "invalid duration")
	}
}

func TestSetupDatabase_NonPositiveConnMaxIdleTime(t *testing.T) {
	tests := []struct {
		name          string
		connMaxIdleTT string
	}{
		{name: "negative duration", connMaxIdleTT: "-1s"},
		{name: "zero duration", connMaxIdleTT: "0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			dbPath := filepath.Join(dir, "test.db")
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

			cfg := &DatabaseConfig{
				Driver: "sqlite",
				SQLite: SQLiteConfig{Path: dbPath},
				Pool: PoolConfig{
					MaxIdleConns:    5,
					MaxOpenConns:    50,
					ConnMaxLifetime: "30m",
					ConnMaxIdleTime: tt.connMaxIdleTT,
				},
			}

			_, err := SetupDatabase(cfg, logger)
			if err == nil {
				t.Fatalf("SetupDatabase() expected error for conn_max_idle_time=%q, got nil", tt.connMaxIdleTT)
			}
			if !strings.Contains(err.Error(), "pool.conn_max_idle_time") {
				t.Fatalf("SetupDatabase() error = %v, want contains %q", err, "pool.conn_max_idle_time")
			}
			if !strings.Contains(err.Error(), "must be greater than 0") {
				t.Fatalf("SetupDatabase() error = %v, want contains %q", err, "must be greater than 0")
			}
		})
	}
}

func TestSetupDatabase_PostgresConnectFailure_SanitizesPassword(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := &DatabaseConfig{
		Driver: "postgres",
		Postgres: PostgresConfig{
			Host:     "127.0.0.1",
			Port:     1, // use a guaranteed non-Postgres port to force fast connect failure
			User:     "postgres",
			Password: "very-secret-password",
			DBName:   "app",
			SSLMode:  "disable",
		},
		Pool: PoolConfig{
			MaxIdleConns:    1,
			MaxOpenConns:    1,
			ConnMaxLifetime: "1m",
		},
	}

	_, err := SetupDatabase(cfg, logger)
	if err == nil {
		t.Fatal("SetupDatabase() expected postgres connection error, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "failed to connect to postgres database") {
		t.Fatalf("SetupDatabase() error = %q, want contains %q", errMsg, "failed to connect to postgres database")
	}
	if !strings.Contains(errMsg, "host=127.0.0.1") {
		t.Fatalf("SetupDatabase() error = %q, want contains %q", errMsg, "host=127.0.0.1")
	}
	if !strings.Contains(errMsg, "[REDACTED]") {
		t.Fatalf("SetupDatabase() error = %q, want contains %q", errMsg, "[REDACTED]")
	}
	if strings.Contains(errMsg, cfg.Postgres.Password) {
		t.Fatalf("SetupDatabase() error leaked raw password: %q", errMsg)
	}
}

func TestSetupDatabase_NegativeMaxOpenConns(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := &DatabaseConfig{
		Driver: "sqlite",
		SQLite: SQLiteConfig{Path: dbPath},
		Pool: PoolConfig{
			MaxIdleConns:    5,
			MaxOpenConns:    -10,
			ConnMaxLifetime: "30m",
		},
	}

	_, err := SetupDatabase(cfg, logger)
	if err == nil {
		t.Fatal("SetupDatabase() expected error for negative MaxOpenConns, got nil")
	}
	if !strings.Contains(err.Error(), "pool.max_open_conns") {
		t.Errorf("error = %v, want contains %q", err, "pool.max_open_conns")
	}
}

func TestSetupDatabase_NilConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	_, err := SetupDatabase(nil, logger)
	if err == nil {
		t.Fatal("SetupDatabase(nil, logger) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "database config is nil") {
		t.Errorf("error = %q; want to contain %q", err.Error(), "database config is nil")
	}
}

func TestSetupDatabase_NilLogger(t *testing.T) {
	cfg := &DatabaseConfig{Driver: "sqlite"}

	_, err := SetupDatabase(cfg, nil)
	if err == nil {
		t.Fatal("SetupDatabase(cfg, nil) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "logger is nil") {
		t.Errorf("error = %q; want to contain %q", err.Error(), "logger is nil")
	}
}

func TestBuildPostgresDSN(t *testing.T) {
	tests := []struct {
		name string
		cfg  *PostgresConfig
		want string
	}{
		{
			name: "full config",
			cfg: &PostgresConfig{
				Host:     "localhost",
				Port:     5432,
				User:     "admin",
				Password: "secret",
				DBName:   "mydb",
				SSLMode:  "require",
			},
			want: "postgres://admin:secret@localhost:5432/mydb?sslmode=require",
		},
		{
			name: "empty password",
			cfg: &PostgresConfig{
				Host:    "db.example.com",
				Port:    5432,
				User:    "admin",
				DBName:  "mydb",
				SSLMode: "disable",
			},
			want: "postgres://admin:@db.example.com:5432/mydb?sslmode=disable",
		},
		{
			name: "empty sslmode omits query param",
			cfg: &PostgresConfig{
				Host:     "localhost",
				Port:     5432,
				User:     "user",
				Password: "pass",
				DBName:   "testdb",
				SSLMode:  "",
			},
			want: "postgres://user:pass@localhost:5432/testdb",
		},
		{
			name: "no user no password",
			cfg: &PostgresConfig{
				Host:    "localhost",
				Port:    5432,
				DBName:  "testdb",
				SSLMode: "disable",
			},
			want: "postgres://localhost:5432/testdb?sslmode=disable",
		},
		{
			name: "password with special characters",
			cfg: &PostgresConfig{
				Host:     "localhost",
				Port:     5432,
				User:     "admin",
				Password: "p@ss:word/special",
				DBName:   "mydb",
				SSLMode:  "require",
			},
			want: "postgres://admin:p%40ss%3Aword%2Fspecial@localhost:5432/mydb?sslmode=require",
		},
		{
			name: "nil config returns empty",
			cfg:  nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPostgresDSN(tt.cfg)
			if got != tt.want {
				t.Errorf("buildPostgresDSN() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizePostgresConnectError(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *PostgresConfig
		err     error
		wantNil bool
		wantSub string // substring that MUST appear
		wantNo  string // substring that MUST NOT appear (password)
	}{
		{
			name:    "nil error returns nil",
			cfg:     &PostgresConfig{Password: "secret"},
			err:     nil,
			wantNil: true,
		},
		{
			name:    "nil config still wraps",
			cfg:     nil,
			err:     errors.New("connection refused"),
			wantSub: "failed to connect to postgres database",
		},
		{
			name: "password redacted from plain text",
			cfg: &PostgresConfig{
				Host:     "db.example.com",
				Port:     5432,
				DBName:   "mydb",
				SSLMode:  "require",
				User:     "admin",
				Password: "sup3r$ecret!",
			},
			err:     errors.New("failed to connect: password authentication failed for user admin with password sup3r$ecret!"),
			wantSub: "[REDACTED]",
			wantNo:  "sup3r$ecret!",
		},
		{
			name: "url-encoded password redacted",
			cfg: &PostgresConfig{
				Host:     "db.example.com",
				Port:     5432,
				DBName:   "mydb",
				SSLMode:  "require",
				User:     "admin",
				Password: "p@ss w0rd",
			},
			err:     fmt.Errorf("dsn contains p%%40ss+w0rd"),
			wantSub: "[REDACTED]",
			wantNo:  "p@ss w0rd",
		},
		{
			name: "empty password no panic",
			cfg: &PostgresConfig{
				Host:    "localhost",
				Port:    5432,
				DBName:  "test",
				SSLMode: "disable",
			},
			err:     errors.New("connection refused"),
			wantSub: "connection refused",
		},
		{
			name: "host and port appear in wrapped message",
			cfg: &PostgresConfig{
				Host:     "db.example.com",
				Port:     5432,
				DBName:   "mydb",
				SSLMode:  "require",
				Password: "secret",
			},
			err:     errors.New("timeout"),
			wantSub: "host=db.example.com",
			wantNo:  "secret",
		},
		{
			name: "error is not wrapped (no Unwrap chain)",
			cfg: &PostgresConfig{
				Host:     "localhost",
				Port:     5432,
				DBName:   "mydb",
				Password: "secret",
			},
			err:     fmt.Errorf("inner: password=secret"),
			wantSub: "[REDACTED]",
			wantNo:  "secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizePostgresConnectError(tt.cfg, tt.err)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("got %v; want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("got nil; want non-nil error")
			}
			msg := got.Error()
			if tt.wantSub != "" && !strings.Contains(msg, tt.wantSub) {
				t.Errorf("error %q does not contain %q", msg, tt.wantSub)
			}
			if tt.wantNo != "" && strings.Contains(msg, tt.wantNo) {
				t.Errorf("error %q still contains password %q", msg, tt.wantNo)
			}
			// Verify the error does NOT wrap the original (no Unwrap chain).
			if uw, ok := got.(interface{ Unwrap() error }); ok {
				t.Errorf("sanitized error should not be unwrappable, but Unwrap() returned %v", uw.Unwrap())
			}
		})
	}
}
