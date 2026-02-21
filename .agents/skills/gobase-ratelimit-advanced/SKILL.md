---
name: gobase-ratelimit-advanced
description: Advanced rate limiting extension guide for GoBase using ginx, covering time-window and combined limiting patterns, key strategies, dynamic limits, smoothing, headers, and 429 response format.
---

# GoBase — Advanced Rate Limiting Extension Guide

> **Library:** `github.com/simp-lee/ginx`
> **Used in:** `internal/app/app.go` → `app.New()`
> **Load also:** `gobase-ginx-patterns`
> **Current state:** GoBase uses basic `ginx.RateLimit(rps, burst)` on `/api` routes.

---

## 1. Time-Window Rate Limiting

ginx provides time-window rate limiters alongside the default token-bucket (`RateLimit`):

| Function | Window | Use Case |
|----------|--------|----------|
| `ginx.RateLimit(rps, burst)` | Per-second (token bucket) | Burst protection, smoothing spikes |
| `ginx.RateLimitPerMinute(limit)` | 1 minute | Short-term quota per user/IP |
| `ginx.RateLimitPerHour(limit)` | 1 hour | Medium-term quota management |
| `ginx.RateLimitPerDay(limit)` | 24 hours | Daily usage caps |

### Basic Usage

```go
// Allow max 600 requests per minute on API routes
chain.When(ginx.PathHasPrefix("/api"),
    ginx.RateLimitPerMinute(600),
)

// Allow max 10000 requests per hour
chain.When(ginx.PathHasPrefix("/api"),
    ginx.RateLimitPerHour(10000),
)

// Allow max 100000 requests per day
chain.When(ginx.PathHasPrefix("/api"),
    ginx.RateLimitPerDay(100000),
)
```

---

## 2. Combined Limiting (Recommended Pattern)

Use **RPS** (token bucket) for burst protection and **time-window** for quota management together. Each limiter operates independently — a request must pass **all** active limiters.

### Pattern: RPS + Per-Minute Quota

```go
// Burst protection: max 100 RPS with burst allowance of 200
chain.When(ginx.PathHasPrefix("/api"),
    ginx.RateLimit(100, 200),
)

// Quota management: max 600 requests per minute
chain.When(ginx.PathHasPrefix("/api"),
    ginx.RateLimitPerMinute(600),
)
```

**Why combine?**
- `RateLimit(100, 200)` prevents sudden traffic spikes from overwhelming the server.
- `RateLimitPerMinute(600)` ensures a client cannot exhaust its quota by bursting at 200 RPS for a few seconds and then staying idle.

### Pattern: Tiered Limiting

```go
// Layer 1: burst protection
chain.When(ginx.PathHasPrefix("/api"),
    ginx.RateLimit(50, 100),
)

// Layer 2: per-minute cap
chain.When(ginx.PathHasPrefix("/api"),
    ginx.RateLimitPerMinute(500),
)

// Layer 3: daily cap
chain.When(ginx.PathHasPrefix("/api"),
    ginx.RateLimitPerDay(50000),
)
```

### Pattern: Different Limits for Different Endpoints

```go
// Strict limiting for write operations
chain.When(
    ginx.And(
        ginx.PathHasPrefix("/api"),
        ginx.Not(ginx.MethodIs("GET")),
    ),
    ginx.RateLimit(10, 20),
)

// Relaxed limiting for read operations
chain.When(
    ginx.And(
        ginx.PathHasPrefix("/api"),
        ginx.MethodIs("GET"),
    ),
    ginx.RateLimit(100, 200),
)
```

---

## 3. Key Strategies

By default, rate limiting is keyed by client IP. ginx supports alternative key strategies via options.

| Option | Key Source | Use Case |
|--------|-----------|----------|
| `WithIP()` | Client IP address (default) | Unauthenticated APIs, public endpoints |
| `WithUser()` | Authenticated user identity | Per-user quotas after login |
| `WithPath()` | Request URL path | Per-endpoint limiting |
| `WithKeyFunc(fn)` | Custom function | API keys, tenant IDs, composite keys |

