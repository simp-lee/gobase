package config

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/simp-lee/logger"
)

// BuildLoggerOpts constructs a []logger.Option slice from the given LogConfig.
// It is exported so that both the main application logger (via SetupLogger) and
// middleware (e.g. ginx request logger) can share the same configuration logic.
// If cfg is nil, it returns nil.
func BuildLoggerOpts(cfg *LogConfig) []logger.Option {
	if cfg == nil {
		return nil
	}

	level := parseLevel(cfg.Level)

	// Determine output format
	var format logger.OutputFormat
	switch strings.ToLower(cfg.Format) {
	case "text":
		format = logger.FormatText
	case "json":
		format = logger.FormatJSON
	default:
		slog.Warn("unrecognized log format, falling back to custom", slog.String("format", cfg.Format))
		format = logger.FormatCustom
	}

	// Determine color setting (nil defaults to true)
	colorEnabled := true
	if cfg.Color != nil {
		colorEnabled = *cfg.Color
	}

	opts := []logger.Option{
		logger.WithLevel(level),
		logger.WithMiddleware(logger.ContextMiddleware()),
		logger.WithConsoleFormat(format),
		logger.WithConsoleColor(colorEnabled),
	}

	// If file path is configured, add file output options
	if cfg.FilePath != "" {
		opts = append(opts, logger.WithFilePath(cfg.FilePath))
		opts = append(opts, logger.WithFileFormat(format))

		if cfg.MaxSizeMB > 0 {
			opts = append(opts, logger.WithMaxSizeMB(cfg.MaxSizeMB))
		}
		if cfg.RetentionDays > 0 {
			opts = append(opts, logger.WithRetentionDays(cfg.RetentionDays))
		}
		if cfg.MaxBackups > 0 {
			opts = append(opts, logger.WithMaxBackups(cfg.MaxBackups))
		}
		if cfg.CompressRotated != nil {
			opts = append(opts, logger.WithCompressRotated(*cfg.CompressRotated))
		}
	}

	return opts
}

// SetupLogger creates a *logger.Logger based on the provided LogConfig,
// sets it as the global default via slog.SetDefault, and returns it.
// The caller is responsible for calling Close() on the returned logger.
// Invalid level values default to "info"; when called with an unchecked config,
// invalid format values fall back to "custom".
func SetupLogger(cfg *LogConfig) (*logger.Logger, error) {
	if cfg == nil {
		return nil, errors.New("log config is nil")
	}

	opts := BuildLoggerOpts(cfg)

	log, err := logger.New(opts...)
	if err != nil {
		return nil, err
	}

	log.SetDefault()
	return log, nil
}

// parseLevel converts a string level name to the corresponding slog.Level.
// Unrecognized values default to slog.LevelInfo.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
