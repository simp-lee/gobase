---
name: gobase-new-module
description: Guide for creating new business modules in GoBase. Use when adding a new resource/domain entity with CRUD operations — covers module interface, four-layer structure, route registration, domain model, migration, and test patterns.
---

# GoBase — New Module Creation Guide

> Canonical example: `internal/module/user/` — refer to it for any detail not covered here.

## Overview

GoBase organises business logic into self-contained **modules** under `internal/module/{name}/`.
Each module follows a four-layer architecture (DTO → Handler → Service → Repository) and satisfies the `app.Module` interface so its routes are auto-registered at startup.

---

## 1. Domain Model & Interfaces

Create `internal/domain/{name}.go`:

```go
package domain

import "context"

// {Name} represents a {name} in the system.
type {Name} struct {
    BaseModel
    // Add entity-specific fields with GORM + JSON tags.
    Title string `gorm:"size:200;not null" json:"title"`
}

// {Name}Repository defines the data access interface.
type {Name}Repository interface {
    Create(ctx context.Context, entity *{Name}) error
    GetByID(ctx context.Context, id uint) (*{Name}, error)
    List(ctx context.Context, req PageRequest) (*PageResult[{Name}], error)
    Update(ctx context.Context, entity *{Name}) error
    Delete(ctx context.Context, id uint) error
}

// {Name}Service defines the business logic interface.
type {Name}Service interface {
    Create{Name}(ctx context.Context, /* params */) (*{Name}, error)
    Get{Name}(ctx context.Context, id uint) (*{Name}, error)
    List{Name}s(ctx context.Context, req PageRequest) (*PageResult[{Name}], error)
    Update{Name}(ctx context.Context, id uint, /* params */) (*{Name}, error)
    Delete{Name}(ctx context.Context, id uint) error
}
```

**Key rules:**
- Entity embeds `domain.BaseModel` (ID, CreatedAt, UpdatedAt — no soft-delete).
- Interfaces live in the `domain` package, implementations live in the module package.
- Use `PageRequest` / `PageResult[T]` from domain for pagination.

---

## 2. Module Directory Structure

```
internal/module/{name}/
├── dto.go            # Request/response structs with validation tags
├── handler.go        # REST API handler (JSON endpoints)
├── handler_test.go   # Handler unit tests with mock service
├── module.go         # Module struct + RegisterRoutes
├── module_test.go    # Route registration verification
├── page_handler.go   # (optional) htmx page handler
├── page_handler_test.go
├── repository.go     # GORM data access
├── repository_test.go
├── service.go        # Business logic
└── service_test.go
```

---

## 3. File-by-File Implementation

### 3.1 `dto.go` — Request / Response Structs

```go
package {name}

// Create{Name}Request represents the input for creating a new {name}.
type Create{Name}Request struct {
    Title string `json:"title" form:"title" binding:"required,min=2,max=200"`
}

// Update{Name}Request represents the input for updating an existing {name}.
type Update{Name}Request struct {
    Title string `json:"title" form:"title" binding:"required,min=2,max=200"`
}
```

- Include both `json` and `form` tags (supports JSON API + htmx form posts).
- Use `binding` tags from go-playground/validator (e.g. `required,min=2,email`).

### 3.2 `repository.go` — Data Access Layer

