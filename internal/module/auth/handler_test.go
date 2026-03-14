package auth

import (
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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
	logoutErr   error
	refreshResp *TokenResponse
	refreshErr  error
}

func (m *mockService) Login(_ context.Context, _, _ string) (*TokenResponse, error) {
	return m.loginResp, m.loginErr
}

func (m *mockService) Register(_ context.Context, _, _, _ string) (*domain.User, error) {
	return m.registerRes, m.registerErr
}

func (m *mockService) Logout(_ context.Context, _ string) error {
	return m.logoutErr
}

func (m *mockService) RefreshToken(_ context.Context, _ string) (*TokenResponse, error) {
	return m.refreshResp, m.refreshErr
}

func setupAuthRouter(h *AuthHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	NewModule(h).RegisterRoutes(api, nil)
	return r
}

func setupAuthPageRouter(t *testing.T, h *AuthHandler, page string) *gin.Engine {
	t.Helper()
	r := gin.New()
	r.SetHTMLTemplate(loadRealAuthTemplate(t, page))
	r.GET("/login", h.LoginPage)
	r.GET("/register", h.RegisterPage)
	return r
}

func loadRealAuthTemplate(t *testing.T, page string) *template.Template {
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
		{name: "templates/partials/scripts_common.html", path: filepath.Join(templatesRoot, "partials", "scripts_common.html")},
	}

	switch page {
	case "login":
		files = append(files, struct {
			name string
			path string
		}{name: "auth/login.html", path: filepath.Join(templatesRoot, "auth", "login.html")})
	case "register":
		files = append(files, struct {
			name string
			path string
		}{name: "auth/register.html", path: filepath.Join(templatesRoot, "auth", "register.html")})
	default:
		t.Fatalf("unsupported auth page template set: %s", page)
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

func TestAuthHandler_Login_Success(t *testing.T) {
	svc := &mockService{
		loginResp: &TokenResponse{Token: "tok-123", ExpiresAt: 1700000000},
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	body := `{"email":"alice@example.com","password":"secret1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+req.Host)
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

	// Verify auth cookie is set on successful login.
	cookies := w.Result().Cookies()
	var authCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == AccessTokenCookieName {
			authCookie = cookie
			break
		}
	}
	if authCookie == nil {
		t.Fatalf("expected %s cookie to be set", AccessTokenCookieName)
	}
	if authCookie.Value != "tok-123" {
		t.Errorf("cookie value = %q; want %q", authCookie.Value, "tok-123")
	}
	if !authCookie.HttpOnly {
		t.Error("expected auth cookie to be HttpOnly")
	}
	if authCookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie SameSite = %v; want %v", authCookie.SameSite, http.SameSiteStrictMode)
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
	req.Header.Set("Origin", "http://"+req.Host)
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
	req.Header.Set("Origin", "http://"+req.Host)
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
			Username:  "Alice",
			Email:     "alice@example.com",
		},
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	body := `{"username":"Alice","email":"alice@example.com","password":"secret1234","confirm_password":"secret1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+req.Host)
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
			Username  string `json:"username"`
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
	if resp.Data.Username != "Alice" {
		t.Errorf("expected data.username = 'Alice', got %q", resp.Data.Username)
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
	body := `{"username":"","email":"","password":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+req.Host)
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

func TestAuthHandler_Register_ConfirmPasswordMismatch(t *testing.T) {
	svc := &mockService{}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	body := `{"username":"Alice","email":"alice@example.com","password":"secret1234","confirm_password":"different"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+req.Host)
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
		t.Errorf("expected validation error message, got %q", resp.Message)
	}
	if resp.Errors["confirm_password"] != "confirm_password does not match password" {
		t.Errorf("expected confirm_password mismatch error, got %q", resp.Errors["confirm_password"])
	}
}

func TestAuthHandler_Register_ConfirmPasswordRequired(t *testing.T) {
	svc := &mockService{
		registerRes: &domain.User{
			BaseModel: domain.BaseModel{ID: 2},
			Username:  "Bob",
			Email:     "bob@example.com",
		},
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	// Missing confirm_password should be rejected.
	body := `{"username":"Bob","email":"bob@example.com","password":"secret1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+req.Host)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d; body: %s", w.Code, w.Body.String())
	}

	var resp pkg.ValidationErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Code != http.StatusBadRequest {
		t.Errorf("expected code 400, got %d", resp.Code)
	}
}

func TestAuthHandler_Register_ConfirmPasswordMatch(t *testing.T) {
	svc := &mockService{
		registerRes: &domain.User{
			BaseModel: domain.BaseModel{ID: 3},
			Username:  "Carol",
			Email:     "carol@example.com",
		},
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	// Matching confirm_password should proceed normally
	body := `{"username":"Carol","email":"carol@example.com","password":"secret1234","confirm_password":"secret1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+req.Host)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_Register_ServiceError(t *testing.T) {
	svc := &mockService{
		registerErr: domain.NewAppError(domain.CodeAlreadyExists, "email already exists", nil),
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	body := `{"username":"Alice","email":"alice@example.com","password":"secret1234","confirm_password":"secret1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+req.Host)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Same-origin browser request tests
// ---------------------------------------------------------------------------

func TestIsSameOriginBrowserRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name    string
		host    string
		origin  string
		referer string
		want    bool
	}{
		{"no headers", "example.com", "", "", false},
		{"matching origin", "example.com", "http://example.com", "", true},
		{"matching origin with port", "example.com:8080", "http://example.com:8080", "", true},
		{"scheme mismatch", "example.com", "https://example.com", "", false},
		{"non-matching origin", "example.com", "http://evil.com", "", false},
		{"matching referer", "example.com", "", "http://example.com/some/page", true},
		{"non-matching referer", "example.com", "", "http://evil.com/page", false},
		{"origin null", "example.com", "null", "", false},
		{"origin takes precedence over referer", "example.com", "http://example.com", "http://evil.com/page", true},
		{"malformed origin - no scheme", "example.com", "example.com", "", false},
		{"empty host", "", "http://example.com", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			c.Request.Host = tt.host
			if tt.origin != "" {
				c.Request.Header.Set("Origin", tt.origin)
			}
			if tt.referer != "" {
				c.Request.Header.Set("Referer", tt.referer)
			}

			got := isSameOriginBrowserRequest(c)
			if got != tt.want {
				t.Errorf("isSameOriginBrowserRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthHandler_Login_CrossOriginForbidden(t *testing.T) {
	svc := &mockService{
		loginResp: &TokenResponse{Token: "tok-123", ExpiresAt: 1700000000},
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	body := `{"email":"alice@example.com","password":"secret1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://evil.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_Login_SameOriginAllowed(t *testing.T) {
	svc := &mockService{
		loginResp: &TokenResponse{Token: "tok-123", ExpiresAt: 1700000000},
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	body := `{"email":"alice@example.com","password":"secret1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_Login_NoBrowserHeadersAllowed(t *testing.T) {
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
		t.Fatalf("expected status 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_Register_CrossOriginForbidden(t *testing.T) {
	svc := &mockService{
		registerRes: &domain.User{
			BaseModel: domain.BaseModel{ID: 1},
			Username:  "Alice",
			Email:     "alice@example.com",
		},
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	body := `{"username":"Alice","email":"alice@example.com","password":"secret1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://evil.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_Register_NoBrowserHeadersAllowed(t *testing.T) {
	svc := &mockService{
		registerRes: &domain.User{
			BaseModel: domain.BaseModel{ID: 1},
			Username:  "Alice",
			Email:     "alice@example.com",
		},
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	body := `{"username":"Alice","email":"alice@example.com","password":"secret1234","confirm_password":"secret1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// NewHandlerWithCookieSecure constructor tests
// ---------------------------------------------------------------------------

func TestNewHandlerWithCookieSecure(t *testing.T) {
	svc := &mockService{}

	h := NewHandlerWithCookieSecure(svc, true)
	if !h.forceSecureCookie {
		t.Error("expected forceSecureCookie to be true")
	}

	h2 := NewHandlerWithCookieSecure(svc, false)
	if h2.forceSecureCookie {
		t.Error("expected forceSecureCookie to be false")
	}
}

// ---------------------------------------------------------------------------
// issueAuthCookie tests
// ---------------------------------------------------------------------------

func TestIssueAuthCookie_SetsCookieAttributes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewHandler(&mockService{})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	h.issueAuthCookie(c, "jwt-token-abc", 0)

	cookies := w.Result().Cookies()
	var got *http.Cookie
	for _, ck := range cookies {
		if ck.Name == AccessTokenCookieName {
			got = ck
			break
		}
	}
	if got == nil {
		t.Fatalf("expected %s cookie to be set", AccessTokenCookieName)
	}
	if got.Value != "jwt-token-abc" {
		t.Errorf("cookie value = %q; want %q", got.Value, "jwt-token-abc")
	}
	if !got.HttpOnly {
		t.Error("expected HttpOnly")
	}
	if got.Path != "/" {
		t.Errorf("cookie path = %q; want %q", got.Path, "/")
	}
	if got.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie SameSite = %v; want %v", got.SameSite, http.SameSiteStrictMode)
	}
}

// ---------------------------------------------------------------------------
// shouldSetSecureCookie / isHTTPSRequest tests
// ---------------------------------------------------------------------------

func TestShouldSetSecureCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name              string
		forceSecureCookie bool
		xForwardedProto   string
		xForwardedSSL     string
		want              bool
	}{
		{"force secure true", true, "", "", true},
		{"force secure true overrides http", true, "http", "", true},
		{"no force, plain http", false, "", "", false},
		{"no force, https via proto", false, "https", "", true},
		{"no force, https via forwarded ssl", false, "", "on", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &AuthHandler{forceSecureCookie: tt.forceSecureCookie}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.xForwardedProto != "" {
				c.Request.Header.Set("X-Forwarded-Proto", tt.xForwardedProto)
			}
			if tt.xForwardedSSL != "" {
				c.Request.Header.Set("X-Forwarded-Ssl", tt.xForwardedSSL)
			}
			if got := h.shouldSetSecureCookie(c); got != tt.want {
				t.Errorf("shouldSetSecureCookie() = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestIsHTTPSRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		xForwardedProto string
		xForwardedSSL   string
		want            bool
	}{
		{"no proxy header", "", "", false},
		{"http", "http", "", false},
		{"https", "https", "", true},
		{"HTTPS uppercase", "HTTPS", "", true},
		{"comma separated proto first https", "https,http", "", true},
		{"comma separated proto first http", "http, https", "", false},
		{"forwarded ssl on", "", "on", true},
		{"forwarded ssl uppercase", "", "ON", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.xForwardedProto != "" {
				c.Request.Header.Set("X-Forwarded-Proto", tt.xForwardedProto)
			}
			if tt.xForwardedSSL != "" {
				c.Request.Header.Set("X-Forwarded-Ssl", tt.xForwardedSSL)
			}
			if got := isHTTPSRequest(c); got != tt.want {
				t.Errorf("isHTTPSRequest() = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestLogin_CookieSecure_ViaForceSecureCookie(t *testing.T) {
	svc := &mockService{
		loginResp: &TokenResponse{Token: "tok-secure", ExpiresAt: 1700000000},
	}
	h := NewHandlerWithCookieSecure(svc, true)
	r := setupAuthRouter(h)

	body := `{"email":"alice@example.com","password":"secret1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+req.Host)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	cookies := w.Result().Cookies()
	var authCookie *http.Cookie
	for _, ck := range cookies {
		if ck.Name == AccessTokenCookieName {
			authCookie = ck
			break
		}
	}
	if authCookie == nil {
		t.Fatal("expected auth cookie to be set")
	}
	if !authCookie.Secure {
		t.Error("expected Secure flag on auth cookie when forceSecureCookie=true")
	}
}

// ---------------------------------------------------------------------------
// Logout handler tests
// ---------------------------------------------------------------------------

func TestAuthHandler_Logout_Success_WithCookie(t *testing.T) {
	svc := &mockService{}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: "tok-123"})
	req.Header.Set("Origin", "http://"+req.Host)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", w.Code, w.Body.String())
	}

	// Verify the cookie is cleared.
	cookies := w.Result().Cookies()
	var authCookie *http.Cookie
	for _, ck := range cookies {
		if ck.Name == AccessTokenCookieName {
			authCookie = ck
			break
		}
	}
	if authCookie == nil {
		t.Fatal("expected auth cookie to be set (cleared)")
	}
	if authCookie.MaxAge != -1 {
		t.Errorf("expected MaxAge=-1, got %d", authCookie.MaxAge)
	}
}

func TestAuthHandler_Logout_Success_WithBearer(t *testing.T) {
	svc := &mockService{}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer tok-bearer-123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_Logout_NoToken(t *testing.T) {
	svc := &mockService{}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", w.Code)
	}
}

func TestAuthHandler_Logout_CookieCrossOriginForbidden(t *testing.T) {
	svc := &mockService{}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: "tok-123"})
	req.Header.Set("Origin", "http://evil.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_Logout_ServiceError(t *testing.T) {
	svc := &mockService{logoutErr: domain.ErrUnauthorized}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer tok-bad")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// RefreshToken handler tests
// ---------------------------------------------------------------------------

func TestAuthHandler_RefreshToken_Success_WithCookie(t *testing.T) {
	svc := &mockService{
		refreshResp: &TokenResponse{Token: "new-tok", ExpiresAt: 1800000000},
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: "old-tok"})
	req.Header.Set("Origin", "http://"+req.Host)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Token     string `json:"token"`
			ExpiresAt int64  `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp.Data.Token != "new-tok" {
		t.Errorf("expected token 'new-tok', got %q", resp.Data.Token)
	}
	if resp.Data.ExpiresAt != 1800000000 {
		t.Errorf("expected expires_at 1800000000, got %d", resp.Data.ExpiresAt)
	}

	// Verify new cookie is set.
	cookies := w.Result().Cookies()
	var authCookie *http.Cookie
	for _, ck := range cookies {
		if ck.Name == AccessTokenCookieName {
			authCookie = ck
			break
		}
	}
	if authCookie == nil {
		t.Fatal("expected auth cookie to be set")
	}
	if authCookie.Value != "new-tok" {
		t.Errorf("cookie value = %q; want %q", authCookie.Value, "new-tok")
	}
}

func TestAuthHandler_RefreshToken_Success_WithBearer(t *testing.T) {
	svc := &mockService{
		refreshResp: &TokenResponse{Token: "new-tok", ExpiresAt: 1800000000},
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer old-tok")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_RefreshToken_NoToken(t *testing.T) {
	svc := &mockService{}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", w.Code)
	}
}

func TestAuthHandler_RefreshToken_CookieCrossOriginForbidden(t *testing.T) {
	svc := &mockService{}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: "tok-123"})
	req.Header.Set("Origin", "http://evil.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_RefreshToken_ServiceError(t *testing.T) {
	svc := &mockService{refreshErr: domain.ErrUnauthorized}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer expired-tok")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Token extraction helper tests
// ---------------------------------------------------------------------------

func TestExtractBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		header string
		want   string
		ok     bool
	}{
		{"valid bearer", "Bearer tok-abc", "tok-abc", true},
		{"empty header", "", "", false},
		{"no bearer prefix", "tok-abc", "", false},
		{"bearer only", "Bearer ", "", false},
		{"bearer case insensitive", "bearer tok-abc", "tok-abc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				c.Request.Header.Set("Authorization", tt.header)
			}
			got, ok := extractBearerToken(c)
			if ok != tt.ok {
				t.Errorf("ok = %v; want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("token = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestExtractAccessToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		authHeader string
		cookie     string
		wantToken  string
		wantBearer bool
		wantOK     bool
	}{
		{"bearer token", "Bearer tok-abc", "", "tok-abc", true, true},
		{"cookie token", "", "cookie-tok", "cookie-tok", false, true},
		{"bearer takes precedence", "Bearer bearer-tok", "cookie-tok", "bearer-tok", true, true},
		{"no token", "", "", "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.authHeader != "" {
				c.Request.Header.Set("Authorization", tt.authHeader)
			}
			if tt.cookie != "" {
				c.Request.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: tt.cookie})
			}
			token, fromBearer, ok := extractAccessToken(c)
			if ok != tt.wantOK {
				t.Errorf("ok = %v; want %v", ok, tt.wantOK)
			}
			if fromBearer != tt.wantBearer {
				t.Errorf("fromBearer = %v; want %v", fromBearer, tt.wantBearer)
			}
			if token != tt.wantToken {
				t.Errorf("token = %q; want %q", token, tt.wantToken)
			}
		})
	}
}

func TestClearAuthCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewHandler(&mockService{})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	h.clearAuthCookie(c)

	cookies := w.Result().Cookies()
	var got *http.Cookie
	for _, ck := range cookies {
		if ck.Name == AccessTokenCookieName {
			got = ck
			break
		}
	}
	if got == nil {
		t.Fatalf("expected %s cookie to be set", AccessTokenCookieName)
	}
	if got.Value != "" {
		t.Errorf("cookie value = %q; want empty", got.Value)
	}
	if got.MaxAge != -1 {
		t.Errorf("cookie MaxAge = %d; want -1", got.MaxAge)
	}
	if !got.HttpOnly {
		t.Error("expected HttpOnly")
	}
	if got.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie SameSite = %v; want %v", got.SameSite, http.SameSiteStrictMode)
	}
}

func TestLoginPage_ReturnsOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(&mockService{})
	r := setupAuthPageRouter(t, h, "login")

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("LoginPage status = %d; want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, `action="/api/v1/auth/login"`) {
		t.Errorf("LoginPage body missing login action, got %q", body)
	}
	if !strings.Contains(body, `hx-post="/api/v1/auth/login"`) {
		t.Errorf("LoginPage body missing login hx-post, got %q", body)
	}
	if !strings.Contains(body, `name="_csrf_token"`) {
		t.Errorf("LoginPage body missing CSRF hidden field, got %q", body)
	}
}

func TestRegisterPage_ReturnsOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(&mockService{})
	r := setupAuthPageRouter(t, h, "register")

	req := httptest.NewRequest(http.MethodGet, "/register", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("RegisterPage status = %d; want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, `action="/api/v1/auth/register"`) {
		t.Errorf("RegisterPage body missing register action, got %q", body)
	}
	if !strings.Contains(body, `hx-post="/api/v1/auth/register"`) {
		t.Errorf("RegisterPage body missing register hx-post, got %q", body)
	}
	if !strings.Contains(body, `name="_csrf_token"`) {
		t.Errorf("RegisterPage body missing CSRF hidden field, got %q", body)
	}
	if !strings.Contains(body, `name="confirm_password"`) {
		t.Errorf("RegisterPage body missing confirm_password input, got %q", body)
	}
	if !strings.Contains(body, `normalized.confirmPassword = normalized.confirm_password`) {
		t.Errorf("RegisterPage body missing confirm_password error normalization, got %q", body)
	}
}

// ---------------------------------------------------------------------------
// Same-origin Referer fallback test
// ---------------------------------------------------------------------------

func TestAuthHandler_Login_SameOriginRefererAllowed(t *testing.T) {
	svc := &mockService{
		loginResp: &TokenResponse{Token: "tok-123", ExpiresAt: 1700000000},
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	body := `{"email":"alice@example.com","password":"secret1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No Origin header — falls back to Referer.
	req.Header.Set("Referer", "http://"+req.Host+"/login")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Bearer token bypasses cross-site origin check (Logout / Refresh)
// ---------------------------------------------------------------------------

func TestAuthHandler_Logout_BearerIgnoresCrossSiteOrigin(t *testing.T) {
	svc := &mockService{}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer valid-token-123")
	req.Header.Set("Origin", "http://evil.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_Logout_BearerWithoutOriginAllowed(t *testing.T) {
	svc := &mockService{}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer valid-token-123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_RefreshToken_BearerIgnoresCrossSiteOrigin(t *testing.T) {
	svc := &mockService{
		refreshResp: &TokenResponse{Token: "new-tok", ExpiresAt: 1800000000},
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer old-token-123")
	req.Header.Set("Origin", "http://evil.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_RefreshToken_BearerWithoutOriginAllowed(t *testing.T) {
	svc := &mockService{
		refreshResp: &TokenResponse{Token: "new-tok", ExpiresAt: 1800000000},
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer old-token-123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Internal service error tests (CodeInternal → 500)
// ---------------------------------------------------------------------------

func TestAuthHandler_Logout_InternalServiceError(t *testing.T) {
	svc := &mockService{
		logoutErr: domain.NewAppError(domain.CodeInternal, "revoke failed", nil),
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer valid-token-123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_RefreshToken_InternalServiceError(t *testing.T) {
	svc := &mockService{
		refreshErr: domain.NewAppError(domain.CodeInternal, "refresh failed", nil),
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer old-token-123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Secure cookie detection via X-Forwarded-Proto
// ---------------------------------------------------------------------------

func TestAuthHandler_Login_SetsSecureCookieWhenForwardedHTTPS(t *testing.T) {
	svc := &mockService{
		loginResp: &TokenResponse{Token: "tok-123", ExpiresAt: 1700000000},
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	body := `{"email":"alice@example.com","password":"secret1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Origin", "https://"+req.Host)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var authCookie *http.Cookie
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == AccessTokenCookieName {
			authCookie = cookie
			break
		}
	}
	if authCookie == nil {
		t.Fatalf("expected %s cookie to be set", AccessTokenCookieName)
	}
	if !authCookie.Secure {
		t.Fatal("expected auth cookie to be Secure when X-Forwarded-Proto=https")
	}
}

func TestAuthHandler_Login_SetsSecureCookieWhenForwardedSSLOn(t *testing.T) {
	svc := &mockService{
		loginResp: &TokenResponse{Token: "tok-123", ExpiresAt: 1700000000},
	}
	h := NewHandler(svc)
	r := setupAuthRouter(h)

	body := `{"email":"alice@example.com","password":"secret1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Ssl", "on")
	req.Header.Set("Origin", "https://"+req.Host)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var authCookie *http.Cookie
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == AccessTokenCookieName {
			authCookie = cookie
			break
		}
	}
	if authCookie == nil {
		t.Fatalf("expected %s cookie to be set", AccessTokenCookieName)
	}
	if !authCookie.Secure {
		t.Fatal("expected auth cookie to be Secure when X-Forwarded-Ssl=on")
	}
}
