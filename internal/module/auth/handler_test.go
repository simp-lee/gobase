package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/simp-lee/gobase/internal/domain"
	"github.com/simp-lee/gobase/internal/pkg"
)

// mockService implements Service for handler testing.
type mockService struct {
	loginResp   *TokenResponse
	loginErr    error
	registerRes *domain.User
	registerErr error
}

func (m *mockService) Login(_ context.Context, _, _ string) (*TokenResponse, error) {
	return m.loginResp, m.loginErr
}

func (m *mockService) Register(_ context.Context, _, _, _ string) (*domain.User, error) {
	return m.registerRes, m.registerErr
}

func setupAuthRouter(h *AuthHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	NewModule(h).RegisterRoutes(api, nil)
	return r
}

func TestAuthHandler_Login_Success(t *testing.T) {
	svc := &mockService{
		loginResp: &TokenResponse{Token: "tok-123", ExpiresAt: 1700000000},
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	body := `{"email":"alice@example.com","password":"secret1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Token     string `json:"token"`
			ExpiresAt int64  `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Code != http.StatusOK {
		t.Errorf("expected response code 200, got %d", resp.Code)
	}
	if resp.Message != "success" {
		t.Errorf("expected message 'success', got %q", resp.Message)
	}
	if resp.Data.Token != "tok-123" {
		t.Errorf("expected token 'tok-123', got %q", resp.Data.Token)
	}
	if resp.Data.ExpiresAt != 1700000000 {
		t.Errorf("expected expires_at 1700000000, got %d", resp.Data.ExpiresAt)
	}
}

func TestAuthHandler_Login_ValidationError(t *testing.T) {
	svc := &mockService{}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	// Missing required fields
	body := `{"email":"","password":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
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
}

func TestAuthHandler_Login_ServiceError(t *testing.T) {
	svc := &mockService{
		loginErr: domain.ErrUnauthorized,
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	body := `{"email":"alice@example.com","password":"wrongpassword"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", w.Code)
	}
}

func TestAuthHandler_Register_Success(t *testing.T) {
	svc := &mockService{
		registerRes: &domain.User{
			BaseModel: domain.BaseModel{ID: 1},
			Name:      "Alice",
			Email:     "alice@example.com",
		},
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	body := `{"name":"Alice","email":"alice@example.com","password":"secret1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", w.Code)
	}

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			ID        uint   `json:"id"`
			Name      string `json:"name"`
			Email     string `json:"email"`
			CreatedAt string `json:"created_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Code != http.StatusCreated {
		t.Errorf("expected response code 201, got %d", resp.Code)
	}
	if resp.Message != "user registered successfully" {
		t.Errorf("expected message 'user registered successfully', got %q", resp.Message)
	}
	if resp.Data.ID != 1 {
		t.Errorf("expected data.id = 1, got %d", resp.Data.ID)
	}
	if resp.Data.Name != "Alice" {
		t.Errorf("expected data.name = 'Alice', got %q", resp.Data.Name)
	}
	if resp.Data.Email != "alice@example.com" {
		t.Errorf("expected data.email = 'alice@example.com', got %q", resp.Data.Email)
	}
	if resp.Data.CreatedAt == "" {
		t.Error("expected data.created_at to be non-empty")
	}
}

func TestAuthHandler_Register_ValidationError(t *testing.T) {
	svc := &mockService{}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	// Missing required fields
	body := `{"name":"","email":"","password":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
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
}

func TestAuthHandler_Register_ServiceError(t *testing.T) {
	svc := &mockService{
		registerErr: domain.NewAppError(domain.CodeAlreadyExists, "email already exists", nil),
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	body := `{"name":"Alice","email":"alice@example.com","password":"secret1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", w.Code)
	}
}
