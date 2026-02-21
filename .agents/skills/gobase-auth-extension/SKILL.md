---
name: gobase-auth-extension
description: Guide for adding Auth (JWT) and RBAC middleware to GoBase using ginx, including prerequisites, chain patterns, context helpers, RBAC middleware, error responses, and route recommendations.
---

# GoBase — Auth / RBAC Extension Guide

> **Depends on:** `ginx.Auth(jwt.Service)`, `ginx.RequirePermission(rbac.Service, resource, action)`
> **Load also:** `gobase-architecture`, `gobase-ginx-patterns`

---

## 1. Prerequisites

Before integrating Auth/RBAC, the application must have:

### 1.1 User Model Changes

Add credential fields to the `User` entity in `internal/domain/user.go`:

```go
type User struct {
    domain.BaseModel
    Name         string `gorm:"size:100;not null" json:"name"`
    Email        string `gorm:"size:255;uniqueIndex;not null" json:"email"`
    PasswordHash string `gorm:"size:255;not null" json:"-"` // never expose in JSON
}
```

**Key rules:**
- `PasswordHash` uses `json:"-"` to prevent leaking in API responses.
- Use `bcrypt` (`golang.org/x/crypto/bcrypt`) for hashing — never store plaintext passwords.
- bcrypt has a 72 byte input limit — enforce `max=72` on password validation.

### 1.2 Login Endpoint

Create a login endpoint that:
1. Accepts credentials (email + password).
2. Verifies password against `PasswordHash` using bcrypt.
3. Issues a JWT via `jwt.Service.GenerateToken(userID, roles)`.
4. Returns the token string and expiry in the response body.

### 1.3 JWT Service (not a generic interface)

ginx `Auth` middleware takes `jwt.Service` from `github.com/simp-lee/jwt` directly — **not** a generic `TokenValidator` interface:

```go
import "github.com/simp-lee/jwt"

jwtService, err := jwt.New(secretKey, // must be ≥ 32 characters
    jwt.WithMaxTokenLifetime(24 * time.Hour),
)
```

`jwt.Service` provides:
- `GenerateToken(userID string, roles []string) (*jwt.Token, error)`
- `ValidateToken(tokenString string) (*jwt.Token, error)`
- `RevokeToken(tokenID string) error`
- `RevokeAllUserTokens(userID string) error`
- `Close() error` — **must call on shutdown** (stops background cleanup goroutine)

**Revocation is in-memory only** — lost on process restart.

---

## 2. Auth Chain Mount Pattern

Mount `ginx.Auth` at the **chain level** using conditions, not per route group.

### 2.1 Protect API Routes Except Public Endpoints

```go
chain.When(
    ginx.And(
        ginx.PathHasPrefix("/api"),
        ginx.Not(ginx.PathIs(
            "/api/v1/auth/login",
            "/api/v1/auth/register",
        )),
    ),
    ginx.Auth(jwtService),
)
```

`ginx.PathIs(paths...)` matches exact URL paths. Use it to exclude public endpoints.

> **Note:** `ginx.PathIn()` does **not** exist. Use `ginx.PathIs(paths...)` for multi-path matching.

### 2.2 Where to Add in app.New()

Add Auth after the existing middleware in `internal/app/app.go`:

```go
chain := ginx.NewChain().
    Use(ginx.RecoveryWith(htmlRecoveryHandler, loggerOpts...)).
    Use(ginx.RequestID(...)).
    Use(ginx.Logger(loggerOpts...)).
    Use(ginx.CORS(corsOpts...)).
    Use(ginx.Timeout(ginx.WithTimeout(timeoutDuration)))

// Rate limiting (existing)
if cfg.Server.RateLimit.Enabled {
    chain.When(ginx.PathHasPrefix("/api"),
        ginx.RateLimit(rps, cfg.Server.RateLimit.Burst))
}

// Auth — add here, after rate limiting, before Build()
if cfg.Auth.Enabled {
    chain.When(
        ginx.And(
            ginx.PathHasPrefix("/api"),
            ginx.Not(ginx.PathIs(cfg.Auth.PublicPaths...)),
        ),
        ginx.Auth(jwtService),
    )
}

engine.Use(chain.Build())
```

