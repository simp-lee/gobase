package user

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"math/bits"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/simp-lee/gobase/internal/domain"
)

// --- mock service for page handler tests ---

type mockUserService struct {
	users  map[uint]*domain.User
	nextID uint
	// hooks for error injection
	createErr error
	getErr    error
	listErr   error
	updateErr error
	deleteErr error
	// captures the most recent ListUsers request for assertion
	lastListReq domain.PageRequest
}

func newMockService() *mockUserService {
	return &mockUserService{users: make(map[uint]*domain.User), nextID: 1}
}

func (m *mockUserService) CreateUser(_ context.Context, username, email string) (*domain.User, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	u := &domain.User{
		BaseModel: domain.BaseModel{ID: m.nextID},
		Username:  username,
		Email:     email,
		Role:      domain.RoleUser,
		Status:    domain.StatusActive,
	}
	m.users[u.ID] = u
	m.nextID++
	return u, nil
}

func (m *mockUserService) GetUser(_ context.Context, id uint) (*domain.User, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	u, ok := m.users[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return u, nil
}

func (m *mockUserService) ListUsers(_ context.Context, req domain.PageRequest) (*domain.PageResult[domain.User], error) {
	m.lastListReq = req
	if m.listErr != nil {
		return nil, m.listErr
	}
	items := make([]domain.User, 0, len(m.users))
	for _, u := range m.users {
		items = append(items, *u)
	}
	return &domain.PageResult[domain.User]{
		Items:       items,
		TotalItems:  int64(len(items)),
		CurrentPage: req.Page,
		PageSize:    req.PageSize,
		TotalPages:  1,
	}, nil
}

func (m *mockUserService) UpdateUser(ctx context.Context, id uint, username, email, role, status string) (*domain.User, error) {
	// Mirror production semantics: non-admin callers have admin-only fields
	// silently ignored rather than rejected.
	if role != "" && !isAdminFieldAuthorized(ctx) {
		role = ""
	}
	if status != "" && !isAdminFieldAuthorized(ctx) {
		status = ""
	}
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	u, ok := m.users[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	u.Username = username
	u.Email = email
	if role != "" {
		u.Role = role
	}
	if status != "" {
		u.Status = status
	}
	return u, nil
}

func (m *mockUserService) DeleteUser(_ context.Context, id uint) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if _, ok := m.users[id]; !ok {
		return domain.ErrNotFound
	}
	delete(m.users, id)
	return nil
}

// --- helper to set up gin test router with minimal templates ---

// setupTestRouter creates a gin engine for handler testing.
// Template rendering is not verified here; we focus on status codes, headers, and error paths.
// For endpoints that call c.HTML, the router uses a stub HTML renderer.
func setupTestRouter(h *UserPageHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if role := c.GetHeader("X-Test-Role"); role != "" {
			c.Set("role", role)
		}
		c.Next()
	})

	// Stub templates so c.HTML() calls don't panic.
	tmpl := template.Must(template.New("").Parse(
		`{{define "user/list.html"}}list:BaseURL={{.BaseURL}}:HasPagination={{if .Pagination}}yes{{else}}no{{end}}:StatusFilter={{.StatusFilter}}:FilterQuery={{.FilterQuery}}:StatusSortQuery={{.StatusSortQuery}}:StatusSortDirection={{.StatusSortDirection}}:FlashSuccess={{if .Flash}}{{.Flash.Success}}{{end}}{{end}}` +
			`{{define "user/list_fragment.html"}}fragment:BaseURL={{.BaseURL}}:HasPagination={{if .Pagination}}yes{{else}}no{{end}}:StatusFilter={{.StatusFilter}}:FilterQuery={{.FilterQuery}}:StatusSortQuery={{.StatusSortQuery}}:StatusSortDirection={{.StatusSortDirection}}:FlashSuccess={{if .Flash}}{{.Flash.Success}}{{end}}{{end}}` +
			`{{define "user/form.html"}}form{{if .Error}}:{{.Error}}{{end}}{{end}}` +
			`{{define "errors/400.html"}}400{{end}}` +
			`{{define "errors/404.html"}}404{{end}}` +
			`{{define "errors/500.html"}}500{{end}}`,
	))
	r.SetHTMLTemplate(tmpl)

	// Register routes matching the real app.
	r.GET("/users", h.ListPage)
	r.GET("/users/new", h.NewPage)
	r.GET("/users/:id/edit", h.EditPage)
	r.POST("/users", h.CreateHTMX)
	r.PUT("/users/:id", h.UpdateHTMX)
	r.DELETE("/users/:id", h.DeleteHTMX)

	return r
}

