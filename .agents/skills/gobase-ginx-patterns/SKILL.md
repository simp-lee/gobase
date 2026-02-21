---
name: gobase-ginx-patterns
description: ginx Chain API usage patterns for GoBase AI agents, covering fluent chain building, condition combinators, middleware response formats, OnError scope, response option semantics, and common pitfalls.
---

# ginx — Chain API Usage Patterns

> **Library:** `github.com/simp-lee/ginx`
> **Used in:** `internal/app/app.go` → `app.New()`

---

## 1. Chain API Paradigm

ginx uses a fluent builder to compose middleware into a single `gin.HandlerFunc`:

```go
chain := ginx.NewChain().
    Use(middlewareA).
    Use(middlewareB).
    When(condition, middlewareC).
    Unless(condition, middlewareD).
    OnError(errorHandler)

engine.Use(chain.Build())
```

### Methods

| Method | Signature | Behavior |
|--------|-----------|----------|
| `Use` | `Use(middleware ginx.Middleware)` | Always applies the middleware to every request |
| `When` | `When(cond ginx.Condition, middleware ginx.Middleware)` | Applies middleware only when `cond` returns true |
| `Unless` | `Unless(cond ginx.Condition, middleware ginx.Middleware)` | Applies middleware only when `cond` returns false |
| `OnError` | `OnError(handler ginx.ErrorHandler)` | Fires when `c.Error()` is called by a handler or middleware |
| `Build` | `Build() gin.HandlerFunc` | Returns one composed handler for `engine.Use()` |

### Version note

This repo uses `github.com/simp-lee/ginx v0.0.0-20260219153935-5d0a4cb33929`.
In this version, `Build()` returns a single `gin.HandlerFunc` and `When/Unless` accept one `ginx.Middleware` each call.

### Real Usage (from `app.New()`)

```go
chain := ginx.NewChain().
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

// Conditional: rate-limit only /api routes.
if cfg.Server.RateLimit.Enabled {
    chain.When(ginx.PathHasPrefix("/api"),
        ginx.RateLimit(rps, cfg.Server.RateLimit.Burst))
}

engine.Use(chain.Build())
```

---

## 2. Condition Combinators

ginx provides composable predicates for `When()` / `Unless()`:

| Function | Matches |
|----------|---------|
| `ginx.PathIs(paths...)` | Exact URL path match (variadic) |
| `ginx.PathHasPrefix(prefix)` | URL path starts with prefix |
| `ginx.PathHasSuffix(suffix)` | URL path ends with suffix |
| `ginx.PathMatches(pattern)` | URL path matches regex pattern |
| `ginx.MethodIs(methods...)` | HTTP method matches (variadic) |
| `ginx.HeaderExists(name)` | Request has the specified header |
| `ginx.HeaderEquals(name, value)` | Header equals specified value |
| `ginx.ContentTypeIs(types...)` | Content-Type matches |
| `ginx.And(cond1, cond2)` | Both conditions true |
| `ginx.Or(cond1, cond2)` | Either condition true |
| `ginx.Not(cond)` | Inverts condition |

> **Note:** `ginx.PathIn()` does **not** exist. Use `ginx.PathIs(paths...)` for multi-path matching.

### Examples

```go
// Rate-limit POST/PUT/DELETE on API routes only:
chain.When(
    ginx.And(
        ginx.PathHasPrefix("/api"),
        ginx.Not(ginx.MethodIs("GET")),
    ),
    ginx.RateLimit(10, 20),
)

// Skip logger for health checks:
chain.Unless(ginx.PathIs("/health"), ginx.Logger(loggerOpts...))
```

---

## 3. Middleware Response Format Differences

Each ginx middleware handles errors/responses through its own mechanism.
They do **not** all use the same response path.

| Middleware | Response Behavior |
|------------|-------------------|
| **Recovery** | Calls the custom handler function (`htmlRecoveryHandler`). The handler inspects `Accept` header and renders HTML or JSON accordingly. |
| **Timeout** | Writes a static response configured via `WithTimeoutResponse(response any)`. ginx JSON-serializes it directly — **bypasses** `gin.Context` rendering. |
| **RateLimit** | Writes a static response configured via `WithRateLimitResponse(response any)`. ginx JSON-serializes it directly — **bypasses** `gin.Context` rendering. |
| **CORS** | No response body — sets CORS headers only. Handles preflight `OPTIONS` requests. |
| **Logger** | No response body — logs request metadata only. |
| **RequestID** | No response body — injects `X-Request-Id` header and optionally enriches context. |

