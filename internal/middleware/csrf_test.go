package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

const testCSRFSecret = "test-secret-key-for-csrf"

func setupCSRFRouter() *gin.Engine {
	r := gin.New()
	r.Use(CSRF(testCSRFSecret))
	r.GET("/form", func(c *gin.Context) {
		token := GetCSRFToken(c)
		c.String(http.StatusOK, token)
	})
	r.POST("/form", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	r.PUT("/update", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	r.DELETE("/delete", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	r.PATCH("/patch", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return r
}

// getCSRFTokenFromGET performs a GET request and returns the token from the body
// and the cookie value.
func getCSRFTokenFromGET(t *testing.T, r *gin.Engine) (token string, cookie string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/form", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /form: expected 200, got %d", w.Code)
	}
	token = w.Body.String()
	for _, c := range w.Result().Cookies() {
		if c.Name == "_csrf_token" {
			cookie = c.Value
			break
		}
	}
	if cookie == "" {
		t.Fatal("expected _csrf_token cookie to be set")
	}
	return token, cookie
}

func TestCSRF_GET_SetsTokenCookie(t *testing.T) {
	r := setupCSRFRouter()
	token, cookie := getCSRFTokenFromGET(t, r)

	if token == "" {
		t.Error("expected non-empty token in response body")
	}
	if cookie != token {
		t.Errorf("cookie value %q != context token %q", cookie, token)
	}
	if !validToken(token, testCSRFSecret) {
		t.Error("generated token has invalid HMAC signature")
	}
}

func TestCSRF_GET_CookieAttributes(t *testing.T) {
	r := setupCSRFRouter()
	req := httptest.NewRequest(http.MethodGet, "/form", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var found *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "_csrf_token" {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("_csrf_token cookie not found")
	}
	if found.HttpOnly {
		t.Error("expected HttpOnly=false")
	}
	if found.Path != "/" {
		t.Errorf("expected Path=/, got %q", found.Path)
	}
	if found.SameSite != http.SameSiteStrictMode {
		t.Errorf("expected SameSite=Strict, got %v", found.SameSite)
	}
}

func TestCSRF_GET_ExistingValidCookie_NoNewCookie(t *testing.T) {
	r := setupCSRFRouter()
	_, cookie := getCSRFTokenFromGET(t, r)

	// Second request with the existing valid cookie.
	req := httptest.NewRequest(http.MethodGet, "/form", nil)
	req.AddCookie(&http.Cookie{Name: "_csrf_token", Value: cookie})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != cookie {
		t.Errorf("expected same token %q, got %q", cookie, body)
	}
	// Should not set a new cookie since the existing one is valid.
	for _, c := range w.Result().Cookies() {
		if c.Name == "_csrf_token" {
			t.Error("expected no new _csrf_token cookie when existing cookie is valid")
		}
	}
}

func TestCSRF_GET_InvalidCookie_RegeneratesToken(t *testing.T) {
	r := setupCSRFRouter()
	req := httptest.NewRequest(http.MethodGet, "/form", nil)
	req.AddCookie(&http.Cookie{Name: "_csrf_token", Value: "garbage"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var found bool
	for _, c := range w.Result().Cookies() {
		if c.Name == "_csrf_token" {
			found = true
			if !validToken(c.Value, testCSRFSecret) {
				t.Error("regenerated token has invalid signature")
			}
		}
	}
	if !found {
		t.Error("expected new _csrf_token cookie after invalid cookie")
	}
}

func TestCSRF_POST_ValidToken_FormField(t *testing.T) {
	r := setupCSRFRouter()
	_, cookie := getCSRFTokenFromGET(t, r)

	form := url.Values{}
	form.Set("_csrf_token", cookie)
	req := httptest.NewRequest(http.MethodPost, "/form", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "_csrf_token", Value: cookie})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestCSRF_POST_ValidToken_Header(t *testing.T) {
	r := setupCSRFRouter()
	_, cookie := getCSRFTokenFromGET(t, r)

	req := httptest.NewRequest(http.MethodPost, "/form", nil)
	req.Header.Set("X-CSRF-Token", cookie)
	req.AddCookie(&http.Cookie{Name: "_csrf_token", Value: cookie})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestCSRF_POST_MissingCookie_Returns403(t *testing.T) {
	r := setupCSRFRouter()

	form := url.Values{}
	form.Set("_csrf_token", "some-token")
	req := httptest.NewRequest(http.MethodPost, "/form", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestCSRF_POST_MissingToken_Returns403(t *testing.T) {
	r := setupCSRFRouter()
	_, cookie := getCSRFTokenFromGET(t, r)

	// POST without form field or header.
	req := httptest.NewRequest(http.MethodPost, "/form", nil)
	req.AddCookie(&http.Cookie{Name: "_csrf_token", Value: cookie})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestCSRF_POST_InvalidToken_Returns403(t *testing.T) {
	r := setupCSRFRouter()
	_, cookie := getCSRFTokenFromGET(t, r)

	form := url.Values{}
	form.Set("_csrf_token", "invalid-token")
	req := httptest.NewRequest(http.MethodPost, "/form", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "_csrf_token", Value: cookie})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestCSRF_POST_ForgedEqualInvalidTokens_Returns403(t *testing.T) {
	r := setupCSRFRouter()

	form := url.Values{}
	form.Set("_csrf_token", "forged")
	req := httptest.NewRequest(http.MethodPost, "/form", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "_csrf_token", Value: "forged"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestCSRF_POST_EqualTokensWithTamperedSignature_Returns403(t *testing.T) {
	r := setupCSRFRouter()
	valid := mustGenerateToken(testCSRFSecret)
	parts := strings.SplitN(valid, ".", 2)
	tampered := parts[0] + "." + signNonce(parts[0], "wrong-secret")

	form := url.Values{}
	form.Set("_csrf_token", tampered)
	req := httptest.NewRequest(http.MethodPost, "/form", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "_csrf_token", Value: tampered})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestCSRF_PUT_ValidToken(t *testing.T) {
	r := setupCSRFRouter()
	_, cookie := getCSRFTokenFromGET(t, r)

	req := httptest.NewRequest(http.MethodPut, "/update", nil)
	req.Header.Set("X-CSRF-Token", cookie)
	req.AddCookie(&http.Cookie{Name: "_csrf_token", Value: cookie})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestCSRF_DELETE_ValidToken(t *testing.T) {
	r := setupCSRFRouter()
	_, cookie := getCSRFTokenFromGET(t, r)

	req := httptest.NewRequest(http.MethodDelete, "/delete", nil)
	req.Header.Set("X-CSRF-Token", cookie)
	req.AddCookie(&http.Cookie{Name: "_csrf_token", Value: cookie})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestCSRF_PATCH_ValidToken(t *testing.T) {
	r := setupCSRFRouter()
	_, cookie := getCSRFTokenFromGET(t, r)

	req := httptest.NewRequest(http.MethodPatch, "/patch", nil)
	req.Header.Set("X-CSRF-Token", cookie)
	req.AddCookie(&http.Cookie{Name: "_csrf_token", Value: cookie})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGetCSRFToken_ReturnsEmpty_WhenNotSet(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if got := GetCSRFToken(c); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestGetCSRFToken_ReturnsToken_WhenSet(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("CSRFToken", "my-token")
	if got := GetCSRFToken(c); got != "my-token" {
		t.Errorf("expected my-token, got %q", got)
	}
}

func TestSetCSRFToken_SetsFromCookie(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.AddCookie(&http.Cookie{Name: "_csrf_token", Value: "token-from-cookie"})

	SetCSRFToken(c)

	if got := GetCSRFToken(c); got != "token-from-cookie" {
		t.Errorf("expected token-from-cookie, got %q", got)
	}
}

func TestSetCSRFToken_NoOverwrite_WhenAlreadySet(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.AddCookie(&http.Cookie{Name: "_csrf_token", Value: "cookie-token"})
	c.Set("CSRFToken", "existing-token")

	SetCSRFToken(c)

	if got := GetCSRFToken(c); got != "existing-token" {
		t.Errorf("expected existing-token (no overwrite), got %q", got)
	}
}

func TestSetCSRFToken_NoCookie_DoesNothing(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	SetCSRFToken(c)

	if got := GetCSRFToken(c); got != "" {
		t.Errorf("expected empty string when no cookie, got %q", got)
	}
}

func TestValidToken(t *testing.T) {
	tests := []struct {
		name   string
		token  string
		secret string
		want   bool
	}{
		{"valid token", mustGenerateToken(testCSRFSecret), testCSRFSecret, true},
		{"wrong secret", mustGenerateToken(testCSRFSecret), "wrong-secret", false},
		{"empty token", "", testCSRFSecret, false},
		{"no dot separator", "abcdef1234", testCSRFSecret, false},
		{"empty nonce", "." + signNonce("", testCSRFSecret), testCSRFSecret, false},
		{"empty signature", "abcdef.", testCSRFSecret, false},
		{"tampered nonce", "tampered." + signNonce("original", testCSRFSecret), testCSRFSecret, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validToken(tt.token, tt.secret); got != tt.want {
				t.Errorf("validToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTokensMatch(t *testing.T) {
	token := mustGenerateToken(testCSRFSecret)
	if !tokensMatch(token, token) {
		t.Error("identical tokens should match")
	}
	if tokensMatch(token, "different") {
		t.Error("different tokens should not match")
	}
}

// TestCSRF_APIRoute_ExemptWhenMiddlewareNotApplied verifies that when the CSRF
// middleware is NOT registered on a route group (e.g. /api/*), POST/PUT/DELETE
// requests go through without any CSRF check.
func TestCSRF_APIRoute_ExemptWhenMiddlewareNotApplied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Web routes: CSRF middleware applied.
	web := r.Group("/")
	web.Use(CSRF(testCSRFSecret))
	web.POST("/form", func(c *gin.Context) {
		c.String(http.StatusOK, "web ok")
	})

	// API routes: NO CSRF middleware — simulates how routes.go registers /api/*.
	api := r.Group("/api")
	api.POST("/users", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "api ok"})
	})
	api.PUT("/users/1", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "api ok"})
	})
	api.DELETE("/users/1", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "api ok"})
	})

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"POST /api/users without CSRF", http.MethodPost, "/api/users"},
		{"PUT /api/users/1 without CSRF", http.MethodPut, "/api/users/1"},
		{"DELETE /api/users/1 without CSRF", http.MethodDelete, "/api/users/1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No CSRF cookie or token provided — should still succeed.
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected 200, got %d; body: %s", w.Code, w.Body.String())
			}
		})
	}

	// Verify the web route still requires CSRF.
	t.Run("POST /form still requires CSRF", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/form", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", w.Code)
		}
	})
}

func mustGenerateToken(secret string) string {
	token, err := generateToken(secret)
	if err != nil {
		panic(err)
	}
	return token
}
