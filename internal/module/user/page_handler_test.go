package user

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
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
}

func newMockService() *mockUserService {
	return &mockUserService{users: make(map[uint]*domain.User), nextID: 1}
}

func (m *mockUserService) CreateUser(_ context.Context, name, email string) (*domain.User, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	u := &domain.User{
		BaseModel: domain.BaseModel{ID: m.nextID},
		Name:      name,
		Email:     email,
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
	if m.listErr != nil {
		return nil, m.listErr
	}
	items := make([]domain.User, 0, len(m.users))
	for _, u := range m.users {
		items = append(items, *u)
	}
	return &domain.PageResult[domain.User]{
		Items:      items,
		Total:      int64(len(items)),
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalPages: 1,
	}, nil
}

func (m *mockUserService) UpdateUser(_ context.Context, id uint, name, email string) (*domain.User, error) {
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	u, ok := m.users[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	u.Name = name
	u.Email = email
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

	// Stub templates so c.HTML() calls don't panic.
	tmpl := template.Must(template.New("").Parse(
		`{{define "user/list.html"}}list{{end}}` +
			`{{define "user/form.html"}}form{{if .Error}}:{{.Error}}{{end}}{{end}}` +
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

func TestCreateHTMX_Success(t *testing.T) {
	svc := newMockService()
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	form := url.Values{}
	form.Set("name", "Alice")
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

	// Verify user was created in mock service.
	if len(svc.users) != 1 {
		t.Errorf("expected 1 user, got %d", len(svc.users))
	}
}

func TestCreateHTMX_ServiceError(t *testing.T) {
	svc := newMockService()
	svc.createErr = domain.NewAppError(domain.CodeAlreadyExists, "email already exists", nil)
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	form := url.Values{}
	form.Set("name", "Bob")
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
	form.Set("name", "Bob")
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
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Name: "Old", Email: "old@example.com"}
	svc.updateErr = domain.NewAppError(domain.CodeAlreadyExists, "email already exists", nil)
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	form := url.Values{}
	form.Set("name", "Updated")
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
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Name: "Old", Email: "old@example.com"}
	svc.updateErr = domain.NewAppError(domain.CodeInternal, "db connection lost", nil)
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	form := url.Values{}
	form.Set("name", "Updated")
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
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Name: "Old", Email: "old@example.com"}
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	form := url.Values{}
	form.Set("name", "Updated")
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

	// Verify the update was applied.
	if svc.users[1].Name != "Updated" {
		t.Errorf("expected name Updated, got %q", svc.users[1].Name)
	}
}

func TestUpdateHTMX_InvalidID(t *testing.T) {
	svc := newMockService()
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	form := url.Values{}
	form.Set("name", "Test")
	form.Set("email", "test@example.com")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/users/abc", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestDeleteHTMX_Success(t *testing.T) {
	svc := newMockService()
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Name: "ToDelete", Email: "del@example.com"}
	h := NewUserPageHandler(svc)
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/users/1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	trigger := w.Header().Get("HX-Trigger")
	var triggerData map[string]map[string]string
	if err := json.Unmarshal([]byte(trigger), &triggerData); err != nil {
		t.Fatalf("failed to parse HX-Trigger: %v", err)
	}
	if triggerData["showToast"]["message"] != "用户删除成功" {
		t.Errorf("expected toast message '用户删除成功', got %q", triggerData["showToast"]["message"])
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
	svc.users[1] = &domain.User{BaseModel: domain.BaseModel{ID: 1}, Name: "Test", Email: "test@example.com"}
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
