package config

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/simp-lee/logger"
)

func boolPtr(b bool) *bool { return &b }

func TestSetupLogger_LevelMapping(t *testing.T) {
	tests := []struct {
		name      string
		level     string
		wantLevel slog.Level
	}{
		{"debug level", "debug", slog.LevelDebug},
		{"info level", "info", slog.LevelInfo},
		{"warn level", "warn", slog.LevelWarn},
		{"error level", "error", slog.LevelError},
		{"uppercase DEBUG", "DEBUG", slog.LevelDebug},
		{"mixed case Info", "Info", slog.LevelInfo},
		{"invalid defaults to info", "invalid", slog.LevelInfo},
		{"empty defaults to info", "", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, err := SetupLogger(&LogConfig{Level: tt.level, Format: "text"})
			if err != nil {
				t.Fatalf("SetupLogger error: %v", err)
			}
			defer log.Close()

			if log == nil {
				t.Fatal("SetupLogger returned nil")
			}
			if !log.Enabled(context.TODO(), tt.wantLevel) {
				t.Errorf("expected level %v to be enabled", tt.wantLevel)
			}
			// Verify that levels below the configured level are disabled.
			if tt.wantLevel > slog.LevelDebug {
				belowLevel := tt.wantLevel - 1
				if log.Enabled(context.TODO(), belowLevel) {
					t.Errorf("expected level %v to be disabled (configured: %v)", belowLevel, tt.wantLevel)
				}
			}
		})
	}
}

func TestSetupLogger_ConsoleOnly(t *testing.T) {
	log, err := SetupLogger(&LogConfig{Level: "info", Format: "text"})
	if err != nil {
		t.Fatalf("SetupLogger error: %v", err)
	}
	defer log.Close()

	if log == nil {
		t.Fatal("SetupLogger returned nil")
	}
}

func TestSetupLogger_ConsoleAndFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.log")

	log, err := SetupLogger(&LogConfig{
		Level:    "info",
		Format:   "json",
		FilePath: filePath,
	})
	if err != nil {
		t.Fatalf("SetupLogger error: %v", err)
	}
	defer log.Close()

	if log == nil {
		t.Fatal("SetupLogger returned nil")
	}
}

func TestSetupLogger_ColorDisabled(t *testing.T) {
	log, err := SetupLogger(&LogConfig{
		Level:  "info",
		Format: "text",
		Color:  boolPtr(false),
	})
	if err != nil {
		t.Fatalf("SetupLogger error: %v", err)
	}
	defer log.Close()

	if log == nil {
		t.Fatal("SetupLogger returned nil")
	}
}

func TestSetupLogger_SetsDefault(t *testing.T) {
	log, err := SetupLogger(&LogConfig{Level: "warn", Format: "text"})
	if err != nil {
		t.Fatalf("SetupLogger error: %v", err)
	}
	defer log.Close()

	defaultLogger := slog.Default()
	// The default logger should be the one we just set.
	if defaultLogger.Handler() != log.Handler() {
		t.Error("SetupLogger did not set slog.Default()")
	}
}

