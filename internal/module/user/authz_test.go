package user

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/simp-lee/ginx"
)

func TestIsRequesterAdmin_GinxUserRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", func(c *gin.Context) {
		ginx.SetUserRoles(c, []string{"reader", " ADMIN "})
		if !isRequesterAdmin(c) {
			t.Fatalf("expected admin by ginx roles")
		}
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestIsRequesterAdmin_RoleField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", func(c *gin.Context) {
		c.Set("role", " admin ")
		if !isRequesterAdmin(c) {
			t.Fatalf("expected admin by role field")
		}
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestIsRequesterAdmin_GinxRolesPrecedence_DenyOverrideByRoleField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", func(c *gin.Context) {
		ginx.SetUserRoles(c, []string{"user"})
		c.Set("role", "admin")
		if isRequesterAdmin(c) {
			t.Fatalf("expected false when ginx roles are non-admin")
		}
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAdminFieldAuthzKey_InjectAndRetrieve(t *testing.T) {
	t.Run("authorized true", func(t *testing.T) {
		ctx := withAdminFieldAuthorized(context.Background(), true)
		if !isAdminFieldAuthorized(ctx) {
			t.Fatal("expected isAdminFieldAuthorized=true")
		}
	})

	t.Run("authorized false", func(t *testing.T) {
		ctx := withAdminFieldAuthorized(context.Background(), false)
		if isAdminFieldAuthorized(ctx) {
			t.Fatal("expected isAdminFieldAuthorized=false")
		}
	})

	t.Run("no key in context", func(t *testing.T) {
		if isAdminFieldAuthorized(context.Background()) {
			t.Fatal("expected isAdminFieldAuthorized=false for bare context")
		}
	})

	t.Run("nil context", func(t *testing.T) {
		var nilCtx context.Context
		if isAdminFieldAuthorized(nilCtx) {
			t.Fatal("expected isAdminFieldAuthorized=false for nil context")
		}
	})

	t.Run("nil context to withAdminFieldAuthorized", func(t *testing.T) {
		var nilCtx context.Context
		ctx := withAdminFieldAuthorized(nilCtx, true)
		if !isAdminFieldAuthorized(ctx) {
			t.Fatal("expected isAdminFieldAuthorized=true even when starting with nil")
		}
	})
}

func TestIsRequesterAdmin_RolesKeyPrecedence_DenyOverrideByRoleField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", func(c *gin.Context) {
		c.Set("roles", []string{"viewer"})
		c.Set("role", "admin")
		if isRequesterAdmin(c) {
			t.Fatalf("expected false when roles key is non-admin")
		}
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