func setupTestRouterWithRealTemplates(t *testing.T, h *UserPageHandler) *gin.Engine {
	t.Helper()
	r := setupTestRouter(h)
	r.SetHTMLTemplate(loadRealUserFormTemplate(t))
	return r
}

func loadRealUserFormTemplate(t *testing.T) *template.Template {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve current file path")
	}

	templatesRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "web", "templates"))
	files := []struct {
		name string
		path string
	}{
		{name: "templates/layouts/base.html", path: filepath.Join(templatesRoot, "layouts", "base.html")},
		{name: "templates/partials/nav.html", path: filepath.Join(templatesRoot, "partials", "nav.html")},
		{name: "templates/partials/scripts_common.html", path: filepath.Join(templatesRoot, "partials", "scripts_common.html")},
		{name: "user/form.html", path: filepath.Join(templatesRoot, "user", "form.html")},
	}

	tmpl := template.New("")
	for _, f := range files {
		content, err := os.ReadFile(f.path)
		if err != nil {
			t.Fatalf("failed to read template %s: %v", f.path, err)
		}
		if _, err := tmpl.New(f.name).Parse(string(content)); err != nil {
			t.Fatalf("failed to parse template %s: %v", f.path, err)
		}
	}

	return tmpl
}

// --- tests ---

func TestNewUserPageHandler(t *testing.T) {
	svc := newMockService()
	h := NewUserPageHandler(svc)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.svc != svc {
		t.Fatal("expected handler to hold the given service")
	}
}

func TestListPage_Success(t *testing.T) {
	svc := newMockService()
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Username: "Alice", Email: "alice@example.com"}
	svc.users[2] = &domain.User{BaseModel: domain.BaseModel{ID: 2}, Username: "Bob", Email: "bob@example.com"}
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/users", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	// Verify Pagination object is passed to template.
	if !strings.Contains(body, "HasPagination=yes") {
		t.Errorf("expected template data to include Pagination, got %q", body)
	}
	// Verify BaseURL is passed to template.
	if !strings.Contains(body, "BaseURL=/users") {
		t.Errorf("expected template data to include BaseURL=/users, got %q", body)
	}
}

func TestListPage_ServiceError(t *testing.T) {
	svc := newMockService()
	svc.listErr = errors.New("db connection lost")
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/users", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "500") {
		t.Errorf("expected 500 error template body, got %q", w.Body.String())
	}
}

func TestCreateHTMX_Success(t *testing.T) {
	svc := newMockService()
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	form := url.Values{}
	form.Set("username", "Alice")
	form.Set("email", "alice@example.com")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	// Verify HX-Redirect header.
	if got := w.Header().Get("HX-Redirect"); got != "/users" {
		t.Errorf("expected HX-Redirect /users, got %q", got)
	}

	// Verify HX-Trigger header contains showToast.
	trigger := w.Header().Get("HX-Trigger")
	if trigger == "" {
		t.Fatal("expected HX-Trigger header to be set")
	}
	var triggerData map[string]map[string]string
	if err := json.Unmarshal([]byte(trigger), &triggerData); err != nil {
		t.Fatalf("failed to parse HX-Trigger: %v", err)
	}
	toast, ok := triggerData["showToast"]
	if !ok {
		t.Fatal("expected showToast in HX-Trigger")
	}
	if toast["type"] != "success" {
		t.Errorf("expected toast type success, got %q", toast["type"])
	}
	if toast["message"] != "用户创建成功" {
		t.Errorf("expected toast message '用户创建成功', got %q", toast["message"])
	}
	if cookies := w.Header().Values("Set-Cookie"); len(cookies) == 0 || !strings.Contains(strings.Join(cookies, "\n"), flashToastCookieName+"=") {
		t.Fatalf("expected flash toast cookie to be set, got %v", cookies)
	}

	// Verify user was created in mock service.
	if len(svc.users) != 1 {
		t.Errorf("expected 1 user, got %d", len(svc.users))
	}
}

