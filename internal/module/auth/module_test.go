package auth

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestAuthModuleRegisterRoutes verifies that AuthModule satisfies the
// app.Module interface contract (RegisterRoutes(api, pages *gin.RouterGroup))
// and registers the expected auth routes.
func TestAuthModuleRegisterRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	pages := r.Group("/")

	mod := NewModule(&AuthHandler{})
	mod.RegisterRoutes(api, pages)

	expected := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/auth/login"},
		{http.MethodPost, "/api/auth/register"},
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

	_ = NewModule(nil)
}