```go
package {name}

import (
    "context"
    "errors"

    "github.com/simp-lee/gobase/internal/domain"
    "github.com/simp-lee/gobase/internal/pkg"
    "gorm.io/gorm"
)

var (
    allowedSortFields   = []string{"id", "title", "created_at", "updated_at"}
    allowedFilterFields = []string{"title"}
)

type {name}Repository struct {
    db *gorm.DB
}

func New{Name}Repository(db *gorm.DB) domain.{Name}Repository {
    return &{name}Repository{db: db}
}

func (r *{name}Repository) Create(ctx context.Context, entity *domain.{Name}) error {
    if err := r.db.WithContext(ctx).Create(entity).Error; err != nil {
        return mapError(err)
    }
    return nil
}

func (r *{name}Repository) GetByID(ctx context.Context, id uint) (*domain.{Name}, error) {
    var entity domain.{Name}
    if err := r.db.WithContext(ctx).First(&entity, id).Error; err != nil {
        return nil, mapError(err)
    }
    return &entity, nil
}

func (r *{name}Repository) List(ctx context.Context, req domain.PageRequest) (*domain.PageResult[domain.{Name}], error) {
    var total int64
    base := r.db.WithContext(ctx).Model(&domain.{Name}{}).
        Scopes(pkg.Filter(req, allowedFilterFields))

    if err := base.Count(&total).Error; err != nil {
        return nil, mapError(err)
    }

    var items []domain.{Name}
    if err := base.Scopes(
        pkg.Paginate(req),
        pkg.Sort(req, allowedSortFields),
    ).Find(&items).Error; err != nil {
        return nil, mapError(err)
    }

    return pkg.NewPageResult(items, total, req), nil
}

func (r *{name}Repository) Update(ctx context.Context, entity *domain.{Name}) error {
    if err := r.db.WithContext(ctx).Save(entity).Error; err != nil {
        return mapError(err)
    }
    return nil
}

func (r *{name}Repository) Delete(ctx context.Context, id uint) error {
    result := r.db.WithContext(ctx).Delete(&domain.{Name}{}, id)
    if result.Error != nil {
        return mapError(result.Error)
    }
    if result.RowsAffected == 0 {
        return domain.ErrNotFound
    }
    return nil
}

// mapError converts GORM errors to domain errors.
func mapError(err error) error {
    if err == nil {
        return nil
    }
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return domain.ErrNotFound
    }
    if errors.Is(err, gorm.ErrDuplicatedKey) || isDuplicateKeyError(err) {
        return domain.NewAppError(domain.CodeAlreadyExists, "already exists", err)
    }
    return domain.NewAppError(domain.CodeInternal, "database error", err)
}
```

- Constructor returns `domain.{Name}Repository` (interface type).
- Use `pkg.Paginate`, `pkg.Sort`, and `pkg.Filter` scopes for list queries.
- Use `pkg.NewPageResult` to build paginated results.
- Every GORM error goes through `mapError` → domain error.

### 3.3 `service.go` — Business Logic

```go
package {name}

import (
    "context"
    "strings"

    "github.com/simp-lee/gobase/internal/domain"
)

type {name}Service struct {
    repo domain.{Name}Repository
}

func New{Name}Service(repo domain.{Name}Repository) domain.{Name}Service {
    return &{name}Service{repo: repo}
}

func (s *{name}Service) Create{Name}(ctx context.Context, title string) (*domain.{Name}, error) {
    title = strings.TrimSpace(title)
    // input validation → return domain.NewAppError(domain.CodeValidation, ...)

    entity := &domain.{Name}{Title: title}
    if err := s.repo.Create(ctx, entity); err != nil {
        return nil, err
    }
    return entity, nil
}

// Get{Name}, List{Name}s, Update{Name}, Delete{Name} follow the same pattern.
```

- Constructor returns `domain.{Name}Service` (interface type).
- Validate inputs in the service layer; return `domain.AppError` for business rule violations.

### 3.4 `handler.go` — REST API Handler

