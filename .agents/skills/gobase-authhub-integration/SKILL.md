---
name: gobase-authhub-integration
description: Guide for integrating authhub (third-party OAuth login) into downstream GoBase applications, covering recommended structure, config, route registration, callback handling, user linking, provider examples, and responsibility boundaries with GoBase core auth.
---

# GoBase — AuthHub External Integration Guide

> **Library:** `github.com/simp-lee/authhub`
> **Scope:** Downstream application code only — GoBase framework is not modified.
> **Load also:** `gobase-architecture`, `gobase-auth-extension`

---

## 1. Overview

AuthHub provides third-party OAuth login (GitHub, Google, etc.) as a standalone library.
It is **not** a built-in GoBase capability — integration happens entirely in the downstream application that imports GoBase.

This skill guides AI agents through adding authhub support to a GoBase-based application while maintaining clean separation from GoBase's core auth system (JWT + bcrypt password login).

---

## 2. Non-Goals

| Non-Goal | Rationale |
|----------|-----------|
| Modify GoBase core (`github.com/simp-lee/gobase`) | OAuth is application-level, not framework-level |
| Add OAuth dependencies to GoBase's `go.mod` | Keeps the starter kit dependency tree minimal |
| Replace GoBase's JWT auth system | authhub **complements** password login; it does not supersede it |
| Provide a universal OAuth abstraction | authhub handles provider specifics; the downstream app only wires it |
| Implement account merging or multi-provider linking UI | These are product-level decisions left to the downstream application |

---

## 3. Responsibility Boundaries

```
┌─────────────────────────────────────────────────┐
│  Downstream Application                         │
│                                                 │
│  ┌──────────────┐    ┌───────────────────────┐  │
│  │ authhub      │    │ GoBase auth module    │  │
│  │ module       │    │ (JWT, login, register)│  │
│  │              │    │                       │  │
│  │ OAuth flow   │───▶│ GenerateToken()       │  │
│  │ Provider cfg │    │ User lookup/create    │  │
│  │ Callback     │    │                       │  │
│  └──────────────┘    └───────────────────────┘  │
│         │                      │                │
│         ▼                      ▼                │
│  ┌──────────────────────────────────────────┐   │
│  │ domain.User + OAuth identity storage     │   │
│  └──────────────────────────────────────────┘   │
└─────────────────────────────────────────────────┘
```

- **GoBase auth module** owns password authentication, JWT issuance/validation, token revocation, and user registration.
- **authhub module** owns OAuth provider configuration, redirect/callback flow, and mapping OAuth identities to local users.
- After a successful OAuth callback, authhub calls GoBase's JWT service to issue a token — the same token format used for password login.

---

## 4. Recommended Project Structure

Add authhub as a new module under `internal/module/` in the downstream application:

```
internal/module/authhub/
├── dto.go              # OAuth-related request/response structs
├── handler.go          # HTTP handlers: redirect + callback endpoints
├── handler_test.go
├── module.go           # Implements app.Module, registers routes
├── module_test.go
├── provider.go         # Provider configuration and authhub client setup
├── provider_test.go
├── repository.go       # OAuth identity persistence (user_oauth_identities table)
├── repository_test.go
├── service.go          # OAuth flow logic, user find-or-create
└── service_test.go
```

Additionally, extend the domain layer:

```
internal/domain/
├── user.go             # Existing — no changes required to GoBase's User struct
└── oauth_identity.go   # New — OAuthIdentity entity + OAuthIdentityRepository interface
```

---

## 5. Domain Model Extension

### 5.1 OAuth Identity Entity

Create `internal/domain/oauth_identity.go` in the downstream app:

```go
package domain

import "context"

// OAuthIdentity links an external OAuth provider account to a local user.
type OAuthIdentity struct {
    BaseModel
    UserID       uint   `gorm:"not null;index:idx_oauth_user" json:"user_id"`
    Provider     string `gorm:"size:50;not null;uniqueIndex:idx_oauth_provider_subject" json:"provider"`
    Subject      string `gorm:"size:255;not null;uniqueIndex:idx_oauth_provider_subject" json:"subject"`
    Email        string `gorm:"size:255" json:"email,omitempty"`
    DisplayName  string `gorm:"size:200" json:"display_name,omitempty"`
}

// OAuthIdentityRepository defines data access for OAuth identities.
type OAuthIdentityRepository interface {
    Create(ctx context.Context, identity *OAuthIdentity) error
    GetByProviderAndSubject(ctx context.Context, provider, subject string) (*OAuthIdentity, error)
    ListByUserID(ctx context.Context, userID uint) ([]OAuthIdentity, error)
    Delete(ctx context.Context, id uint) error
}
```

**Key rules:**
- `Provider` + `Subject` form a unique composite index — one external account maps to exactly one local user.
- `UserID` references `domain.User.ID` — the existing GoBase user table.
- Do **not** modify GoBase's `User` struct; the link lives in the separate `oauth_identities` table.

---

## 6. Configuration

Add OAuth provider settings to the downstream app's `config.yaml`:

```yaml
oauth:
  enabled: false
  base_url: "http://localhost:8080"    # used to build callback URLs
  providers:
    github:
      client_id: ""
      client_secret: ""
      scopes: ["user:email"]
    google:
      client_id: ""
      client_secret: ""
      scopes: ["openid", "email", "profile"]
```

Extend the downstream app's config struct:

```go
type OAuthProviderConfig struct {
    ClientID     string   `koanf:"client_id" validate:"required"`
    ClientSecret string   `koanf:"client_secret" validate:"required"`
    Scopes       []string `koanf:"scopes"`
}

type OAuthConfig struct {
    Enabled   bool                           `koanf:"enabled"`
    BaseURL   string                         `koanf:"base_url" validate:"required_if=Enabled true,omitempty,url"`
    Providers map[string]OAuthProviderConfig  `koanf:"providers"`
}
```

> **Secret management:** `client_secret` values should be injected via environment variables using GoBase's existing `APP__` prefix overlay (e.g. `APP__OAUTH__PROVIDERS__GITHUB__CLIENT_SECRET`) or a secrets manager. Never commit real secrets to `config.yaml`.

---

## 7. Route Registration

### 7.1 Module Implementation

```go
package authhub

import "github.com/gin-gonic/gin"

type Module struct {
    handler *Handler
}

func NewModule(h *Handler) *Module {
    if h == nil {
        panic("authhub.NewModule: handler must not be nil")
    }
    return &Module{handler: h}
}

func (m *Module) RegisterRoutes(api *gin.RouterGroup, pages *gin.RouterGroup) {
    oauth := api.Group("/oauth")
    oauth.GET("/:provider", m.handler.RedirectToProvider)
    oauth.GET("/:provider/callback", m.handler.HandleCallback)
}
```

### 7.2 Registering in app.New()

In the downstream app's wiring code, conditionally add the authhub module:

```go
if cfg.OAuth.Enabled {
    oauthRepo := authhub.NewRepository(db)
    oauthSvc  := authhub.NewService(oauthRepo, userRepo, jwtSvc, cfg.OAuth)
    modules   = append(modules, authhub.NewModule(authhub.NewHandler(oauthSvc)))
}
```

### 7.3 Public Path Exclusion

OAuth endpoints must be accessible without an existing JWT. Add them to the Auth chain's exclusion list:

```go
chain.When(
    ginx.And(
        ginx.PathHasPrefix("/api"),
        ginx.Not(ginx.Or(
            ginx.PathIs("/api/v1/auth/login", "/api/v1/auth/register"),
            ginx.PathHasPrefix("/api/v1/oauth"),   // OAuth redirect + callback
        )),
    ),
    ginx.Auth(jwtService),
)
```

---

## 8. Integration Flow

### 8.1 Redirect (GET `/api/v1/oauth/:provider`)

1. Read `provider` param, look up the provider config.
2. Generate a cryptographically random `state` string; store it in a short-lived, `HttpOnly`, `SameSite=Lax` cookie.
3. Build the OAuth authorization URL via authhub and redirect the user.

### 8.2 Callback (GET `/api/v1/oauth/:provider/callback`)

1. Verify `state` param matches the cookie value; reject on mismatch (CSRF protection).
2. Exchange the `code` for an access token via authhub.
3. Fetch the user's profile from the provider via authhub.
4. **Find-or-create** the local user:
   - Look up `OAuthIdentity` by `(provider, subject)`.
   - If found → load the associated `domain.User`.
   - If not found → create a new `domain.User` (use provider email/name as defaults), then create the `OAuthIdentity` link.
5. Issue a JWT via `jwtSvc.GenerateToken(userID, roles, expiry)` — the same mechanism used by password login.
6. Set the access token cookie (same cookie name as password login: `access_token`) and redirect to the application home page.

### 8.3 Service Layer Sketch

```go
type oauthService struct {
    identityRepo domain.OAuthIdentityRepository
    userRepo     domain.UserRepository
    jwtSvc       jwt.Service
    cfg          OAuthConfig
}

func (s *oauthService) HandleCallback(ctx context.Context, provider, code string) (*auth.TokenResponse, error) {
    // 1. Exchange code for token + profile via authhub
    profile, err := s.exchangeAndFetchProfile(ctx, provider, code)
    if err != nil {
        return nil, domain.NewAppError(domain.CodeUnauthorized, "oauth exchange failed", err)
    }

    // 2. Find or create identity + user (inside a transaction)
    user, err := s.findOrCreateUser(ctx, provider, profile)
    if err != nil {
        return nil, err
    }

    // 3. Issue JWT — same as password login path
    token, err := s.jwtSvc.GenerateToken(
        strconv.FormatUint(uint64(user.ID), 10),
        []string{user.Role},
        s.tokenExpiry,
    )
    if err != nil {
        return nil, domain.NewAppError(domain.CodeInternal, "token generation failed", err)
    }

    parsedToken, parseErr := s.jwtSvc.ParseToken(token)
    if parseErr != nil {
        return nil, domain.NewAppError(domain.CodeInternal, "failed to parse generated token", parseErr)
    }

    return &auth.TokenResponse{
        Token:     token,
        ExpiresAt: parsedToken.ExpiresAt.Unix(),
    }, nil
}
```

