---
name: gobase-architecture
description: GoBase project architecture overview for AI agents, including layers, layout, dependency rules, response standard, config, middleware chain, module interface, and DI pattern.
---

# GoBase — Project Architecture Overview

> **Module path:** `github.com/simp-lee/gobase`
> **Go version:** 1.25+
> **Key dependencies:** gin, ginx, gorm, koanf, validator/v10

---

## 1. Clean Architecture (2-Layer)

GoBase follows a simplified Clean Architecture with three layers inside each module:

```
Handler  →  Service  →  Repository
  (HTTP)     (business)    (data access)
```

- **Handler** — receives HTTP requests, parses input (bind + validate), calls Service, returns `pkg.Response`.
- **Service** — pure business logic, operates on domain types, returns domain errors (`*domain.AppError`).
- **Repository** — data access via GORM; translates DB errors to domain errors.

Interfaces for Service and Repository are defined in `internal/domain/` (e.g. `domain.UserService`, `domain.UserRepository`). Concrete implementations live in `internal/module/<name>/`.

---

## 2. Directory Layout

```
cmd/
  server/main.go          — HTTP server entry point
  seed/main.go            — seed data CLI tool
configs/
  config.yaml             — default YAML configuration
internal/
  app/                    — application core (wiring, routing, templates)
    app.go                — New() constructor: logger → DB → DI → engine → routes
    routes.go             — RegisterRoutes(): static, health, API group, page group
    module.go             — Module interface definition
    template.go           — TemplateRenderer: layout/partial/page inheritance
  config/                 — configuration structs + loaders
    config.go             — Config struct, Load(), Validate()
    database.go           — SetupDatabase() (SQLite / Postgres, connection pool)
    logger.go             — SetupLogger() (simp-lee/logger wrapper)
  domain/                 — entities, interfaces, error codes
    model.go              — BaseModel, PageRequest, PageResult[T]
    user.go               — User entity, UserRepository interface, UserService interface
    errors.go             — AppError type, error codes (CodeNotFound, etc.), HTTPStatusCode()
  middleware/             — application-level middleware (non-ginx)
    csrf.go               — HMAC-SHA256 CSRF token middleware for HTML forms
  module/
    user/                 — example CRUD module
      dto.go              — CreateUserRequest, UpdateUserRequest (binding tags)
      handler.go          — UserHandler (JSON API endpoints)
      page_handler.go     — UserPageHandler (HTML pages + htmx endpoints)
      repository.go       — userRepository (GORM implementation)
      service.go          — userService (business logic)
  pkg/                    — shared helpers (no domain knowledge)
    response.go           — Response struct, Success(), Error(), List(), BindAndValidate()
    pagination.go         — ParsePageRequest(), Paginate(), Sort(), Filter() GORM scopes
    tx.go                 — WithTx() transaction helper
    ctxlog.go             — (reserved for context-aware logging helpers)
web/
  embed.go                — //go:embed for templates + static assets
  templates/              — Go HTML templates (layouts/, partials/, errors/, page dirs)
  static/                 — CSS, JS, vendor assets
```

---

## 3. Dependency Direction Rules

```
cmd/server  →  internal/app  →  internal/config
                              →  internal/domain
                              →  internal/module/*
                              →  internal/middleware
                              →  internal/pkg

internal/module/*  →  internal/domain
                   →  internal/pkg
                   ✗  NEVER imports internal/app

internal/pkg       →  internal/domain  (for types only)
                   ✗  NEVER imports internal/app or internal/module/*

internal/domain    →  (standard library only)
                   ✗  NEVER imports any other internal package
```

**Key rule:** `internal/module/*` must NEVER import `internal/app`. All wiring flows top-down from `app`.

---

## 4. Response Standard

All JSON API responses use `pkg.Response`:

```go
type Response struct {
    Code    int    `json:"code"`    // HTTP status code
    Message string `json:"message"` // "success" or error description
    Data    any    `json:"data"`    // payload (nil on error)
}
```

Helper functions:
- `pkg.Success(c, data)` — 200 with data
- `pkg.Error(c, err)` — maps `*domain.AppError` to HTTP status; otherwise 500
- `pkg.List(c, result)` — 200 with `PageResult[T]` in Data
- `pkg.ValidationError(c, err)` — 400 with per-field errors in `ValidationErrorResponse`
- `pkg.BindAndValidate(c, &req)` — bind + validate; auto-sends 400 on failure, returns bool

Domain error code → HTTP status mapping (in `domain.HTTPStatusCode()`):
| Code | Constant | HTTP Status |
|------|----------|-------------|
| 1 | `CodeNotFound` | 404 |
| 2 | `CodeAlreadyExists` | 409 |
| 3 | `CodeValidation` | 400 |
| 4 | `CodeInternal` | 500 |

---

## 5. Config Management

**Library:** koanf v2

**Loading order** (in `config.Load()`):
1. YAML file (default: `configs/config.yaml`)
2. Environment variable overlay (prefix `APP__`, double-underscore as hierarchy separator)

**Env var convention:**
- `APP__SERVER__PORT=9090` → `server.port`
- `APP__DATABASE__POOL__MAX_IDLE_CONNS=20` → `database.pool.max_idle_conns`
- Single underscores are preserved as part of the key name; double underscores delimit hierarchy.