---

## 4. OnError Scope

`OnError` registers an error handler that fires **only** when `c.Error()` is called by a downstream handler or middleware.

### What triggers OnError

- Application code calling `c.Error(err)` in a handler
- Middleware calling `c.Error(err)` before `c.Next()`

### What does NOT trigger OnError

- **Timeout errors** — handled by Timeout middleware's own response mechanism
- **RateLimit errors** — handled by RateLimit middleware's own response mechanism
- **Recovery (panics)** — handled by Recovery middleware's custom handler

**Key insight:** Timeout, RateLimit, and Recovery each have self-contained error responses. They never call `c.Error()`, so `OnError` is not involved.

---

## 5. WithTimeoutResponse / WithRateLimitResponse

These options accept a static `any` value that ginx JSON-serializes directly.
They are **not** routed through `gin.Context` methods like `c.JSON()`.

### Usage

```go
ginx.Timeout(
    ginx.WithTimeout(30*time.Second),
    ginx.WithTimeoutResponse(pkg.Response{
        Code:    408,
        Message: "request timeout",
    }),
)

ginx.RateLimit(10, 20,
    ginx.WithRateLimitResponse(pkg.Response{
        Code:    429,
        Message: "too many requests",
    }),
)
```

### Behavior

1. ginx calls `json.Marshal(response)` on the provided value.
2. ginx writes the bytes directly to `http.ResponseWriter` with the appropriate status code.
3. `gin.Context` is **not** used to render the response.
4. Accept-based HTML/JSON branching does **not** apply — the response is always JSON.

---

## 6. Common AI Pitfalls

### Pitfall 1: Thinking OnError catches Timeout/RateLimit errors

**Wrong:**
```go
chain.OnError(func(c *gin.Context) {
    // "This will handle timeout errors too" — NO, it won't
    pkg.Error(c, c.Errors.Last())
})
```

**Reality:** Timeout and RateLimit use their own static response mechanism. OnError only fires for explicit `c.Error()` calls from handlers.

### Pitfall 2: Using renderError inside WithTimeoutResponse

**Wrong:**
```go
ginx.Timeout(
    ginx.WithTimeout(30*time.Second),
    ginx.WithTimeoutResponse(func(c *gin.Context) {
        renderError(c) // ginx doesn't call this as a handler
    }),
)
```

**Reality:** `WithTimeoutResponse` accepts a static value, not a function. ginx serializes it with `json.Marshal` and writes raw bytes — no `gin.Context` involved.

### Pitfall 3: Confusing When vs Unless

- `When(cond, mw)` — apply middleware **when** condition is true
- `Unless(cond, mw)` — apply middleware **unless** condition is true (i.e., when false)

They are logical inverses. `When(cond, mw)` ≡ `Unless(Not(cond), mw)`.

### Pitfall 4: Forgetting Build() returns a single handler

**Wrong:**
```go
engine.Use(chain) // chain is *Chain, not a gin.HandlerFunc
```

**Right:**
```go
engine.Use(chain.Build()) // Build() -> gin.HandlerFunc
```

### Pitfall 5: Mixing application-level middleware with ginx chain

`middleware.CSRF(secret)` is an **application-level** middleware registered on route groups, not via the ginx chain. Don't add it to `ginx.NewChain()`.

```go
// Correct — CSRF on page route group only:
pages := engine.Group("/")
pages.Use(middleware.CSRF(csrfSecret))

// Wrong — CSRF in ginx chain (applies globally, breaks API routes):
chain.Use(middleware.CSRF(csrfSecret)) // DON'T
```

---

## Quick Reference: GoBase Middleware Stack

```
Request
  │
  ├─ Recovery        (ginx — global, Use)
  ├─ RequestID       (ginx — global, Use)
  ├─ Logger          (ginx — global, Use)
  ├─ CORS            (ginx — global, Use)
  ├─ Timeout         (ginx — global, Use)
  ├─ RateLimit       (ginx — conditional, When /api)
  │
  ├─ CSRF            (app middleware — page route group only)
  │
  └─ Handler
```