---

## 9. Interaction with GoBase Auth

| Aspect | GoBase Auth (built-in) | authhub Module (downstream) |
|--------|------------------------|-----------------------------|
| User creation | `auth.Register()` — email + password | `findOrCreateUser()` — from provider profile |
| Password | Required, bcrypt-hashed | Not set; user has no password initially |
| JWT issuance | `jwtSvc.GenerateToken()` | Same `jwtSvc.GenerateToken()` |
| JWT validation | `ginx.Auth(jwtSvc)` middleware | Same — tokens are indistinguishable |
| Token revocation | `auth.Logout()` / `jwtSvc.RevokeToken()` | Same — works for all tokens |
| User model | `domain.User` | Same `domain.User` — no changes |
| Cookie name | `access_token` | Same `access_token` |

**Key principle:** After OAuth callback, the user session is identical to a password-login session. No middleware or handler downstream needs to know whether the user authenticated via password or OAuth.

### 9.1 Password-less Users

Users created via OAuth do not have a `PasswordHash`. If the application also supports password login, decide the downstream policy:

- **Option A:** Allow users to set a password later via a "Set Password" page (common).
- **Option B:** Silently reject password login for users without a PasswordHash — return the same `domain.ErrUnauthorized` (no information leak).

GoBase's existing login service already handles this correctly: `bcrypt.CompareHashAndPassword` with an empty hash will fail, and the dummy-hash timing protection still applies.

---

## 10. Provider Examples (Conceptual)

### 10.1 GitHub

```yaml
oauth:
  providers:
    github:
      client_id: "your-github-client-id"
      client_secret: "your-github-client-secret"
      scopes: ["user:email"]
```

- OAuth authorize URL: `https://github.com/login/oauth/authorize`
- Token exchange URL: `https://github.com/login/oauth/access_token`
- User info: `https://api.github.com/user`
- `subject`: GitHub numeric user ID (stable, never changes)

### 10.2 Google

```yaml
oauth:
  providers:
    google:
      client_id: "your-google-client-id"
      client_secret: "your-google-client-secret"
      scopes: ["openid", "email", "profile"]
```

- Uses OpenID Connect discovery at `https://accounts.google.com/.well-known/openid-configuration`
- `subject`: The `sub` claim from the ID token (stable Google account identifier)

### 10.3 Adding a New Provider

1. Add the provider entry to `config.yaml` under `oauth.providers`.
2. Register the provider in `provider.go` — configure authhub with the provider's endpoints and scopes.
3. No handler/route changes required — the `:provider` param routes to the correct config dynamically.

---

## 11. Testing Strategy

### 11.1 Unit Tests

- **Service:** Mock `OAuthIdentityRepository`, `UserRepository`, `jwt.Service`, and the authhub HTTP exchange. Test find-or-create logic for both "new user" and "returning user" paths.
- **Handler:** Mock the Service interface. Verify redirect URL generation, state cookie setting/checking, and callback error handling.
- **Repository:** Use GORM + SQLite in-memory (same as GoBase's existing repo tests) to verify CRUD and unique constraint behavior.

### 11.2 Important Test Cases

| Case | Expected Behavior |
|------|-------------------|
| New OAuth user, no existing local account | Create `User` + `OAuthIdentity`, return valid JWT |
| Returning OAuth user | Lookup by `(provider, subject)`, return valid JWT |
| OAuth user with disabled/pending status | Reject login, return appropriate error |
| Invalid/expired `code` param | Return 401, do not create any records |
| CSRF `state` mismatch | Return 403, reject callback |
| Provider not configured | Return 400 with clear error message |

---

## 12. Migration

The downstream application needs one new table. Add a GORM `AutoMigrate` call:

```go
if cfg.Server.Mode == "debug" {
    if err := db.AutoMigrate(&domain.OAuthIdentity{}); err != nil {
        return nil, fmt.Errorf("auto migrate: %w", err)
    }
}
```

This creates the `oauth_identities` table with the composite unique index on `(provider, subject)`.

---

## 13. Common Pitfalls

| Pitfall | Mitigation |
|---------|------------|
| Storing OAuth access/refresh tokens in the database | Not needed — authhub tokens are ephemeral, used only during the callback. GoBase issues its own JWT for session management. |
| Using provider email as a unique user key | Emails can change. Use `(provider, subject)` as the stable identifier. |
| Forgetting to exclude OAuth routes from JWT auth | The callback URL must be public; add to `ginx.PathIs(...)` exclusion list. |
| Modifying GoBase's `User` struct to add OAuth fields | Keep OAuth data in the separate `OAuthIdentity` table. GoBase's `User` stays unchanged. |
| Hardcoding callback URLs | Derive from `cfg.OAuth.BaseURL` + route pattern. Different environments (dev/staging/prod) use different base URLs. |
