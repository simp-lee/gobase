package user

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/simp-lee/gobase/internal/domain"
	"github.com/simp-lee/gobase/internal/pkg"
)

// setupAPIRouter creates a gin engine with REST API routes for handler testing.
func setupAPIRouter(h *UserHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	api := r.Group("/api/v1/users")
	api.POST("", h.Create)
	api.GET("", h.List)
	api.GET("/:id", h.Get)
	api.PUT("/:id", h.Update)
	api.DELETE("/:id", h.Delete)

	return r
}

func TestUserHandler_Create(t *testing.T) {
	svc := newMockService()
	h := NewUserHandler(svc)
	r := setupAPIRouter(h)

	body := `{"name":"Alice","email":"alice@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
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
	if resp.Message != "success" {
		t.Errorf("expected message 'success', got %q", resp.Message)
	}
}

func TestUserHandler_Create_ValidationError(t *testing.T) {
	svc := newMockService()
	h := NewUserHandler(svc)
	r := setupAPIRouter(h)

	// Missing required fields
	body := `{"name":"","email":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
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
	if resp.Code != http.StatusBadRequest {
		t.Errorf("expected code 400, got %d", resp.Code)
	}
	if resp.Message != "validation error" {
		t.Errorf("expected message 'validation error', got %q", resp.Message)
	}
	if resp.Errors == nil {
		t.Fatal("expected errors map to be non-nil")
	}
	if _, ok := resp.Errors["name"]; !ok {
		t.Error("expected 'name' field in errors map")
	}
	if _, ok := resp.Errors["email"]; !ok {
		t.Error("expected 'email' field in errors map")
	}
}

func TestUserHandler_Create_ServiceError(t *testing.T) {
	svc := newMockService()
	svc.createErr = domain.NewAppError(domain.CodeAlreadyExists, "email already exists", nil)
	h := NewUserHandler(svc)
	r := setupAPIRouter(h)

	body := `{"name":"Alice","email":"alice@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", w.Code)
	}
}

func TestUserHandler_Get(t *testing.T) {
	svc := newMockService()
	// Seed a user
	svc.users[1] = &domain.User{
		BaseModel: domain.BaseModel{ID: 1},
		Name:      "Alice",
		Email:     "alice@example.com",
	}
	h := NewUserHandler(svc)
	r := setupAPIRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp pkg.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Code != http.StatusOK {
		t.Errorf("expected response code 200, got %d", resp.Code)
	}
}

func TestUserHandler_Get_NotFound(t *testing.T) {
	svc := newMockService()
	h := NewUserHandler(svc)
	r := setupAPIRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}
}

func TestUserHandler_Get_InvalidID(t *testing.T) {
	svc := newMockService()
	h := NewUserHandler(svc)
	r := setupAPIRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestUserHandler_List(t *testing.T) {
	svc := newMockService()
	svc.users[1] = &domain.User{
		BaseModel: domain.BaseModel{ID: 1},
		Name:      "Alice",
		Email:     "alice@example.com",
	}
	svc.users[2] = &domain.User{
		BaseModel: domain.BaseModel{ID: 2},
		Name:      "Bob",
		Email:     "bob@example.com",
	}
	h := NewUserHandler(svc)
	r := setupAPIRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp pkg.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Code != http.StatusOK {
		t.Errorf("expected response code 200, got %d", resp.Code)
	}
}

func TestUserHandler_List_PaginationParams(t *testing.T) {
	svc := newMockService()
	for i := uint(1); i <= 10; i++ {
		svc.users[i] = &domain.User{
			BaseModel: domain.BaseModel{ID: i},
			Name:      "User",
			Email:     "user@example.com",
		}
	}
	h := NewUserHandler(svc)
	r := setupAPIRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users?page=2&page_size=5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp pkg.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected Data to be a map, got %T", resp.Data)
	}
	if page, _ := data["current_page"].(float64); int(page) != 2 {
		t.Errorf("expected current_page=2, got %v", data["current_page"])
	}
	if pageSize, _ := data["items_per_page"].(float64); int(pageSize) != 5 {
		t.Errorf("expected items_per_page=5, got %v", data["items_per_page"])
	}
}

func TestUserHandler_List_ServiceError(t *testing.T) {
	svc := newMockService()
	svc.listErr = domain.NewAppError(domain.CodeInternal, "db error", nil)
	h := NewUserHandler(svc)
	r := setupAPIRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}
}

func TestUserHandler_Update(t *testing.T) {
	svc := newMockService()
	svc.users[1] = &domain.User{
		BaseModel: domain.BaseModel{ID: 1},
		Name:      "Alice",
		Email:     "alice@example.com",
	}
	h := NewUserHandler(svc)
	r := setupAPIRouter(h)

	body := `{"name":"Alice Updated","email":"alice2@example.com"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp pkg.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Message != "success" {
		t.Errorf("expected message 'success', got %q", resp.Message)
	}
}

func TestUserHandler_Update_InvalidID(t *testing.T) {
	svc := newMockService()
	h := NewUserHandler(svc)
	r := setupAPIRouter(h)

	body := `{"name":"Alice","email":"alice@example.com"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/abc", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestUserHandler_Update_ValidationError(t *testing.T) {
	svc := newMockService()
	h := NewUserHandler(svc)
	r := setupAPIRouter(h)

	body := `{"name":"","email":"invalid"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/1", strings.NewReader(body))
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
	if _, ok := resp.Errors["name"]; !ok {
		t.Error("expected 'name' field in errors map")
	}
	if _, ok := resp.Errors["email"]; !ok {
		t.Error("expected 'email' field in errors map")
	}
}

func TestUserHandler_Update_NotFound(t *testing.T) {
	svc := newMockService()
	h := NewUserHandler(svc)
	r := setupAPIRouter(h)

	body := `{"name":"Alice","email":"alice@example.com"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/999", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}
}

func TestUserHandler_Delete(t *testing.T) {
	svc := newMockService()
	svc.users[1] = &domain.User{
		BaseModel: domain.BaseModel{ID: 1},
		Name:      "Alice",
		Email:     "alice@example.com",
	}
	h := NewUserHandler(svc)
	r := setupAPIRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/users/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestUserHandler_Delete_NotFound(t *testing.T) {
	svc := newMockService()
	h := NewUserHandler(svc)
	r := setupAPIRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/users/999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}
}

func TestUserHandler_Delete_InvalidID(t *testing.T) {
	svc := newMockService()
	h := NewUserHandler(svc)
	r := setupAPIRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/users/abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}
