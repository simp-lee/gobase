package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/simp-lee/ginx"
)

type authResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func setupAdminRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequireAdmin())
	r.GET("/admin", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return r
}

func TestRequireAdmin_NoRole(t *testing.T) {
	r := setupAdminRouter()

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["message"] == nil {
		t.Error("expected message in response")
	}
}

func TestRequireAdmin_NonAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Simulate JWT middleware setting role context
	r.Use(func(c *gin.Context) {
		c.Set("role", "user")
		c.Next()
	})
	r.Use(RequireAdmin())
	r.GET("/admin", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["message"] == nil {
		t.Error("expected message in response")
	}
}

func TestRequireAdmin_AdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("role", "admin")
		c.Next()
	})
	r.Use(RequireAdmin())
	r.GET("/admin", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %q", w.Body.String())
	}
}

func TestRequireAdmin_AdminCaseInsensitive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("role", "Admin")
		c.Next()
	})
	r.Use(RequireAdmin())
	r.GET("/admin", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireAdmin_GinxRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Simulate ginx setting roles via SetUserRoles (context key "ginx.user_roles")
	r.Use(func(c *gin.Context) {
		c.Set("ginx.user_roles", []string{"editor", "admin"})
		c.Next()
	})
	r.Use(RequireAdmin())
	r.GET("/admin", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireAdmin_RolesSlice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("roles", []string{"viewer"})
		c.Next()
	})
	r.Use(RequireAdmin())
	r.GET("/admin", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRequireAdmin_RoleFieldWhitespace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("role", " admin ")
		c.Next()
	})
	r.Use(RequireAdmin())
	r.GET("/admin", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireAdmin_RolesSliceCaseInsensitiveWhitespace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("roles", []string{"user", " AdMiN "})
		c.Next()
	})
	r.Use(RequireAdmin())
	r.GET("/admin", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireAdmin_GinxSetUserRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ginx.SetUserRoles(c, []string{"reader", " ADMIN "})
		c.Next()
	})
	r.Use(RequireAdmin())
	r.GET("/admin", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireAdmin_NoRole_ExactMessage(t *testing.T) {
	r := setupAdminRouter()

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp authResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Code != http.StatusForbidden {
		t.Errorf("code=%d; want %d", resp.Code, http.StatusForbidden)
	}
	if resp.Message != "admin access required" {
		t.Errorf("message=%q; want %q", resp.Message, "admin access required")
	}
}

func TestRequireAdmin_NonAdmin_ExactMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("roles", []string{"editor", "user"})
		c.Next()
	})
	r.Use(RequireAdmin())
	r.GET("/admin", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp authResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Code != http.StatusForbidden {
		t.Errorf("code=%d; want %d", resp.Code, http.StatusForbidden)
	}
	if resp.Message != "admin access required" {
		t.Errorf("message=%q; want %q", resp.Message, "admin access required")
	}
}