```go
package {name}

import (
    "net/http"

    "github.com/gin-gonic/gin"

    "github.com/simp-lee/gobase/internal/domain"
    "github.com/simp-lee/gobase/internal/pkg"
)

type {Name}Handler struct {
    svc domain.{Name}Service
}

func New{Name}Handler(svc domain.{Name}Service) *{Name}Handler {
    return &{Name}Handler{svc: svc}
}

// Create handles POST /api/v1/{names}
func (h *{Name}Handler) Create(c *gin.Context) {
    var req Create{Name}Request
    if !pkg.BindAndValidate(c, &req) {
        return
    }

    entity, err := h.svc.Create{Name}(c.Request.Context(), req.Title)
    if err != nil {
        pkg.Error(c, err)
        return
    }

    c.JSON(http.StatusCreated, pkg.Response{
        Code:    http.StatusCreated,
        Message: "success",
        Data:    entity,
    })
}

// Get handles GET /api/v1/{names}/:id
func (h *{Name}Handler) Get(c *gin.Context) {
    id, err := parseID(c)
    if err != nil {
        pkg.Error(c, domain.NewAppError(domain.CodeValidation, err.Error(), nil))
        return
    }

    entity, err := h.svc.Get{Name}(c.Request.Context(), id)
    if err != nil {
        pkg.Error(c, err)
        return
    }

    pkg.Success(c, entity)
}

// List handles GET /api/v1/{names}
func (h *{Name}Handler) List(c *gin.Context) {
    req := pkg.ParsePageRequest(c)

    result, err := h.svc.List{Name}s(c.Request.Context(), req)
    if err != nil {
        pkg.Error(c, err)
        return
    }

    pkg.List(c, result)
}

// Update handles PUT /api/v1/{names}/:id
func (h *{Name}Handler) Update(c *gin.Context) {
    id, err := parseID(c)
    if err != nil {
        pkg.Error(c, domain.NewAppError(domain.CodeValidation, err.Error(), nil))
        return
    }

    var req Update{Name}Request
    if !pkg.BindAndValidate(c, &req) {
        return
    }

    entity, err := h.svc.Update{Name}(c.Request.Context(), id, req.Title)
    if err != nil {
        pkg.Error(c, err)
        return
    }

    pkg.Success(c, entity)
}

// Delete handles DELETE /api/v1/{names}/:id
func (h *{Name}Handler) Delete(c *gin.Context) {
    id, err := parseID(c)
    if err != nil {
        pkg.Error(c, domain.NewAppError(domain.CodeValidation, err.Error(), nil))
        return
    }

    if err := h.svc.Delete{Name}(c.Request.Context(), id); err != nil {
        pkg.Error(c, err)
        return
    }

    pkg.Success(c, nil)
}
```

**Handler conventions:**
- Bind + validate with `pkg.BindAndValidate(c, &req)` — it sends the 400 response automatically on failure.
- Success responses: `pkg.Success(c, data)` (200) or manual `c.JSON(http.StatusCreated, pkg.Response{...})` (201).
- Error responses: `pkg.Error(c, err)` — maps `domain.AppError` codes to HTTP status codes.
- List responses: `pkg.List(c, result)`.
- `parseID(c)` is a helper that extracts and validates the `:id` param — copy/adapt from the user module.

### 3.5 `page_handler.go` — htmx Page Handler (Optional)

Only needed when the module has server-rendered UI pages. Follow the same pattern as `user/page_handler.go`:
- Render HTML with `c.HTML(status, "template.html", gin.H{...})`
- Include `"CSRFToken": middleware.GetCSRFToken(c)` in template data
- htmx mutation endpoints (Create/Update/Delete) return `HX-Trigger` response headers for toast notifications

### 3.6 `module.go` — Module Interface Implementation

```go
package {name}

import "github.com/gin-gonic/gin"

// {Name}Module implements the app.Module interface for the {name} domain.
type {Name}Module struct {
    handler     *{Name}Handler
    pageHandler *{Name}PageHandler  // omit if no page handler
}

// NewModule creates a new {Name}Module with the given handlers.
func NewModule(h *{Name}Handler, ph *{Name}PageHandler) *{Name}Module {
    return &{Name}Module{handler: h, pageHandler: ph}
}

// RegisterRoutes registers {name} API and page routes.
func (m *{Name}Module) RegisterRoutes(api *gin.RouterGroup, pages *gin.RouterGroup) {
    // API routes (no CSRF — the api group already skips it)
    api.POST("/{names}", m.handler.Create)
    api.GET("/{names}/:id", m.handler.Get)
    api.GET("/{names}", m.handler.List)
    api.PUT("/{names}/:id", m.handler.Update)
    api.DELETE("/{names}/:id", m.handler.Delete)

    // Page routes (CSRF middleware applied at group level)
    pages.GET("/{names}", m.pageHandler.ListPage)
    pages.GET("/{names}/new", m.pageHandler.NewPage)
    pages.GET("/{names}/:id/edit", m.pageHandler.EditPage)
    pages.POST("/{names}", m.pageHandler.CreateHTMX)
    pages.PUT("/{names}/:id", m.pageHandler.UpdateHTMX)
    pages.DELETE("/{names}/:id", m.pageHandler.DeleteHTMX)
}
```

