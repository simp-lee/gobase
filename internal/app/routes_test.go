package app

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
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

func TestHealthHandler_UsesRequestContextTimeout(t *testing.T) {
	registerBlockingPingDriver()

	sqlDB, err := sql.Open(blockingPingDriverName, "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}

	r := gin.New()
	r.GET("/health", healthHandler(db))

	reqCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	t.Cleanup(cancel)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil).WithContext(reqCtx)

	start := time.Now()
	r.ServeHTTP(w, req)
	elapsed := time.Since(start)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	if elapsed > 300*time.Millisecond {
		t.Fatalf("expected health response to honor request context timeout, elapsed=%v", elapsed)
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

func TestNoRouteHandler_HTMLWildcardAccept(t *testing.T) {
	r := setupTestRouter()
	r.NoRoute(noRouteHandler())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	req.Header.Set("Accept", "*/*")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "404") {
		t.Errorf("expected HTML to contain '404', got %q", w.Body.String())
	}
}

func TestNoRouteHandler_APIPath_PrefersJSON(t *testing.T) {
	r := setupTestRouter()
	r.NoRoute(noRouteHandler())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/nonexistent", nil)
	req.Header.Set("Accept", "*/*")
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

// --- RegisterRoutes validation tests ---

// mockModule implements Module for testing.
type mockModule struct {
	called bool
}

func (m *mockModule) RegisterRoutes(api *gin.RouterGroup, pages *gin.RouterGroup) {
	m.called = true
}

func TestRegisterRoutes_NilRouter(t *testing.T) {
	err := RegisterRoutes(nil, &RouteDeps{})
	if err == nil || !strings.Contains(err.Error(), "router is nil") {
		t.Fatalf("expected 'router is nil' error, got %v", err)
	}
}

func TestRegisterRoutes_NilDeps(t *testing.T) {
	r := setupTestRouter()
	err := RegisterRoutes(r, nil)
	if err == nil || !strings.Contains(err.Error(), "route dependencies are nil") {
		t.Fatalf("expected 'route dependencies are nil' error, got %v", err)
	}
}

func TestRegisterRoutes_NoModules(t *testing.T) {
	r := setupTestRouter()
	err := RegisterRoutes(r, &RouteDeps{
		CSRFSecret: "test-secret-32-chars-long-enough",
	})
	if err == nil || !strings.Contains(err.Error(), "at least one module is required") {
		t.Fatalf("expected 'at least one module is required' error, got %v", err)
	}
}

func TestRegisterRoutes_EmptyCSRF(t *testing.T) {
	r := setupTestRouter()
	err := RegisterRoutes(r, &RouteDeps{
		Modules:    []Module{&mockModule{}},
		CSRFSecret: "",
	})
	if err == nil || !strings.Contains(err.Error(), "csrf secret is required") {
		t.Fatalf("expected 'csrf secret is required' error, got %v", err)
	}
}

func TestRegisterRoutes_ModulesAreCalled(t *testing.T) {
	m := &mockModule{}
	r := setupTestRouter()
	err := RegisterRoutes(r, &RouteDeps{
		Modules:    []Module{m},
		DB:         openTestSQLiteDB(t),
		Mode:       "debug",
		CSRFSecret: "test-secret-32-chars-long-enough",
	})
	if err != nil {
		t.Fatalf("RegisterRoutes: %v", err)
	}
	if !m.called {
		t.Error("expected module RegisterRoutes to be called")
	}
}

func TestRegisterRoutes_NilModuleEntry(t *testing.T) {
	r := setupTestRouter()
	err := RegisterRoutes(r, &RouteDeps{
		Modules:    []Module{&mockModule{}, nil},
		DB:         openTestSQLiteDB(t),
		Mode:       "debug",
		CSRFSecret: "test-secret-32-chars-long-enough",
	})
	if err == nil {
		t.Fatal("expected error for nil module entry, got nil")
	}
	if !strings.Contains(err.Error(), "module at index 1 is nil") {
		t.Fatalf("expected indexed nil-module error, got %v", err)
	}
}

func TestNoRouteHandler_APIv1Path_PrefersJSON(t *testing.T) {
	r := setupTestRouter()
	r.NoRoute(noRouteHandler())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nonexistent", nil)
	req.Header.Set("Accept", "*/*")
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
}

// TestNoRouteHandler_JSONWithWildcardAccept verifies that a non-/api/ path with
// Accept: application/json, */* receives a JSON 404 response, not HTML.
// The JSON guard runs before acceptsHTML so that */* does not win for explicit
// JSON clients.
func TestNoRouteHandler_JSONWithWildcardAccept(t *testing.T) {
	r := setupTestRouter()
	r.NoRoute(noRouteHandler())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	req.Header.Set("Accept", "application/json, */*")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["message"] != "not found" {
		t.Errorf("expected message 'not found', got %v", body["message"])
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected JSON Content-Type, got %q", ct)
	}
}

// TestNoRouteHandler_ExactAPIPath_WithWildcardAccept documents intentional
// behaviour after the trailing-slash fix: path "/api" (no trailing slash) no
// longer matches the "/api/" prefix guard and is therefore treated as a
// browser-like request, returning an HTML 404.
func TestNoRouteHandler_ExactAPIPath_WithWildcardAccept(t *testing.T) {
	r := setupTestRouter()
	r.NoRoute(noRouteHandler())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Accept", "*/*")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	// */* matches acceptsHTML — expect HTML, not JSON.
	body := w.Body.String()
	if !strings.Contains(body, "404") {
		t.Errorf("expected HTML 404 page, got %q", body)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("expected HTML Content-Type, got %q", ct)
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

const blockingPingDriverName = "copilot_blocking_ping"

var registerBlockingPingDriverOnce sync.Once

func registerBlockingPingDriver() {
	registerBlockingPingDriverOnce.Do(func() {
		sql.Register(blockingPingDriverName, blockingPingDriver{})
	})
}

type blockingPingDriver struct{}

func (blockingPingDriver) Open(string) (driver.Conn, error) {
	return blockingPingConn{}, nil
}

type blockingPingConn struct{}

func (blockingPingConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (blockingPingConn) Close() error                        { return nil }
func (blockingPingConn) Begin() (driver.Tx, error)           { return blockingPingTx{}, nil }

func (blockingPingConn) Ping(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

type blockingPingTx struct{}

func (blockingPingTx) Commit() error   { return nil }
func (blockingPingTx) Rollback() error { return nil }
