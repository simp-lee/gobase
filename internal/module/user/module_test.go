package user

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestUserModuleImplementsModuleInterface verifies that UserModule satisfies
// the app.Module interface contract (RegisterRoutes(api, pages *gin.RouterGroup)).
// We don't import app to keep the dependency direction clean; instead we check
// that the method exists with the correct signature and that routes are registered.
func TestUserModuleRegisterRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	pages := r.Group("/")

	mod := NewModule(
		&UserHandler{},
		&UserPageHandler{},
	)
	mod.RegisterRoutes(api, pages)

	// Expected routes: method + path
	expected := []struct {
		method string
		path   string
	}{
		// API routes
		{http.MethodPost, "/api/users"},
		{http.MethodGet, "/api/users/:id"},
		{http.MethodGet, "/api/users"},
		{http.MethodPut, "/api/users/:id"},
		{http.MethodDelete, "/api/users/:id"},
		// Page routes
		{http.MethodGet, "/users"},
		{http.MethodGet, "/users/new"},
		{http.MethodGet, "/users/:id/edit"},
		{http.MethodPost, "/users"},
		{http.MethodPut, "/users/:id"},
		{http.MethodDelete, "/users/:id"},
	}

	routes := r.Routes()
	registered := make(map[string]bool)
	for _, ri := range routes {
		registered[ri.Method+":"+ri.Path] = true
	}

	for _, exp := range expected {
		key := exp.method + ":" + exp.path
		if !registered[key] {
			t.Errorf("expected route %s %s to be registered", exp.method, exp.path)
		}
	}
}

func TestNewModule_PanicsOnNilHandler(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewModule() expected panic for nil handler, got none")
		}
	}()

	_ = NewModule(nil, &UserPageHandler{})
}

func TestNewModule_PanicsOnNilPageHandler(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewModule() expected panic for nil page handler, got none")
		}
	}()

	_ = NewModule(&UserHandler{}, nil)
}
