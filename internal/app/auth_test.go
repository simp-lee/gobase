package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/simp-lee/ginx"
	"github.com/simp-lee/jwt"
	"github.com/simp-lee/logger"
	"golang.org/x/crypto/bcrypt"

	"github.com/simp-lee/gobase/internal/config"
	"github.com/simp-lee/gobase/internal/domain"
	"github.com/simp-lee/gobase/internal/module/auth"
	usermodule "github.com/simp-lee/gobase/internal/module/user"
)

type stubJWTService struct {
	validateTokenFn func(token string) (*jwt.Token, error)
}

type loginFlowUserRepo struct {
	user *domain.User
}

func (r *loginFlowUserRepo) Create(context.Context, *domain.User) error { return nil }

func (r *loginFlowUserRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	if r.user == nil || r.user.Email != email {
		return nil, domain.ErrNotFound
	}
	return r.user, nil
}

func (r *loginFlowUserRepo) GetByID(context.Context, uint) (*domain.User, error) {
	return nil, domain.ErrNotFound
}

func (r *loginFlowUserRepo) List(context.Context, domain.PageRequest) (*domain.PageResult[domain.User], error) {
	return nil, nil
}

func (r *loginFlowUserRepo) Update(context.Context, *domain.User) error { return nil }

func (r *loginFlowUserRepo) Delete(context.Context, uint) error { return nil }

func (s *stubJWTService) GenerateToken(string, []string, time.Duration) (string, error) {
	return "", nil
}
func (s *stubJWTService) ValidateToken(token string) (*jwt.Token, error) {
	if s.validateTokenFn != nil {
		return s.validateTokenFn(token)
	}
	return nil, nil
}
func (s *stubJWTService) ValidateAndParse(string) (*jwt.Token, error) { return nil, nil }
func (s *stubJWTService) RefreshToken(string) (string, error)         { return "", nil }
func (s *stubJWTService) RefreshTokenExtend(string, time.Duration) (string, error) {
	return "", nil
}
func (s *stubJWTService) RevokeToken(string) error              { return nil }
func (s *stubJWTService) IsTokenRevoked(string) bool            { return false }
func (s *stubJWTService) ParseToken(string) (*jwt.Token, error) { return nil, nil }
func (s *stubJWTService) RevokeAllUserTokens(string) error      { return nil }
func (s *stubJWTService) Close()                                {}

func TestPromoteTokenFromCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name              string
		authorization     string
		accessCookie      string
		wantPromoted      bool
		wantAuthorization string
	}{
		{
			name:              "cookie token promotes to bearer header",
			accessCookie:      "cookie-token",
			wantPromoted:      true,
			wantAuthorization: "Bearer cookie-token",
		},
		{
			name:              "existing bearer header is not overwritten",
			authorization:     "Bearer existing-header-token",
			accessCookie:      "cookie-token",
			wantPromoted:      false,
			wantAuthorization: "Bearer existing-header-token",
		},
		{
			name:              "no cookie does not promote and continues",
			wantPromoted:      false,
			wantAuthorization: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			req := httptest.NewRequest(http.MethodGet, "/resource", nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			if tt.accessCookie != "" {
				req.AddCookie(&http.Cookie{Name: auth.AccessTokenCookieName, Value: tt.accessCookie})
			}
			c.Request = req

			gotPromoted := promoteTokenFromCookie(c)
			if gotPromoted != tt.wantPromoted {
				t.Fatalf("promoted=%v, want=%v", gotPromoted, tt.wantPromoted)
			}

			if gotAuthorization := c.GetHeader("Authorization"); gotAuthorization != tt.wantAuthorization {
				t.Fatalf("authorization=%q, want=%q", gotAuthorization, tt.wantAuthorization)
			}
		})
	}
}

func TestOptionalBearerAuthorizationHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name              string
		authorization     string
		accessCookie      string
		wantAuthorization string
	}{
		{
			name:              "promotes cookie token and allows request",
			accessCookie:      "cookie-token",
			wantAuthorization: "Bearer cookie-token",
		},
		{
			name:              "keeps existing bearer and allows request",
			authorization:     "Bearer existing-token",
			accessCookie:      "cookie-token",
			wantAuthorization: "Bearer existing-token",
		},
		{
			name:              "no token still allows request",
			wantAuthorization: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(optionalBearerAuthorizationHeader())
			router.GET("/resource", func(c *gin.Context) {
				if gotAuthorization := c.GetHeader("Authorization"); gotAuthorization != tt.wantAuthorization {
					t.Fatalf("authorization=%q, want=%q", gotAuthorization, tt.wantAuthorization)
				}
				c.Status(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodGet, "/resource", nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			if tt.accessCookie != "" {
				req.AddCookie(&http.Cookie{Name: auth.AccessTokenCookieName, Value: tt.accessCookie})
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusNoContent {
				t.Fatalf("status=%d, want=%d", w.Code, http.StatusNoContent)
			}
		})
	}
}

func TestRequireBearerAuthorizationHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	csrfSecret := "test-csrf-secret"
	validCSRFToken := buildSignedCSRFToken("nonce-value", csrfSecret)

	tests := []struct {
		name           string
		method         string
		headerAuth     string
		accessCookie   string
		csrfCookie     string
		csrfHeader     string
		wantStatusCode int
	}{
		{
			name:           "missing bearer and no promotable cookie returns unauthorized",
			method:         http.MethodGet,
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name:           "promoted write request without csrf token returns forbidden",
			method:         http.MethodPost,
			accessCookie:   "access-token-from-cookie",
			wantStatusCode: http.StatusForbidden,
		},
		{
			name:           "promoted write request with invalid csrf pair returns forbidden",
			method:         http.MethodPost,
			accessCookie:   "access-token-from-cookie",
			csrfCookie:     validCSRFToken,
			csrfHeader:     "invalid-request-token",
			wantStatusCode: http.StatusForbidden,
		},
		{
			name:           "promoted write request with valid csrf pair passes",
			method:         http.MethodPost,
			accessCookie:   "access-token-from-cookie",
			csrfCookie:     validCSRFToken,
			csrfHeader:     validCSRFToken,
			wantStatusCode: http.StatusNoContent,
		},
		{
			name:           "non-promoted write request with bearer header passes without csrf",
			method:         http.MethodPost,
			headerAuth:     "Bearer header-token",
			wantStatusCode: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(requireBearerAuthorizationHeader(csrfSecret))
			router.Handle(tt.method, "/resource", func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})

			req := httptest.NewRequest(tt.method, "/resource", nil)
			if tt.headerAuth != "" {
				req.Header.Set("Authorization", tt.headerAuth)
			}
			if tt.csrfHeader != "" {
				req.Header.Set("X-CSRF-Token", tt.csrfHeader)
			}
			if tt.accessCookie != "" {
				req.AddCookie(&http.Cookie{Name: auth.AccessTokenCookieName, Value: tt.accessCookie})
			}
			if tt.csrfCookie != "" {
				req.AddCookie(&http.Cookie{Name: "_csrf_token", Value: tt.csrfCookie})
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatusCode {
				t.Fatalf("status=%d, want=%d", w.Code, tt.wantStatusCode)
			}
		})
	}
}

func buildSignedCSRFToken(nonce, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(nonce))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return nonce + "." + sig
}

func TestBridgeAuthContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		setup          func(c *gin.Context)
		wantUserID     uint
		wantUserIDSet  bool
		wantRole       string
		wantRoleSet    bool
		wantStatusCode int
	}{
		{
			name:           "missing auth context continues without setting user fields",
			setup:          func(c *gin.Context) {},
			wantStatusCode: http.StatusNoContent,
		},
		{
			name: "valid user id and multiple roles sets userID and first role",
			setup: func(c *gin.Context) {
				ginx.SetUserID(c, "42")
				ginx.SetUserRoles(c, []string{"admin", "user"})
			},
			wantUserID:     42,
			wantUserIDSet:  true,
			wantRole:       "admin",
			wantRoleSet:    true,
			wantStatusCode: http.StatusNoContent,
		},
		{
			name: "invalid user id does not set userID and still continues",
			setup: func(c *gin.Context) {
				ginx.SetUserID(c, "not-a-number")
				ginx.SetUserRoles(c, []string{"user"})
			},
			wantUserIDSet:  false,
			wantRole:       "user",
			wantRoleSet:    true,
			wantStatusCode: http.StatusNoContent,
		},
		{
			name: "too-large user id for current uint does not set userID",
			setup: func(c *gin.Context) {
				if strconv.IntSize == 32 {
					ginx.SetUserID(c, "4294967296")
					return
				}
				ginx.SetUserID(c, "18446744073709551616")
			},
			wantUserIDSet:  false,
			wantRoleSet:    false,
			wantStatusCode: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				tt.setup(c)
				c.Next()
			})
			router.Use(bridgeAuthContext())
			router.GET("/resource", func(c *gin.Context) {
				_, hasUserID := c.Get("userID")
				_, hasRole := c.Get("role")

				if hasUserID != tt.wantUserIDSet {
					t.Fatalf("hasUserID=%v, want=%v", hasUserID, tt.wantUserIDSet)
				}
				if tt.wantUserIDSet {
					if gotUserID := c.GetUint("userID"); gotUserID != tt.wantUserID {
						t.Fatalf("userID=%d, want=%d", gotUserID, tt.wantUserID)
					}
				}

				if hasRole != tt.wantRoleSet {
					t.Fatalf("hasRole=%v, want=%v", hasRole, tt.wantRoleSet)
				}
				if tt.wantRoleSet {
					if gotRole := c.GetString("role"); gotRole != tt.wantRole {
						t.Fatalf("role=%q, want=%q", gotRole, tt.wantRole)
					}
				}

				c.Status(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodGet, "/resource", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatusCode {
				t.Fatalf("status=%d, want=%d", w.Code, tt.wantStatusCode)
			}
		})
	}
}

func TestOptionalAuthContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		authHeader     string
		accessCookie   string
		jwtSvc         jwt.Service
		wantUserID     uint
		wantUserIDSet  bool
		wantRole       string
		wantRoleSet    bool
		wantStatusCode int
	}{
		{
			name:       "valid bearer token sets ginx and app auth context",
			authHeader: "Bearer valid-token",
			jwtSvc: &stubJWTService{validateTokenFn: func(token string) (*jwt.Token, error) {
				if token != "valid-token" {
					return nil, errors.New("unexpected token")
				}
				return &jwt.Token{UserID: "42", Roles: []string{"admin", "user"}}, nil
			}},
			wantUserID:     42,
			wantUserIDSet:  true,
			wantRole:       "admin",
			wantRoleSet:    true,
			wantStatusCode: http.StatusNoContent,
		},
		{
			name:           "missing token continues without auth context",
			jwtSvc:         &stubJWTService{},
			wantUserIDSet:  false,
			wantRoleSet:    false,
			wantStatusCode: http.StatusNoContent,
		},
		{
			name:           "invalid token continues without auth context",
			authHeader:     "Bearer invalid-token",
			jwtSvc:         &stubJWTService{validateTokenFn: func(string) (*jwt.Token, error) { return nil, errors.New("invalid") }},
			wantUserIDSet:  false,
			wantRoleSet:    false,
			wantStatusCode: http.StatusNoContent,
		},
		{
			name:         "cookie token promotion validates and sets auth context",
			accessCookie: "cookie-token",
			jwtSvc: &stubJWTService{validateTokenFn: func(token string) (*jwt.Token, error) {
				if token != "cookie-token" {
					return nil, errors.New("unexpected token")
				}
				return &jwt.Token{UserID: "7", Roles: []string{"user"}}, nil
			}},
			wantUserID:     7,
			wantUserIDSet:  true,
			wantRole:       "user",
			wantRoleSet:    true,
			wantStatusCode: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(optionalAuthContext(tt.jwtSvc))
			router.GET("/resource", func(c *gin.Context) {
				_, hasUserID := c.Get("userID")
				_, hasRole := c.Get("role")

				if hasUserID != tt.wantUserIDSet {
					t.Fatalf("hasUserID=%v, want=%v", hasUserID, tt.wantUserIDSet)
				}
				if tt.wantUserIDSet {
					if gotUserID := c.GetUint("userID"); gotUserID != tt.wantUserID {
						t.Fatalf("userID=%d, want=%d", gotUserID, tt.wantUserID)
					}
					if got, ok := ginx.GetUserID(c); !ok || got != strconv.FormatUint(uint64(tt.wantUserID), 10) {
						t.Fatalf("ginx userID=(%q,%v), want=(%d,true)", got, ok, tt.wantUserID)
					}
				}

				if hasRole != tt.wantRoleSet {
					t.Fatalf("hasRole=%v, want=%v", hasRole, tt.wantRoleSet)
				}
				if tt.wantRoleSet {
					if gotRole := c.GetString("role"); gotRole != tt.wantRole {
						t.Fatalf("role=%q, want=%q", gotRole, tt.wantRole)
					}
					if got, ok := ginx.GetUserRoles(c); !ok || len(got) == 0 || got[0] != tt.wantRole {
						t.Fatalf("ginx roles=(%v,%v), want first role %q", got, ok, tt.wantRole)
					}
				}

				c.Status(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodGet, "/resource", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			if tt.accessCookie != "" {
				req.AddCookie(&http.Cookie{Name: auth.AccessTokenCookieName, Value: tt.accessCookie})
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatusCode {
				t.Fatalf("status=%d, want=%d", w.Code, tt.wantStatusCode)
			}
		})
	}
}

func TestSetupAuth_BridgeAuthContextScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Auth: config.AuthConfig{
			Enabled:     true,
			JWTSecret:   "test-secret-key-must-be-at-least-32-chars-long!",
			TokenExpiry: "1h",
			PublicPaths: []string{"/api/public"},
		},
	}

	chain := ginx.NewChain()
	_, jwtSvc, _, err := setupAuth(cfg, nil, nil, chain, logger.Default(), "")
	if err != nil {
		t.Fatalf("setupAuth() error = %v", err)
	}
	t.Cleanup(func() {
		if jwtSvc != nil {
			jwtSvc.Close()
		}
	})

	token, err := jwtSvc.GenerateToken("42", []string{"admin"}, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	t.Run("api public path does not bridge auth context", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			ginx.SetUserID(c, "88")
			ginx.SetUserRoles(c, []string{"seed-role"})
			c.Next()
		})
		router.Use(chain.Build())
		router.GET("/api/public", func(c *gin.Context) {
			if _, ok := c.Get("userID"); ok {
				t.Fatal("userID should not be bridged on API public path")
			}
			if _, ok := c.Get("role"); ok {
				t.Fatal("role should not be bridged on API public path")
			}
			c.Status(http.StatusNoContent)
		})

		req := httptest.NewRequest(http.MethodGet, "/api/public", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("status=%d, want=%d", w.Code, http.StatusNoContent)
		}
	})

	t.Run("api protected path authenticates but does not bridge app context", func(t *testing.T) {
		router := gin.New()
		router.Use(chain.Build())
		router.GET("/api/private", func(c *gin.Context) {
			if got, ok := ginx.GetUserID(c); !ok || got != "42" {
				t.Fatalf("ginx userID=(%q,%v), want=(%q,true)", got, ok, "42")
			}
			if _, ok := c.Get("userID"); ok {
				t.Fatal("userID should not be bridged on API protected path")
			}
			if _, ok := c.Get("role"); ok {
				t.Fatal("role should not be bridged on API protected path")
			}
			c.Status(http.StatusNoContent)
		})

		req := httptest.NewRequest(http.MethodGet, "/api/private", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("status=%d, want=%d", w.Code, http.StatusNoContent)
		}
	})

	t.Run("non-api protected page bridges auth context", func(t *testing.T) {
		router := gin.New()
		router.Use(chain.Build())
		router.GET("/users", func(c *gin.Context) {
			if got, ok := ginx.GetUserID(c); !ok || got != "42" {
				t.Fatalf("ginx userID=(%q,%v), want=(%q,true)", got, ok, "42")
			}
			if got := c.GetUint("userID"); got != 42 {
				t.Fatalf("userID=%d, want=%d", got, 42)
			}
			if got := c.GetString("role"); got != "admin" {
				t.Fatalf("role=%q, want=%q", got, "admin")
			}
			c.Status(http.StatusNoContent)
		})

		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("status=%d, want=%d", w.Code, http.StatusNoContent)
		}
	})

	t.Run("static route remains unauthenticated", func(t *testing.T) {
		router := gin.New()
		router.Use(chain.Build())
		router.GET("/static/css/app.css", func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		})

		req := httptest.NewRequest(http.MethodGet, "/static/css/app.css", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code == http.StatusUnauthorized {
			t.Fatalf("status=%d, static route should not be blocked by auth middleware", w.Code)
		}
		if w.Code != http.StatusNoContent {
			t.Fatalf("status=%d, want=%d", w.Code, http.StatusNoContent)
		}
	})
}

