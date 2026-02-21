package config

import (
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
}

func TestSetupDatabase_NonPositiveConnMaxLifetime(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := &DatabaseConfig{
		Driver: "sqlite",
		SQLite: SQLiteConfig{Path: dbPath},
		Pool: PoolConfig{
			MaxIdleConns:    5,
			MaxOpenConns:    50,
			ConnMaxLifetime: "-1s",
		},
	}

	_, err := SetupDatabase(cfg, logger)
	if err == nil {
		t.Fatal("SetupDatabase() expected error for non-positive duration, got nil")
	}
	if !strings.Contains(err.Error(), "pool.conn_max_lifetime") {
		t.Fatalf("SetupDatabase() error = %v, want contains %q", err, "pool.conn_max_lifetime")
	}
}

func TestSetupDatabase_DebugLogMode(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	// Debug-level logger → GORM should use Info log mode (logs all SQL).
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

	// Just verify the connection works — log mode is an internal GORM setting
	// that cannot be introspected easily; we verify it doesn't error.
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
	if got := effectiveMaxOpenConns(0); got != 100 {
		t.Errorf("effectiveMaxOpenConns(0) = %d; want 100", got)
	}
	if got := effectiveMaxOpenConns(50); got != 50 {
		t.Errorf("effectiveMaxOpenConns(50) = %d; want 50", got)
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
}