**Critical:** The module satisfies `app.Module` via Go's implicit interface — do **not** import the `app` package. The method signature `RegisterRoutes(api *gin.RouterGroup, pages *gin.RouterGroup)` is all that's required.

---

## 4. Wiring Into the Application

In `internal/app/app.go`, inside the `New()` function:

### 4.1 Add AutoMigrate (Debug Mode)

```go
// 3. AutoMigrate in debug mode only.
if cfg.Server.Mode == "debug" {
    if err := db.AutoMigrate(&domain.User{}, &domain.{Name}{}); err != nil {
        return nil, fmt.Errorf("auto migrate: %w", err)
    }
}
```

### 4.2 Wire Dependencies

```go
// 4. Manual dependency injection: repository → service → handler.

// User module
userRepo := user.NewUserRepository(db)
userSvc  := user.NewUserService(userRepo)
userHandler := user.NewUserHandler(userSvc)
userPageHandler := user.NewUserPageHandler(userSvc)
userModule := user.NewModule(userHandler, userPageHandler)

// {Name} module
{name}Repo := {name}.New{Name}Repository(db)
{name}Svc  := {name}.New{Name}Service({name}Repo)
{name}Handler := {name}.New{Name}Handler({name}Svc)
{name}PageHandler := {name}.New{Name}PageHandler({name}Svc)  // if applicable
{name}Module := {name}.NewModule({name}Handler, {name}PageHandler)

modules := []Module{userModule, {name}Module}
```

The `modules` slice is passed to `RegisterRoutes` via `RouteDeps.Modules`, and the loop in `routes.go` auto-registers all routes:

```go
for _, m := range deps.Modules {
    m.RegisterRoutes(api, pages)
}
```

No other wiring is needed — the module is fully active once added to the slice.

---

## 5. Test Patterns

### 5.1 Mock Service

Define a mock that implements the domain service interface in the test file:

```go
type mock{Name}Service struct {
    items  map[uint]*domain.{Name}
    nextID uint
    // error hooks
    createErr error
    getErr    error
    listErr   error
    updateErr error
    deleteErr error
}

func newMockService() *mock{Name}Service {
    return &mock{Name}Service{items: make(map[uint]*domain.{Name}), nextID: 1}
}

// Implement all domain.{Name}Service methods with in-memory map logic.
// Return injected errors when the corresponding hook is set.
```

### 5.2 Handler Test Setup

```go
func setupAPIRouter(h *{Name}Handler) *gin.Engine {
    gin.SetMode(gin.TestMode)
    r := gin.New()

    api := r.Group("/api/v1/{names}")
    api.POST("", h.Create)
    api.GET("", h.List)
    api.GET("/:id", h.Get)
    api.PUT("/:id", h.Update)
    api.DELETE("/:id", h.Delete)

    return r
}
```

### 5.3 Test Structure

```go
func TestHandler_Create(t *testing.T) {
    svc := newMockService()
    h := New{Name}Handler(svc)
    r := setupAPIRouter(h)

    body := `{"title":"Test Item"}`
    req := httptest.NewRequest(http.MethodPost, "/api/v1/{names}", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    if w.Code != http.StatusCreated {
        t.Fatalf("expected status 201, got %d", w.Code)
    }

    var resp pkg.Response
    if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
        t.Fatalf("failed to unmarshal response: %v", err)
    }
    if resp.Code != http.StatusCreated {
        t.Errorf("expected response code 201, got %d", resp.Code)
    }
}

func TestHandler_Create_ValidationError(t *testing.T) {
    svc := newMockService()
    h := New{Name}Handler(svc)
    r := setupAPIRouter(h)

    body := `{"title":""}`
    req := httptest.NewRequest(http.MethodPost, "/api/v1/{names}", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    if w.Code != http.StatusBadRequest {
        t.Fatalf("expected status 400, got %d", w.Code)
    }

    var resp pkg.ValidationErrorResponse
    if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
        t.Fatalf("failed to unmarshal response: %v", err)
    }
    if resp.Errors == nil {
        t.Fatal("expected errors map to be non-nil")
    }
}
```