func TestListPage_ConsumesFlashToastCookie(t *testing.T) {
	svc := newMockService()
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/users", nil)
	req.AddCookie(&http.Cookie{Name: flashToastCookieName, Value: "success:%E7%94%A8%E6%88%B7%E5%88%9B%E5%BB%BA%E6%88%90%E5%8A%9F"})
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "FlashSuccess=用户创建成功") {
		t.Fatalf("expected flash success in template data, got %q", body)
	}
	if cookies := w.Header().Values("Set-Cookie"); len(cookies) == 0 || !strings.Contains(strings.Join(cookies, "\n"), flashToastCookieName+"=;") {
		t.Fatalf("expected flash toast cookie to be cleared, got %v", cookies)
	}
}

func TestCreateHTMX_ServiceError(t *testing.T) {
	svc := newMockService()
	svc.createErr = domain.NewAppError(domain.CodeAlreadyExists, "email already exists", nil)
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	form := url.Values{}
	form.Set("username", "Bob")
	form.Set("email", "bob@example.com")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)

	// On service error the handler re-renders the form (200 with error in data).
	// HX-Redirect should NOT be set and error message should be rendered.
	if got := w.Header().Get("HX-Redirect"); got != "" {
		t.Errorf("expected no HX-Redirect on error, got %q", got)
	}
	if !strings.Contains(w.Body.String(), "email already exists") {
		t.Errorf("expected response body to include error message, got %q", w.Body.String())
	}
}

func TestCreateHTMX_InternalError(t *testing.T) {
	svc := newMockService()
	svc.createErr = domain.NewAppError(domain.CodeInternal, "db connection lost", nil)
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	form := url.Values{}
	form.Set("username", "Bob")
	form.Set("email", "bob@example.com")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "创建用户失败，请稍后重试") {
		t.Errorf("expected fallback message in body, got %q", body)
	}
	if strings.Contains(body, "db connection lost") {
		t.Errorf("expected technical detail to be hidden, but body contains 'db connection lost': %q", body)
	}
}

func TestUpdateHTMX_ServiceError_RendersErrorMessage(t *testing.T) {
	svc := newMockService()
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Username: "Old", Email: "old@example.com"}
	svc.updateErr = domain.NewAppError(domain.CodeAlreadyExists, "email already exists", nil)
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	form := url.Values{}
	form.Set("username", "Updated")
	form.Set("email", "updated@example.com")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/users/1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if got := w.Header().Get("HX-Redirect"); got != "" {
		t.Errorf("expected no HX-Redirect on error, got %q", got)
	}
	if !strings.Contains(w.Body.String(), "email already exists") {
		t.Errorf("expected response body to include error message, got %q", w.Body.String())
	}
}

func TestUpdateHTMX_InternalError(t *testing.T) {
	svc := newMockService()
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Username: "Old", Email: "old@example.com"}
	svc.updateErr = domain.NewAppError(domain.CodeInternal, "db connection lost", nil)
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	form := url.Values{}
	form.Set("username", "Updated")
	form.Set("email", "updated@example.com")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/users/1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "更新用户失败，请稍后重试") {
		t.Errorf("expected fallback message in body, got %q", body)
	}
	if strings.Contains(body, "db connection lost") {
		t.Errorf("expected technical detail to be hidden, but body contains 'db connection lost': %q", body)
	}
}

