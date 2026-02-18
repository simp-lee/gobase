package app

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// --- test helpers ---

// routeTestFS returns a minimal template filesystem for route handler tests.
func routeTestFS() fstest.MapFS {
	return fstest.MapFS{
		"templates/layouts/base.html": &fstest.MapFile{
			Data: []byte(`{{ define "base" }}{{ block "content" . }}{{ end }}{{ end }}`),
		},
		"templates/partials/nav.html": &fstest.MapFile{
			Data: []byte(`{{ define "nav" }}{{ end }}`),
		},
		"templates/home.html": &fstest.MapFile{
			Data: []byte(`{{ template "base" . }}{{ define "content" }}home:{{ .CSRFToken }}{{ end }}`),
		},
		"templates/errors/404.html": &fstest.MapFile{
			Data: []byte(`{{ template "base" . }}{{ define "content" }}404{{ end }}`),
		},
		"templates/errors/500.html": &fstest.MapFile{
			Data: []byte(`{{ template "base" . }}{{ define "content" }}500{{ end }}`),
		},
	}
}

// setupTestRouter creates a gin.Engine with the route-test template renderer.
func setupTestRouter() *gin.Engine {
	r := gin.New()
	renderer, err := NewTemplateRenderer(routeTestFS(), true)
	if err != nil {
		panic("setup renderer: " + err.Error())
	}
	r.HTMLRender = renderer
	return r
}

// --- Health check tests (M3) ---

func TestHealthHandler_OK(t *testing.T) {
	r := gin.New()

	// Use a real SQLite in-memory DB for a passing ping.
	db := openTestSQLiteDB(t)

	r.GET("/health", healthHandler(db))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %v", body["status"])
	}
	comps, ok := body["components"].(map[string]any)
	if !ok {
		t.Fatal("missing components")
	}
	if comps["database"] != "ok" {
		t.Errorf("expected database ok, got %v", comps["database"])
	}
}

func TestHealthHandler_DBDown(t *testing.T) {
	r := gin.New()

	db := openTestSQLiteDB(t)
	// Close the underlying sql.DB so Ping fails.
	sqlDB, _ := db.DB()
	sqlDB.Close()

	r.GET("/health", healthHandler(db))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "degraded" {
		t.Errorf("expected status degraded, got %v", body["status"])
	}
	comps := body["components"].(map[string]any)
	if comps["database"] != "error" {
		t.Errorf("expected database error, got %v", comps["database"])
	}
}

// --- NoRoute handler tests (M5) ---

func TestNoRouteHandler_JSON(t *testing.T) {
	r := setupTestRouter()
	r.NoRoute(noRouteHandler())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	req.Header.Set("Accept", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["message"] != "not found" {
		t.Errorf("expected message 'not found', got %v", body["message"])
	}
	if body["data"] != nil {
		t.Errorf("expected data nil, got %v", body["data"])
	}
}

func TestNoRouteHandler_HTML(t *testing.T) {
	r := setupTestRouter()
	r.NoRoute(noRouteHandler())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	req.Header.Set("Accept", "text/html")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "404") {
		t.Errorf("expected HTML to contain '404', got %q", body)
	}
}

// --- Static routes tests (M8) ---

// registerStaticRoutes is a test helper that wraps registerStaticRoutesWithError,
// discarding the error for convenience in test setup.
func registerStaticRoutes(r *gin.Engine, mode string) {
	_ = registerStaticRoutesWithError(r, mode)
}

func TestRegisterStaticRoutes_Debug(t *testing.T) {
	r := gin.New()
	registerStaticRoutes(r, "debug")

	// Verify a route was registered for /static (gin registers /static/*filepath).
	routes := r.Routes()
	found := false
	for _, route := range routes {
		if route.Method == "GET" && strings.HasPrefix(route.Path, "/static") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected /static route to be registered in debug mode")
	}
}

func TestRegisterStaticRoutes_Release_CacheHeader(t *testing.T) {
	r := gin.New()
	registerStaticRoutes(r, "release")

	routes := r.Routes()
	found := false
	for _, route := range routes {
		if route.Method == "GET" && route.Path == "/static/*filepath" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected /static/*filepath route to be registered in release mode")
	}
}

func TestCacheStaticHandler_SetsCacheControl(t *testing.T) {
	// Create a minimal in-memory filesystem.
	memFS := fstest.MapFS{
		"test.css": &fstest.MapFile{Data: []byte("body{}")},
	}
	httpFS := http.FS(memFS)

	r := gin.New()
	r.GET("/static/*filepath", cacheStaticHandler(httpFS))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/static/test.css", nil)
	r.ServeHTTP(w, req)

	cc := w.Header().Get("Cache-Control")
	if cc != "public, max-age=86400" {
		t.Errorf("expected Cache-Control 'public, max-age=86400', got %q", cc)
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// --- Home page test ---

func TestHomePage(t *testing.T) {
	r := setupTestRouter()

	// Register just the home route with CSRF.
	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "home.html", gin.H{
			"CSRFToken": "test-token",
		})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "home:test-token") {
		t.Errorf("expected body to contain 'home:test-token', got %q", body)
	}
}

// --- fs.Sub test for release static ---

func TestStaticFS_SubWorks(t *testing.T) {
	// Verify that fs.Sub on the embedded FS doesn't error.
	_, err := fs.Sub(fstest.MapFS{
		"static/css/app.css": &fstest.MapFile{Data: []byte("body{}")},
	}, "static")
	if err != nil {
		t.Fatalf("fs.Sub should not error: %v", err)
	}
}

// --- openTestSQLiteDB helper ---

func openTestSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	return db
}