### 2.3 Conditional Assembly Pattern

Auth/RBAC is opt-in via `config.yaml`:

```go
var jwtSvc jwt.Service
var rbacSvc rbac.Service

if cfg.Auth.Enabled {
    jwtSvc, err = jwt.New(cfg.Auth.JWTSecret,
        jwt.WithMaxTokenLifetime(tokenExpiry),
    )
    // ... error handling, defer Close()

    if cfg.Auth.RBAC.Enabled {
        sqlDB, _ := db.DB() // GORM → *sql.DB bridge
        rbacSvc, err = rbac.New(rbac.WithCachedStorage(sqlDB, cacheConfig))
        // ... error handling, defer Close()
    }

    authSvc := auth.NewService(jwtSvc, userRepo)
    modules = append(modules, auth.NewModule(auth.NewHandler(authSvc)))
}
```

---

## 3. Context Helpers

ginx provides context helpers to read authenticated user info from `*gin.Context`. These are automatically populated by `ginx.Auth` after successful token validation.

### 3.1 Getters (use in handlers)

| Function | Return Type | Description |
|----------|-------------|-------------|
| `ginx.GetUserID(c)` | `(string, bool)` | Authenticated user's ID |
| `ginx.GetUserRoles(c)` | `([]string, bool)` | Roles from the JWT token |
| `ginx.GetTokenID(c)` | `(string, bool)` | JWT token ID (jti) |
| `ginx.GetTokenExpiresAt(c)` | `(time.Time, bool)` | Token expiration time |
| `ginx.GetTokenIssuedAt(c)` | `(time.Time, bool)` | Token issuance time |
| `ginx.GetUserIDOrAbort(c)` | `(string, bool)` | Gets user ID; aborts 401 if missing |

> **Only these getters exist.** No `GetUsername`, `GetEmail`, `GetPermissions`, or `GetAuthClaims`.

**Usage in a handler:**

```go
func (h *UserHandler) GetProfile(c *gin.Context) {
    userID, ok := ginx.GetUserID(c)
    if !ok {
        pkg.Error(c, domain.NewAppError(domain.CodeUnauthorized, "not authenticated", nil))
        return
    }
    // userID is string — parse to uint for domain layer
    id, _ := strconv.ParseUint(userID, 10, 64)
    user, err := h.service.GetUser(c.Request.Context(), uint(id))
    if err != nil {
        pkg.Error(c, err)
        return
    }
    pkg.Success(c, user)
}
```

### 3.2 Setters (used internally by ginx.Auth)

| Function | Description |
|----------|-------------|
| `ginx.SetUserID(c, id)` | Set user ID in context |
| `ginx.SetUserRoles(c, roles)` | Set user roles |
| `ginx.SetTokenID(c, id)` | Set token ID |
| `ginx.SetTokenExpiresAt(c, t)` | Set expiration |
| `ginx.SetTokenIssuedAt(c, t)` | Set issuance time |

**Note:** Setters are called by `ginx.Auth` internally. Application handlers should use **getters only**.

---

## 4. WithAuthQueryToken Option

```go
ginx.Auth(jwtService, ginx.WithAuthQueryToken(true))
```

When enabled, ginx also looks for the token in the `token` query parameter (e.g., `?token=xxx`).

**Only use this for WebSocket scenarios** where HTTP headers cannot be set by the client (e.g., browser WebSocket API does not support custom headers).

**Default behavior** (without this option): ginx reads the token from the `Authorization: Bearer <token>` header only.

### When to Use