func TestUpdateHTMX_Success(t *testing.T) {
	svc := newMockService()
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Username: "Old", Email: "old@example.com"}
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	form := url.Values{}
	form.Set("username", "Updated")
	form.Set("email", "updated@example.com")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/users/1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	if got := w.Header().Get("HX-Redirect"); got != "/users" {
		t.Errorf("expected HX-Redirect /users, got %q", got)
	}

	trigger := w.Header().Get("HX-Trigger")
	var triggerData map[string]map[string]string
	if err := json.Unmarshal([]byte(trigger), &triggerData); err != nil {
		t.Fatalf("failed to parse HX-Trigger: %v", err)
	}
	if triggerData["showToast"]["message"] != "用户更新成功" {
		t.Errorf("expected toast message '用户更新成功', got %q", triggerData["showToast"]["message"])
	}
	if cookies := w.Header().Values("Set-Cookie"); len(cookies) == 0 || !strings.Contains(strings.Join(cookies, "\n"), flashToastCookieName+"=") {
		t.Fatalf("expected flash toast cookie to be set, got %v", cookies)
	}

	// Verify the update was applied.
	if svc.users[1].Username != "Updated" {
		t.Errorf("expected username Updated, got %q", svc.users[1].Username)
	}
}

func TestUpdateHTMX_RoleAdminSuccess(t *testing.T) {
	svc := newMockService()
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Username: "Old", Email: "old@example.com", Role: domain.RoleUser}
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	form := url.Values{}
	form.Set("username", "Updated")
	form.Set("email", "updated@example.com")
	form.Set("role", "admin")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/users/1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Test-Role", "admin")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if svc.users[1].Role != domain.RoleAdmin {
		t.Fatalf("expected role %q, got %q", domain.RoleAdmin, svc.users[1].Role)
	}
}

func TestEditPage_RealTemplate_AdminShowsRoleField(t *testing.T) {
	svc := newMockService()
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Username: "Old", Email: "old@example.com", Role: domain.RoleUser}
	h := NewUserPageHandler(svc)
	r := setupTestRouterWithRealTemplates(t, h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/users/1/edit", nil)
	req.Header.Set("X-Test-Role", "admin")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "name=\"role\"") {
		t.Fatalf("expected role field in admin edit form, got %q", body)
	}
	if !strings.Contains(body, "<option value=\"user\" selected>") {
		t.Fatalf("expected current role option to be selected, got %q", body)
	}
}

func TestEditPage_RealTemplate_NonAdminHidesRoleField(t *testing.T) {
	svc := newMockService()
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Username: "Old", Email: "old@example.com", Role: domain.RoleUser}
	h := NewUserPageHandler(svc)
	r := setupTestRouterWithRealTemplates(t, h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/users/1/edit", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "name=\"role\"") {
		t.Fatalf("expected role field to be hidden for non-admin edit form, got %q", body)
	}
}

func TestEditPage_RealTemplate_AdminShowsStatusField(t *testing.T) {
	svc := newMockService()
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Username: "Old", Email: "old@example.com", Role: domain.RoleUser, Status: domain.StatusActive}
	h := NewUserPageHandler(svc)
	r := setupTestRouterWithRealTemplates(t, h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/users/1/edit", nil)
	req.Header.Set("X-Test-Role", "admin")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "name=\"status\"") {
		t.Fatalf("expected status field in admin edit form, got %q", body)
	}
	if !strings.Contains(body, "<option value=\"active\" selected>") {
		t.Fatalf("expected current status option to be selected, got %q", body)
	}
}

func TestEditPage_RealTemplate_NonAdminHidesStatusField(t *testing.T) {
	svc := newMockService()
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Username: "Old", Email: "old@example.com", Role: domain.RoleUser, Status: domain.StatusActive}
	h := NewUserPageHandler(svc)
	r := setupTestRouterWithRealTemplates(t, h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/users/1/edit", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "name=\"status\"") {
		t.Fatalf("expected status field to be hidden for non-admin edit form, got %q", body)
	}
}

func TestListPage_StatusFilterPassedToTemplate(t *testing.T) {
	svc := newMockService()
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Username: "Alice", Email: "alice@example.com", Status: domain.StatusActive}
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/users?status=active", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "StatusFilter=active") {
		t.Errorf("expected StatusFilter=active in template data, got %q", body)
	}
	if !strings.Contains(body, "status=active") {
		t.Errorf("expected FilterQuery to contain status=active, got %q", body)
	}
}

