package config

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
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