### 5.4 Response Unmarshalling Types

- `pkg.Response` — for success and non-validation error responses
- `pkg.ValidationErrorResponse` — for 400 validation error responses with per-field errors

### 5.5 Page Handler Test Setup (if applicable)

```go
func setupTestRouter(h *{Name}PageHandler) *gin.Engine {
    gin.SetMode(gin.TestMode)
    r := gin.New()

    // Stub templates so c.HTML() calls don't panic.
    tmpl := template.Must(template.New("").Parse(
        `{{define "{name}/list.html"}}list{{end}}` +
        `{{define "{name}/form.html"}}form{{end}}` +
        `{{define "errors/400.html"}}400{{end}}` +
        `{{define "errors/404.html"}}404{{end}}` +
        `{{define "errors/500.html"}}500{{end}}`,
    ))
    r.SetHTMLTemplate(tmpl)

    r.GET("/{names}", h.ListPage)
    // ... other page routes
    return r
}
```

### 5.6 Module Test

```go
func TestModuleRegisterRoutes(t *testing.T) {
    gin.SetMode(gin.TestMode)
    r := gin.New()
    api := r.Group("/api")
    pages := r.Group("/")

    mod := NewModule(New{Name}Handler(nil), New{Name}PageHandler(nil))
    mod.RegisterRoutes(api, pages)

    // Verify expected routes are registered by inspecting r.Routes()
    expected := []struct {
        method string
        path   string
    }{
        {http.MethodPost, "/api/{names}"},
        {http.MethodGet, "/api/{names}/:id"},
        {http.MethodGet, "/api/{names}"},
        // ... all routes
    }

    routes := r.Routes()
    for _, exp := range expected {
        found := false
        for _, route := range routes {
            if route.Method == exp.method && route.Path == exp.path {
                found = true
                break
            }
        }
        if !found {
            t.Errorf("route %s %s not registered", exp.method, exp.path)
        }
    }
}
```

---

## 6. Checklist

Use this checklist when creating a new module:

- [ ] `internal/domain/{name}.go` — entity struct + Repository + Service interfaces
- [ ] `internal/module/{name}/dto.go` — request structs with validation tags
- [ ] `internal/module/{name}/repository.go` — GORM implementation of Repository interface
- [ ] `internal/module/{name}/service.go` — business logic implementing Service interface
- [ ] `internal/module/{name}/handler.go` — REST API handler
- [ ] `internal/module/{name}/module.go` — `RegisterRoutes` satisfying `app.Module`
- [ ] `internal/module/{name}/page_handler.go` — (optional) htmx page handler
- [ ] `internal/app/app.go` — add `&domain.{Name}{}` to `AutoMigrate`
- [ ] `internal/app/app.go` — wire repo → service → handler → module; append to `modules` slice
- [ ] `web/templates/{name}/` — (optional) HTML templates for page handler
- [ ] Tests for handler, service, repository, page handler, module
- [ ] Run `go test ./internal/module/{name}/...` and `go test ./internal/app/...`

---

## 7. Common Pitfalls

| Pitfall | Fix |
|---|---|
| Importing `app` package from module | Don't — Go implicit interfaces; just match the method signature |
| Forgetting `mapError` in repository | Copy and adapt from `user/repository.go`; handle `ErrRecordNotFound`, `ErrDuplicatedKey` |
| Missing `form` tag on DTO fields | Always include both `json` and `form` tags for htmx compatibility |
| Forgetting AutoMigrate | Add entity to `AutoMigrate` call in `app.go` |
| Not adding module to `modules` slice | Routes won't be registered — always append to the slice |
| Hard-coding HTTP status in error responses | Use `pkg.Error(c, err)` which maps domain error codes to HTTP status |