func TestSetupAuth_ProtectedPageWriteGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const csrfSecret = "test-csrf-secret-for-setup-auth-guard"
	cfg := &config.Config{
		Auth: config.AuthConfig{
			Enabled:     true,
			JWTSecret:   "test-secret-key-must-be-at-least-32-chars-long!",
			TokenExpiry: "1h",
			PublicPaths: []string{"/api/public"},
		},
	}

	chain := ginx.NewChain()
	_, jwtSvc, _, err := setupAuth(cfg, nil, nil, chain, logger.Default(), csrfSecret)
	if err != nil {
		t.Fatalf("setupAuth() error = %v", err)
	}
	t.Cleanup(func() {
		if jwtSvc != nil {
			jwtSvc.Close()
		}
	})

	validAccessToken, err := jwtSvc.GenerateToken("42", []string{"admin"}, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	validCSRFToken := buildSignedCSRFToken("nonce-value", csrfSecret)

	tests := []struct {
		name           string
		setupRequest   func(req *http.Request)
		wantStatusCode int
	}{
		{
			name:           "no bearer token on protected page write returns unauthorized",
			setupRequest:   func(_ *http.Request) {},
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name: "promoted write without csrf token pair returns forbidden",
			setupRequest: func(req *http.Request) {
				req.AddCookie(&http.Cookie{Name: auth.AccessTokenCookieName, Value: validAccessToken})
			},
			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "promoted write with valid csrf token pair passes",
			setupRequest: func(req *http.Request) {
				req.AddCookie(&http.Cookie{Name: auth.AccessTokenCookieName, Value: validAccessToken})
				req.AddCookie(&http.Cookie{Name: "_csrf_token", Value: validCSRFToken})
				req.Header.Set("X-CSRF-Token", validCSRFToken)
			},
			wantStatusCode: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(chain.Build())
			router.POST("/users", func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodPost, "/users", nil)
			tt.setupRequest(req)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatusCode {
				t.Fatalf("status=%d, want=%d", w.Code, tt.wantStatusCode)
			}
		})
	}
}

func TestLoginTokenAdminRole_PropagatesToPageRoleGate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	jwtSvc, err := jwt.New("test-secret-key-must-be-at-least-32-chars-long!")
	if err != nil {
		t.Fatalf("jwt.New() error = %v", err)
	}
	t.Cleanup(func() {
		jwtSvc.Close()
	})

	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword() error = %v", err)
	}

	repo := &loginFlowUserRepo{user: &domain.User{
		BaseModel:    domain.BaseModel{ID: 7},
		Username:     "Admin",
		Email:        "admin@example.com",
		PasswordHash: string(hash),
		Role:         domain.RoleAdmin,
	}}

	authSvc := auth.NewService(jwtSvc, repo, time.Hour)
	loginResp, err := authSvc.Login(context.Background(), "admin@example.com", "password123")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	router := gin.New()
	router.SetHTMLTemplate(templateMust(`{{define "user/form.html"}}{{if .CanEditRole}}can-edit-role{{else}}no-role-edit{{end}}{{end}}`))
	router.Use(middlewareToHandler(ginx.Auth(jwtSvc)))
	router.Use(bridgeAuthContext())
	router.GET("/users/new", usermodule.NewUserPageHandler(nil).NewPage)

	req := httptest.NewRequest(http.MethodGet, "/users/new", nil)
	req.Header.Set("Authorization", "Bearer "+loginResp.Token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want=%d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); body != "can-edit-role" {
		t.Fatalf("body=%q, want=%q", body, "can-edit-role")
	}
}

func templateMust(raw string) *template.Template {
	tpl, err := template.New("").Parse(raw)
	if err != nil {
		panic(err)
	}
	return tpl
}