- **WebSocket endpoints:** The browser WebSocket API (`new WebSocket(url)`) does not allow setting custom headers. Pass the token as a query parameter instead.
- **Regular API endpoints:** Never use `WithAuthQueryToken`. Always use the `Authorization` header.

---

## 5. RBAC Middleware

ginx provides **resource/action** based permission checking via `github.com/simp-lee/rbac`.

> ⚠️ The RBAC model is **resource + action**, not simple role strings. There is no `ginx.RequireRole()`.

### 5.1 Permission Middleware

All require `rbac.Service` + resource + action:

| Middleware | Checks | Use Case |
|------------|--------|----------|
| `ginx.RequirePermission(rbacSvc, resource, action)` | Role + direct permissions | Default — most flexible |
| `ginx.RequireRolePermission(rbacSvc, resource, action)` | Role-based only | Ignore direct user permissions |
| `ginx.RequireUserPermission(rbacSvc, resource, action)` | Direct user only | Bypass role hierarchy |

**Usage on route groups:**

```go
func (m *UserModule) RegisterRoutes(api, pages *gin.RouterGroup) {
    users := api.Group("/users")
    users.GET("", m.handler.List)
    users.GET("/:id", m.handler.Get)

    // Write access requires users:write permission
    write := users.Group("")
    write.Use(ginx.RequirePermission(m.rbacSvc, "users", "write"))
    {
        write.POST("", m.handler.Create)
        write.PUT("/:id", m.handler.Update)
        write.DELETE("/:id", m.handler.Delete)
    }
}
```

### 5.2 Condition Functions (for chain-level When/Unless)

| Condition | Returns true when |
|-----------|-------------------|
| `ginx.IsAuthenticated()` | User ID exists in context |
| `ginx.HasPermission(rbacSvc, resource, action)` | Role + direct permission |
| `ginx.HasRolePermission(rbacSvc, resource, action)` | Role permission only |
| `ginx.HasUserPermission(rbacSvc, resource, action)` | Direct permission only |

**Important:** RBAC conditions require Auth middleware to have run first.

### 5.3 rbac.Service Setup

```go
import "github.com/simp-lee/rbac"

// rbac uses database/sql — bridge from GORM:
sqlDB, err := gormDB.DB()

// CachedStorage recommended (SQL + TTL cache)
rbacSvc, err := rbac.New(rbac.WithCachedStorage(sqlDB, rbac.CacheConfig{
    RoleTTL:       5 * time.Minute,
    UserRoleTTL:   5 * time.Minute,
    PermissionTTL: 5 * time.Minute,
}))
```

**Key facts:**
- rbac uses `*sql.DB`, **not** `*gorm.DB` — bridge via `gormDB.DB()`
- Auto-creates 3 tables: `rbac_roles`, `rbac_user_roles`, `rbac_user_permissions` (default prefix `rbac_`)
- Tables managed **outside GORM migration** — no `AutoMigrate` needed
- Supports wildcard permissions: `*` resource, `articles/*` hierarchical
- `Close()` must be called on shutdown

### 5.4 Permission Model

```go
// Roles: resource → []actions
// e.g., {"users": ["read", "write"], "*": ["*"]}

rbacSvc.HasPermission(userID, "users", "read")  // checks role + direct
rbacSvc.HasPermission(userID, "*", "*")          // wildcard = full access
```

---

## 6. Auth/RBAC Error Responses

ginx returns **fixed JSON responses** for authentication and authorization failures:

### 6.1 Authentication Failure (401)

Returned by `ginx.Auth`:

```json
{"error": "missing token"}
{"error": "invalid token"}
```

### 6.2 Authorization Failure

Returned by `ginx.RequirePermission` and variants:

```json
{"error": "user not authenticated"}     // 401 — no user ID in context
{"error": "permission denied"}          // 403 — RequirePermission
{"error": "insufficient role permissions"}  // 403 — RequireRolePermission
{"error": "insufficient user permissions"} // 403 — RequireUserPermission
{"error": "permission check failed"}    // 500 — rbac service error
```