**Top-level config struct:**
```go
type Config struct {
    Server   ServerConfig   `koanf:"server"`
    Database DatabaseConfig `koanf:"database"`
    Log      LogConfig      `koanf:"log"`
}
```

**Validation:** `Config.Validate()` checks server mode, port range, DB driver, duration fields, rate limit, log level/format.

**Database drivers:** SQLite (dev default) and PostgreSQL (production). Configured via `database.driver`.

---

## 6. ginx Middleware Chain

Global middleware is built using the `ginx.NewChain()` fluent API in `app.New()`:

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

// Conditional middleware:
chain.When(ginx.PathHasPrefix("/api"),
    ginx.RateLimit(rps, burst))

engine.Use(chain.Build())
```

**Chain methods:**
- `Use(middleware)` — always applied
- `When(predicate, middleware)` — applied only when predicate matches
- `Unless(predicate, middleware)` — applied only when predicate is false
- `OnError(handler)` — fires when `c.Error()` is called
- `Build()` — returns a single `gin.HandlerFunc`

**ginx-provided middleware:** Recovery, RequestID, Logger, CORS, Timeout, RateLimit.

**Application-level middleware** (not ginx): `middleware.CSRF(secret)` — used on page route groups for HTML form protection. API routes are exempt.

---

## 7. Module Interface

```go
// internal/app/module.go
type Module interface {
    RegisterRoutes(api *gin.RouterGroup, pages *gin.RouterGroup)
}
```

Each module receives two route groups:
- `api` — JSON API routes (no CSRF middleware)
- `pages` — HTML page routes (with CSRF middleware)

Currently, `RegisterRoutes()` in `routes.go` performs module-loop registration via `deps.Modules` and invokes `m.RegisterRoutes(api, pages)` for each module.

**Current route structure:**
(`internal/app/routes.go` defines `api := r.Group("/api/v1")`)
```
GET    /health                — health check
GET    /                      — home page (with CSRF)

# API routes (no CSRF)
POST   /api/v1/users          — create user
GET    /api/v1/users/:id      — get user
GET    /api/v1/users          — list users
PUT    /api/v1/users/:id      — update user
DELETE /api/v1/users/:id      — delete user

# Page routes (with CSRF)
GET    /users                 — user list page
GET    /users/new             — new user form
GET    /users/:id/edit        — edit user form
POST   /users                 — create user (htmx)
PUT    /users/:id             — update user (htmx)
DELETE /users/:id             — delete user (htmx)
```

---

## 8. DI Pattern — Manual Injection in app.go

All dependency injection is done manually in `app.New()`, following the chain:

```go
// 4. Manual dependency injection: repository → service → handler.
repo := user.NewUserRepository(db)
svc  := user.NewUserService(repo)
handler     := user.NewUserHandler(svc)
pageHandler := user.NewUserPageHandler(svc)
```

**Conventions:**
- Constructors are named `New<Type>(deps...)` and return the interface type (Service, Repository) or a concrete pointer (Handler).
- Each constructor receives only its direct dependencies.
- No DI framework is used — all wiring is explicit in `app.New()`.

---

## 9. Template System

**Renderer:** `TemplateRenderer` in `internal/app/template.go`

**Directory structure under `web/templates/`:**
```
layouts/base.html      — base layout with block slots (title, content, etc.)
partials/*.html        — shared fragments (nav, pagination, toast)
errors/*.html          — error pages (400, 404, 500)
<module>/*.html        — page templates per module (e.g. user/list.html)
home.html              — standalone page
```

**Behavior:**
- **Debug mode:** templates re-parsed on every request (hot reload).
- **Release mode:** templates parsed once at startup from `embed.FS`.

**Pattern:** Page templates call `{{ template "base" . }}` and define blocks (`{{ define "title" }}`, `{{ define "content" }}`).

---

## 10. Startup Sequence (app.New)

1. Setup logger (`config.SetupLogger`)
2. Setup database + connection pool (`config.SetupDatabase`)
3. AutoMigrate in debug mode only
4. Manual DI: repo → service → handler
5. Create gin.Engine with ginx middleware chain
6. Resolve filesystem (embed.FS or os.DirFS) + setup TemplateRenderer
7. Resolve CSRF secret (validate in release mode)
8. Register all routes (`RegisterRoutes`)

Server starts via `app.Run()` which listens for HTTP and handles graceful shutdown on SIGINT/SIGTERM.

---

## Quick Reference for New Modules

To add a new module (e.g. `product`):

1. **Domain:** Define entity, repository interface, and service interface in `internal/domain/`.
2. **Module:** Create `internal/module/product/` with `repository.go`, `service.go`, `handler.go`, `page_handler.go`, `dto.go`.
3. **Wire:** In `app.New()`, instantiate repo → svc → handler → pageHandler.
4. **Routes:** Register API and page routes in `RegisterRoutes()` (or implement `Module` interface).
5. **Templates:** Add page templates under `web/templates/product/`.
6. **Migration:** Add entity to `db.AutoMigrate()` in `app.New()`.