### Examples

```go
// Per-IP (default — explicit form)
ginx.RateLimit(100, 200, ginx.WithIP())

// Per-authenticated-user
ginx.RateLimitPerMinute(60, ginx.WithUser())

// Per-URL-path
ginx.RateLimitPerHour(1000, ginx.WithPath())

// Custom key function (e.g., by API key header)
ginx.RateLimit(50, 100, ginx.WithKeyFunc(func(c *gin.Context) string {
    return c.GetHeader("X-API-Key")
}))

// Composite key: user + path
ginx.RateLimitPerMinute(30, ginx.WithKeyFunc(func(c *gin.Context) string {
    userID, _ := ginx.GetUserID(c)
    return userID + ":" + c.Request.URL.Path
}))
```

### Key Strategy + Chain Condition Pattern

```go
// Public endpoints: limit by IP
chain.When(
    ginx.And(
        ginx.PathHasPrefix("/api/v1/public"),
        ginx.Not(ginx.MethodIs("GET")),
    ),
    ginx.RateLimit(10, 20, ginx.WithIP()),
)

// Authenticated endpoints: limit by user
chain.When(ginx.PathHasPrefix("/api/v1/users"),
    ginx.RateLimitPerMinute(60, ginx.WithUser()),
)
```

---

## 4. Dynamic Limits

When rate limits need to vary per request (e.g., different tiers for free vs. premium users), use dynamic limit functions.

### Token-Bucket Dynamic Limits

```go
ginx.RateLimit(0, 0, ginx.WithDynamicLimits(func(c *gin.Context) (rps, burst int) {
    role, _ := ginx.GetUserRole(c)
    switch role {
    case "premium":
        return 200, 400
    case "basic":
        return 50, 100
    default:
        return 10, 20
    }
}))
```

> **Note:** When `WithDynamicLimits` is used, the `rps` and `burst` arguments to `RateLimit()` serve as fallback defaults. Pass `0, 0` if every request will be covered by the dynamic function.

### Time-Window Dynamic Limits

```go
ginx.RateLimitPerMinute(0, ginx.WithDynamicWindowLimits(func(c *gin.Context) int {
    role, _ := ginx.GetUserRole(c)
    switch role {
    case "premium":
        return 1000  // 1000 req/min
    case "basic":
        return 200   // 200 req/min
    default:
        return 60    // 60 req/min
    }
}))
```

### Integration in app.New()

```go
if cfg.Server.RateLimit.Enabled {
    rps := effectiveRateLimitRPS(cfg.Server.RateLimit.RPS)

    // Basic burst protection with static config
    chain.When(ginx.PathHasPrefix("/api"),
        ginx.RateLimit(rps, cfg.Server.RateLimit.Burst),
    )

    // Dynamic per-user quota (if user service is available)
    chain.When(ginx.PathHasPrefix("/api"),
        ginx.RateLimitPerMinute(0, ginx.WithDynamicWindowLimits(func(c *gin.Context) int {
            // Look up user tier from context or service
            role, _ := ginx.GetUserRole(c)
            return rateLimitForRole(role)
        })),
    )
}
```

---

## 5. Traffic Smoothing (WithWait)

By default, rate-limited requests receive an immediate `429 Too Many Requests` response. `WithWait` queues the request and waits for a token to become available, up to the specified timeout.

```go
// Wait up to 2 seconds before rejecting
ginx.RateLimit(50, 100, ginx.WithWait(2*time.Second))

// Wait up to 5 seconds for time-window limiter
ginx.RateLimitPerMinute(300, ginx.WithWait(5*time.Second))
```

### When to Use