func TestListPage_StatusSortLinkPassedToTemplate(t *testing.T) {
	svc := newMockService()
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Username: "Alice", Email: "alice@example.com", Status: domain.StatusActive}
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/users?page=2&page_size=15&status=active&sort=status:asc", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "StatusSortDirection=asc") {
		t.Fatalf("expected StatusSortDirection=asc in template data, got %q", body)
	}
	if !strings.Contains(body, "StatusSortQuery=page=2&amp;page_size=15&amp;sort=status%3Adesc&amp;status=active") {
		t.Fatalf("expected toggled status sort query in template data, got %q", body)
	}
}

func TestListPage_HTMXRendersFragmentAndPushesURL(t *testing.T) {
	svc := newMockService()
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Username: "Alice", Email: "alice@example.com", Status: domain.StatusActive}
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/users?page=2&page_size=1&status=active&sort=status:asc", nil)
	req.Header.Set("HX-Request", "true")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if got := w.Header().Get("HX-Push-Url"); got != "/users?page=2&page_size=1&status=active&sort=status:asc" {
		t.Fatalf("expected HX-Push-Url to match request URI, got %q", got)
	}
	body := w.Body.String()
	if !strings.HasPrefix(body, "fragment:") {
		t.Fatalf("expected fragment template body, got %q", body)
	}
	if strings.Contains(body, "list:") {
		t.Fatalf("expected full-page template not to be used for HTMX requests, got %q", body)
	}
	if svc.lastListReq.Filter["status"] != domain.StatusActive {
		t.Fatalf("expected status filter to be preserved, got %q", svc.lastListReq.Filter["status"])
	}
	if svc.lastListReq.Sort != "status:asc" {
		t.Fatalf("expected sort to be preserved, got %q", svc.lastListReq.Sort)
	}
	if svc.lastListReq.Page != 2 || svc.lastListReq.PageSize != 1 {
		t.Fatalf("expected pagination to be preserved, got page=%d page_size=%d", svc.lastListReq.Page, svc.lastListReq.PageSize)
	}
}

func TestUpdateHTMX_InvalidID(t *testing.T) {
	svc := newMockService()
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	form := url.Values{}
	form.Set("username", "Test")
	form.Set("email", "test@example.com")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/users/abc", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "400") {
		t.Fatalf("expected 400 template body, got %q", w.Body.String())
	}
}

func TestEditPage_InvalidID(t *testing.T) {
	svc := newMockService()
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/users/abc/edit", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "400") {
		t.Fatalf("expected 400 template body, got %q", w.Body.String())
	}
}

func TestUpdateHTMX_BindError_GetUserInternalError(t *testing.T) {
	svc := newMockService()
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Username: "Old", Email: "old@example.com"}
	svc.getErr = errors.New("db connection lost")
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	form := url.Values{}
	form.Set("username", "A")
	form.Set("email", "not-an-email")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/users/1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}
}

func TestUpdateHTMX_UpdateError_GetUserInternalError(t *testing.T) {
	svc := newMockService()
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Username: "Old", Email: "old@example.com"}
	svc.updateErr = errors.New("update failed")
	svc.getErr = errors.New("db connection lost")
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	form := url.Values{}
	form.Set("username", "Updated")
	form.Set("email", "updated@example.com")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/users/1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}
}

func TestDeleteHTMX_Success(t *testing.T) {
	svc := newMockService()
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Username: "ToDelete", Email: "del@example.com"}
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/users/1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	if got := w.Header().Get("HX-Reswap"); got != "delete" {
		t.Fatalf("expected HX-Reswap delete for delete success, got %q", got)
	}

	if got := w.Header().Get("HX-Trigger"); got != "" {
		t.Fatalf("expected HX-Trigger to be empty for delete success, got %q", got)
	}

	if got := w.Header().Get("HX-Trigger-After-Settle"); got != "" {
		t.Fatalf("expected HX-Trigger-After-Settle to be empty for delete success, got %q", got)
	}

	body := w.Body.String()
	if !strings.Contains(body, `hx-swap-oob="beforeend:body"`) {
		t.Fatalf("expected delete success body to include OOB swap, got %q", body)
	}
	if !strings.Contains(body, "用户删除成功") {
		t.Fatalf("expected delete success body to include toast message, got %q", body)
	}

	// Verify user was removed.
	if len(svc.users) != 0 {
		t.Errorf("expected 0 users after delete, got %d", len(svc.users))
	}
}