func TestBuildLoggerOpts(t *testing.T) {
	// Base options always emitted (Level, Middleware, ConsoleFormat, ConsoleColor) = 4.
	// FilePath non-empty adds FilePath + FileFormat = +2, total 6.
	// Each non-zero rotation field adds +1 each.
	const baseCount = 4
	const fileBaseCount = baseCount + 2 // FilePath + FileFormat

	tests := []struct {
		name      string
		cfg       *LogConfig
		wantNil   bool
		wantCount int
	}{
		{
			name:    "nil config returns nil",
			cfg:     nil,
			wantNil: true,
		},
		// --- Level mapping ---
		{
			name:      "level debug",
			cfg:       &LogConfig{Level: "debug", Format: "text"},
			wantCount: baseCount,
		},
		{
			name:      "level info",
			cfg:       &LogConfig{Level: "info", Format: "text"},
			wantCount: baseCount,
		},
		{
			name:      "level warn",
			cfg:       &LogConfig{Level: "warn", Format: "text"},
			wantCount: baseCount,
		},
		{
			name:      "level error",
			cfg:       &LogConfig{Level: "error", Format: "text"},
			wantCount: baseCount,
		},
		{
			name:      "level unknown defaults to info",
			cfg:       &LogConfig{Level: "unknown", Format: "text"},
			wantCount: baseCount,
		},
		// --- Format ---
		{
			name:      "format text",
			cfg:       &LogConfig{Level: "info", Format: "text"},
			wantCount: baseCount,
		},
		{
			name:      "format json",
			cfg:       &LogConfig{Level: "info", Format: "json"},
			wantCount: baseCount,
		},
		{
			name:      "format unknown falls back to custom",
			cfg:       &LogConfig{Level: "info", Format: "whatever"},
			wantCount: baseCount,
		},
		// --- Color ---
		{
			name:      "color nil defaults to true",
			cfg:       &LogConfig{Level: "info", Format: "text"},
			wantCount: baseCount,
		},
		{
			name:      "color explicitly false",
			cfg:       &LogConfig{Level: "info", Format: "text", Color: boolPtr(false)},
			wantCount: baseCount,
		},
		{
			name:      "color explicitly true",
			cfg:       &LogConfig{Level: "info", Format: "text", Color: boolPtr(true)},
			wantCount: baseCount,
		},
		// --- FilePath empty vs non-empty ---
		{
			name:      "filepath empty no file options",
			cfg:       &LogConfig{Level: "info", Format: "text", FilePath: ""},
			wantCount: baseCount,
		},
		{
			name:      "filepath set adds file options",
			cfg:       &LogConfig{Level: "info", Format: "json", FilePath: "/tmp/test.log"},
			wantCount: fileBaseCount,
		},
		// --- File rotation fields ---
		{
			name: "file with MaxSizeMB only",
			cfg: &LogConfig{
				Level: "info", Format: "text", FilePath: "/tmp/test.log",
				MaxSizeMB: 10,
			},
			wantCount: fileBaseCount + 1,
		},
		{
			name: "file with RetentionDays only",
			cfg: &LogConfig{
				Level: "info", Format: "text", FilePath: "/tmp/test.log",
				RetentionDays: 7,
			},
			wantCount: fileBaseCount + 1,
		},
		{
			name: "file with MaxBackups only",
			cfg: &LogConfig{
				Level: "info", Format: "text", FilePath: "/tmp/test.log",
				MaxBackups: 3,
			},
			wantCount: fileBaseCount + 1,
		},
		{
			name: "file with CompressRotated true",
			cfg: &LogConfig{
				Level: "info", Format: "text", FilePath: "/tmp/test.log",
				CompressRotated: boolPtr(true),
			},
			wantCount: fileBaseCount + 1,
		},
		{
			name: "file with CompressRotated false",
			cfg: &LogConfig{
				Level: "info", Format: "text", FilePath: "/tmp/test.log",
				CompressRotated: boolPtr(false),
			},
			wantCount: fileBaseCount + 1,
		},
		{
			name: "file with all rotation fields",
			cfg: &LogConfig{
				Level: "info", Format: "json", FilePath: "/tmp/test.log",
				MaxSizeMB: 50, RetentionDays: 30, MaxBackups: 5,
				CompressRotated: boolPtr(true),
			},
			wantCount: fileBaseCount + 4,
		},
		{
			name: "file with zero rotation fields adds none",
			cfg: &LogConfig{
				Level: "info", Format: "text", FilePath: "/tmp/test.log",
				MaxSizeMB: 0, RetentionDays: 0, MaxBackups: 0,
			},
			wantCount: fileBaseCount,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := BuildLoggerOpts(tt.cfg)

			if tt.wantNil {
				if opts != nil {
					t.Fatalf("expected nil, got %d options", len(opts))
				}
				return
			}

			if opts == nil {
				t.Fatal("expected non-nil options slice")
			}
			if len(opts) != tt.wantCount {
				t.Errorf("option count = %d, want %d", len(opts), tt.wantCount)
			}
		})
	}
}

func TestBuildLoggerOpts_ProducesValidLogger(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "build_opts.log")

	tests := []struct {
		name string
		cfg  *LogConfig
	}{
		{
			name: "console only text",
			cfg:  &LogConfig{Level: "debug", Format: "text"},
		},
		{
			name: "console only json",
			cfg:  &LogConfig{Level: "warn", Format: "json"},
		},
		{
			name: "console and file with rotation",
			cfg: &LogConfig{
				Level: "info", Format: "json", FilePath: filePath,
				MaxSizeMB: 10, RetentionDays: 7, MaxBackups: 3,
				CompressRotated: boolPtr(true),
			},
		},
		{
			name: "color disabled",
			cfg:  &LogConfig{Level: "info", Format: "text", Color: boolPtr(false)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := BuildLoggerOpts(tt.cfg)
			log, err := logger.New(opts...)
			if err != nil {
				t.Fatalf("logger.New failed: %v", err)
			}
			defer log.Close()

			if log == nil {
				t.Fatal("logger.New returned nil")
			}
		})
	}
}

func TestBuildLoggerOpts_LevelBehavior(t *testing.T) {
	tests := []struct {
		name      string
		level     string
		wantLevel slog.Level
	}{
		{"debug", "debug", slog.LevelDebug},
		{"info", "info", slog.LevelInfo},
		{"warn", "warn", slog.LevelWarn},
		{"error", "error", slog.LevelError},
		{"uppercase WARN", "WARN", slog.LevelWarn},
		{"empty defaults to info", "", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := BuildLoggerOpts(&LogConfig{Level: tt.level, Format: "text"})
			log, err := logger.New(opts...)
			if err != nil {
				t.Fatalf("logger.New failed: %v", err)
			}
			defer log.Close()

			if !log.Enabled(context.TODO(), tt.wantLevel) {
				t.Errorf("expected level %v to be enabled", tt.wantLevel)
			}
			if tt.wantLevel > slog.LevelDebug {
				below := tt.wantLevel - 1
				if log.Enabled(context.TODO(), below) {
					t.Errorf("expected level %v to be disabled", below)
				}
			}
		})
	}
}
