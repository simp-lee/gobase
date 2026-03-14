package app

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	cache "github.com/simp-lee/cache"
	"github.com/simp-lee/ginx"
	"github.com/simp-lee/logger"

	"github.com/simp-lee/gobase/internal/config"
	"github.com/simp-lee/gobase/internal/pkg"
)

func effectiveRateLimitRPS(rps float64) int {
	effective := int(math.Ceil(rps))
	if effective < 1 {
		return 1
	}
	return effective
}

func resolveCORSOptions(mode string, corsCfg *config.CORSConfig) []ginx.Option[ginx.CORSConfig] {
	var opts []ginx.Option[ginx.CORSConfig]

	// Handle AllowOrigins.
	if len(corsCfg.AllowOrigins) > 0 {
		opts = append(opts, ginx.WithAllowOrigins(corsCfg.AllowOrigins...))
	} else if mode != gin.ReleaseMode {
		// In non-release mode with no configured origins, default to permissive.
		opts = append(opts, ginx.WithAllowOrigins("*"))
	}
	// In release mode with no configured origins, don't add WithAllowOrigins — ginx defaults to deny all.

	// Apply optional CORS settings from config.
	if len(corsCfg.AllowMethods) > 0 {
		opts = append(opts, ginx.WithAllowMethods(corsCfg.AllowMethods...))
	}
	if len(corsCfg.AllowHeaders) > 0 {
		opts = append(opts, ginx.WithAllowHeaders(corsCfg.AllowHeaders...))
	}
	if corsCfg.AllowCredentials {
		opts = append(opts, ginx.WithAllowCredentials(true))
	}
	if corsCfg.MaxAge != "" {
		d, err := time.ParseDuration(corsCfg.MaxAge)
		if err != nil {
			slog.Warn("ignoring invalid cors.max_age", slog.String("value", corsCfg.MaxAge), slog.Any("error", err))
		} else if d > 0 {
			opts = append(opts, ginx.WithMaxAge(d))
		}
	}

	return opts
}

// htmlRecoveryHandler is the custom panic handler for ginx.RecoveryWith.
// It renders an HTML error page for browser requests and a JSON response for API clients.
func htmlRecoveryHandler(c *gin.Context, err any) {
	renderError(c, 500, "internal server error")
}

// buildMiddlewareChain assembles the ginx middleware chain including
// recovery, request ID, logger, CORS, timeout, rate limiting, and caching.
func buildMiddlewareChain(cfg *config.Config) (*ginx.Chain, cache.CacheInterface, error) {
	// Build shared logger options for ginx middlewares.
	loggerOpts := config.BuildLoggerOpts(&cfg.Log)

	// Build CORS options from application settings.
	corsOpts := resolveCORSOptions(cfg.Server.Mode, &cfg.Server.CORS)

	// Parse timeout duration.
	timeoutDuration := 30 * time.Second
	serverTimeout := strings.TrimSpace(cfg.Server.Timeout)
	if serverTimeout != "" {
		parsed, err := time.ParseDuration(serverTimeout)
		if err != nil {
			return nil, nil, fmt.Errorf("parse server.timeout %q: %w", cfg.Server.Timeout, err)
		}
		timeoutDuration = parsed
	}

	// Build ginx middleware chain.
	chain := ginx.NewChain().
		WithErrorFormat(func(status int, message string) any {
			return pkg.Response{Code: status, Message: message}
		}).
		Use(ginx.RecoveryWith(htmlRecoveryHandler, loggerOpts...)).
		Use(ginx.RequestID(
			ginx.WithIgnoreIncoming(),
			ginx.WithContextInjector(func(ctx context.Context, requestID string) context.Context {
				return logger.WithContextAttrs(ctx, slog.String("request_id", requestID))
			}),
		)).
		Use(ginx.Logger(loggerOpts...)).
		Use(ginx.CORS(corsOpts...)).
		Use(ginx.Timeout(ginx.WithTimeout(timeoutDuration)))

	// Conditionally add rate limiting for /api routes.
	// /health lives at root level, so PathHasPrefix("/api") already excludes it.
	if cfg.Server.RateLimit.Enabled {
		rps := effectiveRateLimitRPS(cfg.Server.RateLimit.RPS)
		chain.When(
			ginx.PathHasPrefix("/api"),
			ginx.RateLimit(rps, cfg.Server.RateLimit.Burst),
		)
	}

	// Conditionally add response caching for GET /api/* requests.
	// Cache is disabled by default (controlled by server.cache config).
	// ginx.Cache auto-skips requests with Authorization/Cookie headers.
	var cacheInstance cache.CacheInterface
	if cfg.Server.Cache.Enabled {
		// already validated by config.Validate()
		ttl, _ := time.ParseDuration(cfg.Server.Cache.TTL)
		cacheInstance = cache.NewCache(cache.Options{
			DefaultExpiration: ttl,
			CleanupInterval:   ttl * 2,
			MaxSize:           cfg.Server.Cache.MaxSize,
		})
		chain.When(
			ginx.And(ginx.PathHasPrefix("/api"), ginx.MethodIs("GET")),
			ginx.Cache(cacheInstance),
		)
	}

	return chain, cacheInstance, nil
}