func TestDeleteHTMX_NotFound(t *testing.T) {
	svc := newMockService()
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/users/999", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	if got := w.Header().Get("HX-Reswap"); got != "none" {
		t.Errorf("expected HX-Reswap 'none', got %q", got)
	}

	trigger := w.Header().Get("HX-Trigger")
	if trigger == "" {
		t.Fatal("expected HX-Trigger header to be set")
	}
	var triggerData map[string]map[string]string
	if err := json.Unmarshal([]byte(trigger), &triggerData); err != nil {
		t.Fatalf("failed to parse HX-Trigger: %v", err)
	}
	toast, ok := triggerData["showToast"]
	if !ok {
		t.Fatal("expected showToast in HX-Trigger")
	}
	if toast["type"] != "error" {
		t.Errorf("expected toast type error, got %q", toast["type"])
	}
}

func TestDeleteHTMX_InvalidID(t *testing.T) {
	svc := newMockService()
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/users/abc", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	if got := w.Header().Get("HX-Reswap"); got != "none" {
		t.Errorf("expected HX-Reswap 'none', got %q", got)
	}

	trigger := w.Header().Get("HX-Trigger")
	if trigger == "" {
		t.Fatal("expected HX-Trigger header to be set")
	}
	var triggerData map[string]map[string]string
	if err := json.Unmarshal([]byte(trigger), &triggerData); err != nil {
		t.Fatalf("failed to parse HX-Trigger: %v", err)
	}
	toast, ok := triggerData["showToast"]
	if !ok {
		t.Fatal("expected showToast in HX-Trigger")
	}
	if toast["type"] != "error" {
		t.Errorf("expected toast type error, got %q", toast["type"])
	}
}

func TestDeleteHTMX_InternalError(t *testing.T) {
	svc := newMockService()
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Username: "Test", Email: "test@example.com"}
	svc.deleteErr = errors.New("db connection lost")
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/users/1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	if got := w.Header().Get("HX-Reswap"); got != "none" {
		t.Errorf("expected HX-Reswap 'none', got %q", got)
	}

	trigger := w.Header().Get("HX-Trigger")
	if trigger == "" {
		t.Fatal("expected HX-Trigger header to be set")
	}
	var triggerData map[string]map[string]string
	if err := json.Unmarshal([]byte(trigger), &triggerData); err != nil {
		t.Fatalf("failed to parse HX-Trigger: %v", err)
	}
	toast, ok := triggerData["showToast"]
	if !ok {
		t.Fatal("expected showToast in HX-Trigger")
	}
	if toast["type"] != "error" {
		t.Errorf("expected toast type error, got %q", toast["type"])
	}
	if toast["message"] != "删除失败，请稍后重试" {
		t.Errorf("expected toast message '删除失败，请稍后重试', got %q", toast["message"])
	}
}

func TestParseID(t *testing.T) {
	tests := []struct {
		name    string
		param   string
		wantID  uint
		wantErr bool
	}{
		{"valid", "1", 1, false},
		{"large", "42", 42, false},
		{"zero", "0", 0, true},
		{"negative", "-1", 0, true},
		{"non-numeric", "abc", 0, true},
		{"empty", "", 0, true},
		{"max-uint", strconv.FormatUint(uint64(^uint(0)), 10), uint(^uint(0)), false},
		{"overflow-uint64", "18446744073709551616", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{{Key: "id", Value: tt.param}}

			id, err := parseID(c)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if id != tt.wantID {
				t.Errorf("parseID() = %v, want %v", id, tt.wantID)
			}
		})
	}
}