| Scenario | Recommendation |
|----------|----------------|
| API endpoints | `WithWait` with short timeout (1–3s) — smooths brief bursts |
| Real-time / WebSocket | No `WithWait` — reject fast, let clients retry |
| Background / batch APIs | `WithWait` with longer timeout (5–10s) — tolerate queuing |
| Public-facing pages | No `WithWait` — immediate feedback preferred |

### Behavior

- If a token becomes available within the timeout → request proceeds normally.
- If the timeout expires → `429 Too Many Requests` is returned.
- The client experiences added latency (up to `timeout`) instead of an immediate rejection.

---

## 6. HTTP Response Headers

ginx rate limiters automatically set standard HTTP headers on every response:

| Header | Description | Example |
|--------|-------------|---------|
| `X-RateLimit-Limit` | Maximum requests allowed in the window | `100` |
| `X-RateLimit-Remaining` | Requests remaining in the current window | `42` |
| `X-RateLimit-Reset` | Unix timestamp when the window resets | `1740000000` |
| `Retry-After` | Seconds to wait before retrying (only on `429`) | `30` |

### Header Behavior by Limiter Type

- **Token-bucket** (`RateLimit`): `X-RateLimit-Limit` reflects the burst size; `X-RateLimit-Reset` indicates when the next token will be available.
- **Time-window** (`RateLimitPerMinute`, etc.): `X-RateLimit-Limit` reflects the window limit; `X-RateLimit-Reset` is the end of the current window.

### Client-Side Usage

Clients should inspect `Retry-After` on `429` responses to determine when to retry:

```javascript
if (response.status === 429) {
    const retryAfter = response.headers.get('Retry-After');
    await sleep(retryAfter * 1000);
    // retry request
}
```

---

## 7. 429 Response Format

### Default Response

When no custom response is configured, ginx returns:

```json
{"code": 429, "message": "rate limit exceeded"}
```

With HTTP status `429 Too Many Requests`.

### Custom Response

Use `WithRateLimitResponse` to provide a custom static response body:

```go
ginx.RateLimit(100, 200,
    ginx.WithRateLimitResponse(pkg.Response{
        Code:    429,
        Message: "rate limit exceeded",
    }),
)

ginx.RateLimitPerMinute(600,
    ginx.WithRateLimitResponse(pkg.Response{
        Code:    429,
        Message: "per-minute quota exceeded",
    }),
)
```

### Response Mechanism

- ginx calls `json.Marshal(response)` on the provided value.
- ginx writes the bytes directly to `http.ResponseWriter` with status `429`.
- `gin.Context` is **not** used — `c.JSON()` / `c.HTML()` are bypassed.
- The response is **always JSON**, regardless of the `Accept` header.
- Each limiter (RPS, per-minute, per-hour, per-day) can have its own custom response.

> **See also:** `gobase-ginx-patterns` § 5 for the full explanation of `WithRateLimitResponse` semantics.

---

## 8. GoBase Integration Checklist

When extending GoBase with advanced rate limiting:

### Config Changes (`internal/config/config.go`)

If adding time-window or dynamic limits, extend `RateLimitConfig`:

```go
type RateLimitConfig struct {
    Enabled    bool    `koanf:"enabled"`
    RPS        float64 `koanf:"rps"`
    Burst      int     `koanf:"burst"`
    PerMinute  int     `koanf:"per_minute"`   // optional: 0 = disabled
    PerHour    int     `koanf:"per_hour"`     // optional: 0 = disabled
    PerDay     int     `koanf:"per_day"`      // optional: 0 = disabled
}
```

### Config YAML (`configs/config.yaml`)

```yaml
server:
  rate_limit:
    enabled: true
    rps: 100
    burst: 200
    per_minute: 600    # 0 or omit to disable
    per_hour: 10000    # 0 or omit to disable
    per_day: 0         # 0 or omit to disable
```

### Chain Setup (`internal/app/app.go`)