### 6.3 Format Difference from pkg.Response

These use `{"error": "..."}` format, distinct from GoBase's `pkg.Response` (`{"code": N, "message": "...", "data": ...}`). Recommended fix: add `WithAuthErrorResponse(any)` option to ginx following the `WithTimeoutResponse`/`WithRateLimitResponse` pattern.

### 6.4 OnError Does Not Fire

Auth/RBAC rejections are self-contained — `OnError` is not involved.

---

## 7. Route Group Mounting Recommendation

### Principle

- **Auth** → Chain level with conditions (centralized).
- **RBAC** → Per route group (granular, co-located with routes).

### Recommended Structure

```go
// In app.New(): chain-level auth
chain.When(
    ginx.And(
        ginx.PathHasPrefix("/api"),
        ginx.Not(ginx.PathIs(cfg.Auth.PublicPaths...)),
    ),
    ginx.Auth(jwtService),
)

// In module RegisterRoutes(): per-group RBAC
func (m *UserModule) RegisterRoutes(api, pages *gin.RouterGroup) {
    users := api.Group("/users")
    users.GET("", m.handler.List)
    users.GET("/:id", m.handler.Get)

    admin := users.Group("")
    admin.Use(ginx.RequirePermission(m.rbacSvc, "users", "write"))
    {
        admin.POST("", m.handler.Create)
        admin.PUT("/:id", m.handler.Update)
        admin.DELETE("/:id", m.handler.Delete)
    }
}
```

---

## 8. Graceful Shutdown

Both `jwt.Service` and `rbac.Service` require `Close()` during shutdown:

```go
// In App.Run(), after HTTP server shutdown, before DB close:
if a.jwtService != nil {
    a.jwtService.Close()
}
if a.rbacService != nil {
    a.rbacService.Close()
}
```

---

## 9. Common AI Pitfalls

### Pitfall 1: Using non-existent `ginx.RequireRole("admin")`

**Wrong:** `admin.Use(ginx.RequireRole("admin"))`
**Right:** `admin.Use(ginx.RequirePermission(rbacSvc, "users", "write"))` — resource/action model.

### Pitfall 2: Using non-existent context helpers

**Wrong:** `ginx.GetEmail(c)`, `ginx.GetUsername(c)`, `ginx.GetRoles(c)`, `ginx.GetPermissions(c)`
**Right:** `ginx.GetUserID(c)`, `ginx.GetUserRoles(c)`, `ginx.GetTokenID(c)`, `ginx.GetTokenExpiresAt(c)`, `ginx.GetTokenIssuedAt(c)`

### Pitfall 3: Using `ginx.PathIn()` for path exclusion

**Wrong:** `ginx.Not(ginx.PathIn(paths...))` — PathIn does **not** exist.
**Right:** `ginx.Not(ginx.PathIs(paths...))` — PathIs accepts variadic paths.

### Pitfall 4: Passing `*gorm.DB` to rbac

**Wrong:** `rbac.New(rbac.WithSQLStorage(gormDB))` — type mismatch.
**Right:** `sqlDB, _ := gormDB.DB()` then `rbac.New(rbac.WithCachedStorage(sqlDB, config))`

### Pitfall 5: Forgetting Close() on shutdown

`jwt.Service` runs a background goroutine. `rbac.Service` with CachedStorage also needs cleanup. Always call `Close()` in graceful shutdown.

### Pitfall 6: Assuming TokenValidator interface exists

ginx `Auth()` takes `jwt.Service` (concrete type from `github.com/simp-lee/jwt`), **not** a generic interface.

### Pitfall 7: Expecting username/email in JWT context

ginx `Auth` only stores: UserID, UserRoles, TokenID, ExpiresAt, IssuedAt. Query the database using UserID if you need more user info.