func TestParseID_ArchitectureBoundary(t *testing.T) {
	tests := []struct {
		name    string
		param   string
		wantErr bool
	}{
		{
			name:    "over-uint32-boundary",
			param:   "4294967296",
			wantErr: bits.UintSize == 32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{{Key: "id", Value: tt.param}}

			_, err := parseID(c)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSetShowToastHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	setShowToastHeader(c, "操作成功", "success")

	trigger := w.Header().Get("HX-Trigger")
	if trigger == "" {
		t.Fatal("expected HX-Trigger header")
	}

	var data map[string]map[string]string
	if err := json.Unmarshal([]byte(trigger), &data); err != nil {
		t.Fatalf("failed to parse HX-Trigger JSON: %v", err)
	}

	toast := data["showToast"]
	if toast["message"] != "操作成功" {
		t.Errorf("expected message '操作成功', got %q", toast["message"])
	}
	if toast["type"] != "success" {
		t.Errorf("expected type 'success', got %q", toast["type"])
	}
	for _, ch := range trigger {
		if ch > 127 {
			t.Fatalf("expected HX-Trigger header to remain ASCII-only, got %q", trigger)
		}
	}
}

func TestToUserPaginationView_NormalizesTotalPages(t *testing.T) {
	view := toUserPaginationView(&domain.PageResult[domain.User]{
		CurrentPage: 0,
		PageSize:    10,
		TotalPages:  0,
	})

	if view.TotalPages != 1 {
		t.Fatalf("TotalPages=%d; want 1", view.TotalPages)
	}
	if view.LastPage != 1 {
		t.Fatalf("LastPage=%d; want 1", view.LastPage)
	}
	if len(view.Pages) != 1 || view.Pages[0] != 1 {
		t.Fatalf("Pages=%v; want [1]", view.Pages)
	}
}

func TestToUserPaginationView_UsesWindowedPages(t *testing.T) {
	view := toUserPaginationView(&domain.PageResult[domain.User]{
		CurrentPage: 50,
		PageSize:    20,
		TotalPages:  100,
	})

	if view.FirstPageInRange != 48 {
		t.Fatalf("FirstPageInRange=%d; want 48", view.FirstPageInRange)
	}
	if view.LastPageInRange != 52 {
		t.Fatalf("LastPageInRange=%d; want 52", view.LastPageInRange)
	}
	wantPages := []int{48, 49, 50, 51, 52}
	if !reflect.DeepEqual(view.Pages, wantPages) {
		t.Fatalf("Pages=%v; want %v", view.Pages, wantPages)
	}
}

func TestToUserPaginationView_ClampsCurrentPageToTotalPages(t *testing.T) {
	view := toUserPaginationView(&domain.PageResult[domain.User]{
		CurrentPage: 9,
		PageSize:    20,
		TotalPages:  3,
	})

	if view.CurrentPage != 3 {
		t.Fatalf("CurrentPage=%d; want 3", view.CurrentPage)
	}
	if view.NextPage != 3 {
		t.Fatalf("NextPage=%d; want 3", view.NextPage)
	}
	if view.HasNextPage {
		t.Fatal("HasNextPage=true; want false")
	}
	wantPages := []int{1, 2, 3}
	if !reflect.DeepEqual(view.Pages, wantPages) {
		t.Fatalf("Pages=%v; want %v", view.Pages, wantPages)
	}
}

func Test_safePageErrorMessage(t *testing.T) {
	fallback := "操作失败，请稍后重试"

	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil error", nil, fallback},
		{"plain error", errors.New("something broke"), fallback},
		{"CodeNotFound", domain.NewAppError(domain.CodeNotFound, "用户不存在", nil), "用户不存在"},
		{"CodeAlreadyExists", domain.NewAppError(domain.CodeAlreadyExists, "邮箱已存在", nil), "邮箱已存在"},
		{"CodeValidation", domain.NewAppError(domain.CodeValidation, "名称不能为空", nil), "名称不能为空"},
		{"CodeInternal returns fallback", domain.NewAppError(domain.CodeInternal, "database error", nil), fallback},
		{"unknown code returns fallback", domain.NewAppError(999, "secret info", nil), fallback},
		{"empty message returns fallback", domain.NewAppError(domain.CodeNotFound, "", nil), fallback},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safePageErrorMessage(tt.err, fallback)
			if got != tt.want {
				t.Errorf("safePageErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}