```go
if cfg.Server.RateLimit.Enabled {
    rps := effectiveRateLimitRPS(cfg.Server.RateLimit.RPS)
    chain.When(ginx.PathHasPrefix("/api"),
        ginx.RateLimit(rps, cfg.Server.RateLimit.Burst))

    if cfg.Server.RateLimit.PerMinute > 0 {
        chain.When(ginx.PathHasPrefix("/api"),
            ginx.RateLimitPerMinute(cfg.Server.RateLimit.PerMinute))
    }
    if cfg.Server.RateLimit.PerHour > 0 {
        chain.When(ginx.PathHasPrefix("/api"),
            ginx.RateLimitPerHour(cfg.Server.RateLimit.PerHour))
    }
    if cfg.Server.RateLimit.PerDay > 0 {
        chain.When(ginx.PathHasPrefix("/api"),
            ginx.RateLimitPerDay(cfg.Server.RateLimit.PerDay))
    }
}
```

### Cleanup

ginx manages limiter cleanup internally via `ginx.CleanupRateLimiters()`. GoBase already calls this in `app.Shutdown()` — no additional cleanup is needed for time-window limiters.

---

## 9. Common AI Pitfalls

### Pitfall 1: Assuming combined limiters share state

Each `RateLimit`, `RateLimitPerMinute`, etc. call creates an **independent** limiter. They do not share counters. A request must pass all active limiters to proceed.

### Pitfall 2: Using WithWait with very long timeouts

```go
// Bad: 60-second wait ties up a goroutine and may hit Timeout middleware
ginx.RateLimit(10, 20, ginx.WithWait(60*time.Second))
```

`WithWait` timeout should always be shorter than the `Timeout` middleware duration. If `Timeout` is 30s, keep `WithWait` under 5–10s.

### Pitfall 3: Forgetting key strategy isolation

```go
// This limits per-IP, not per-user
ginx.RateLimitPerMinute(60) // default is WithIP()
```

If your API has authenticated users behind a shared IP (e.g., corporate NAT), use `WithUser()` instead.

### Pitfall 4: Dynamic limits with zero defaults and no coverage

```go
// If WithDynamicLimits returns (0, 0) for some path, the request is blocked
ginx.RateLimit(0, 0, ginx.WithDynamicLimits(func(c *gin.Context) (rps, burst int) {
    // forgot to handle unknown roles → returns (0, 0)
    return 0, 0
}))
```

Always include a `default` case in dynamic limit functions that returns a safe fallback.

### Pitfall 5: Applying WithRateLimitResponse once for multiple limiters

Each limiter needs its own `WithRateLimitResponse` if you want custom messages:

```go
// Each limiter has its own response
ginx.RateLimit(100, 200,
    ginx.WithRateLimitResponse(pkg.Response{Code: 429, Message: "rate limit exceeded"}),
)
ginx.RateLimitPerMinute(600,
    ginx.WithRateLimitResponse(pkg.Response{Code: 429, Message: "per-minute quota exceeded"}),
)
```

---

## Quick Reference

```
Rate Limiting Options
├── Limiter Type
│   ├── RateLimit(rps, burst)         — token-bucket (per-second)
│   ├── RateLimitPerMinute(limit)     — fixed window (1 min)
│   ├── RateLimitPerHour(limit)       — fixed window (1 hour)
│   └── RateLimitPerDay(limit)        — fixed window (24 hours)
├── Key Strategy
│   ├── WithIP()                      — per-client IP (default)
│   ├── WithUser()                    — per-authenticated user
│   ├── WithPath()                    — per-URL path
│   └── WithKeyFunc(fn)              — custom key function
├── Dynamic Limits
│   ├── WithDynamicLimits(fn)         — for token-bucket
│   └── WithDynamicWindowLimits(fn)   — for time-window
├── Traffic Smoothing
│   └── WithWait(timeout)             — queue instead of reject
└── Response
    └── WithRateLimitResponse(resp)   — custom 429 body
```
