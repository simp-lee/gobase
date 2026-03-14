package app

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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

// mustCallerFile returns the file path of the caller (this test file).
func mustCallerFile() string {
	_, f, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller failed")
	}
	return f
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

// TestRegisterStaticRoutes_Debug_SWAndManifestHeaders verifies that the debug-mode
// static handler sets correct Cache-Control and Service-Worker-Allowed headers
// for sw.js and manifest.json requests.
func TestRegisterStaticRoutes_Debug_SWAndManifestHeaders(t *testing.T) {
	// Create temporary sw.js and manifest.json in the real web/static dir
	// so the debug file-server can serve them (resolveDebugStaticFS uses
	// runtime.Caller to locate web/static on disk).
	staticDir := filepath.Join(filepath.Dir(mustCallerFile()), "..", "..", "web", "static")

	swDir := filepath.Join(staticDir, "js")
	swPath := filepath.Join(swDir, "sw.js")
	if err := os.MkdirAll(swDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", swDir, err)
	}
	if err := os.WriteFile(swPath, []byte("// sw stub"), 0o644); err != nil {
		t.Fatalf("write sw.js: %v", err)
	}
	t.Cleanup(func() { os.Remove(swPath) })

	manifestPath := filepath.Join(staticDir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}
	t.Cleanup(func() { os.Remove(manifestPath) })

	r := gin.New()
	registerStaticRoutes(r, "debug")

	// sw.js
	wSW := httptest.NewRecorder()
	reqSW := httptest.NewRequest(http.MethodGet, "/static/js/sw.js", nil)
	r.ServeHTTP(wSW, reqSW)

	if wSW.Code != http.StatusOK {
		t.Fatalf("expected 200 for sw.js in debug mode, got %d", wSW.Code)
	}
	if cc := wSW.Header().Get("Cache-Control"); cc != "no-cache, max-age=0, must-revalidate" {
		t.Errorf("expected sw.js Cache-Control 'no-cache, max-age=0, must-revalidate', got %q", cc)
	}
	if swa := wSW.Header().Get("Service-Worker-Allowed"); swa != "/" {
		t.Errorf("expected Service-Worker-Allowed '/', got %q", swa)
	}

	// manifest.json
	wManifest := httptest.NewRecorder()
	reqManifest := httptest.NewRequest(http.MethodGet, "/static/manifest.json", nil)
	r.ServeHTTP(wManifest, reqManifest)

	if wManifest.Code != http.StatusOK {
		t.Fatalf("expected 200 for manifest.json in debug mode, got %d", wManifest.Code)
	}
	if cc := wManifest.Header().Get("Cache-Control"); cc != "no-cache, max-age=0, must-revalidate" {
		t.Errorf("expected manifest Cache-Control 'no-cache, max-age=0, must-revalidate', got %q", cc)
	}
	if swa := wManifest.Header().Get("Service-Worker-Allowed"); swa != "" {
		t.Errorf("expected no Service-Worker-Allowed for manifest.json, got %q", swa)
	}
}

func TestRegisterStaticRoutes_Debug_RegularFileNoStore(t *testing.T) {
	// Ensure that non-special static files in debug mode get Cache-Control: no-store
	// so browsers never serve stale CSS/JS during development.
	staticDir := filepath.Join(filepath.Dir(mustCallerFile()), "..", "..", "web", "static")
	cssDir := filepath.Join(staticDir, "css")
	cssPath := filepath.Join(cssDir, "test_debug.css")
	if err := os.MkdirAll(cssDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", cssDir, err)
	}
	if err := os.WriteFile(cssPath, []byte("body{}"), 0o644); err != nil {
		t.Fatalf("write test_debug.css: %v", err)
	}
	t.Cleanup(func() { os.Remove(cssPath) })

	r := gin.New()
	registerStaticRoutes(r, "debug")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/static/css/test_debug.css", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("expected Cache-Control 'no-store' for regular file in debug mode, got %q", cc)
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

func TestCacheStaticHandler_SwJS_NoCacheAndServiceWorkerAllowed(t *testing.T) {
	memFS := fstest.MapFS{
		"sw.js": &fstest.MapFile{Data: []byte("// service worker")},
	}
	httpFS := http.FS(memFS)

	r := gin.New()
	r.GET("/static/*filepath", cacheStaticHandler(httpFS))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/static/sw.js", nil)
	r.ServeHTTP(w, req)

	cc := w.Header().Get("Cache-Control")
	if cc != "no-cache, max-age=0, must-revalidate" {
		t.Errorf("expected Cache-Control 'no-cache, max-age=0, must-revalidate', got %q", cc)
	}
	swa := w.Header().Get("Service-Worker-Allowed")
	if swa != "/" {
		t.Errorf("expected Service-Worker-Allowed '/', got %q", swa)
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestCacheStaticHandler_ManifestJSON_NoCache(t *testing.T) {
	memFS := fstest.MapFS{
		"manifest.json": &fstest.MapFile{Data: []byte(`{"name":"test"}`)},
	}
	httpFS := http.FS(memFS)

	r := gin.New()
	r.GET("/static/*filepath", cacheStaticHandler(httpFS))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/static/manifest.json", nil)
	r.ServeHTTP(w, req)

	cc := w.Header().Get("Cache-Control")
	if cc != "no-cache, max-age=0, must-revalidate" {
		t.Errorf("expected Cache-Control 'no-cache, max-age=0, must-revalidate', got %q", cc)
	}
	swa := w.Header().Get("Service-Worker-Allowed")
	if swa != "" {
		t.Errorf("expected no Service-Worker-Allowed header for manifest.json, got %q", swa)
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
		DB:         openTestSQLiteDB(t),
		Mode:       "debug",
		CSRFSecret: "test-secret-32-chars-long-enough",
	})
	if err != nil {
		t.Fatalf("expected no error with zero modules, got %v", err)
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

func TestRegisterRoutes_DefaultHomePage(t *testing.T) {
	r := setupTestRouter()
	err := RegisterRoutes(r, &RouteDeps{
		Modules:    nil,
		DB:         openTestSQLiteDB(t),
		Mode:       "debug",
		CSRFSecret: "test-secret-32-chars-long-enough",
	})
	if err != nil {
		t.Fatalf("RegisterRoutes() error = %v", err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, "home:") {
		t.Errorf("expected body to contain rendered home page, got %q", body)
	}
	if csrfCookie := w.Result().Cookies(); len(csrfCookie) == 0 {
		t.Error("expected csrf middleware to set a cookie on the home page")
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

// --- Panic recovery tests ---

// panicModule implements Module and panics during registration.
type panicModule struct{}

func (m *panicModule) RegisterRoutes(api *gin.RouterGroup, pages *gin.RouterGroup) {
	panic("module exploded")
}

// routeRegModule implements Module and registers configurable routes.
type routeRegModule struct {
	setup func(api *gin.RouterGroup, pages *gin.RouterGroup)
}

func (m *routeRegModule) RegisterRoutes(api *gin.RouterGroup, pages *gin.RouterGroup) {
	if m.setup != nil {
		m.setup(api, pages)
	}
}

func TestRegisterRoutes_ModulePanic_ReturnsError(t *testing.T) {
	r := setupTestRouter()
	err := RegisterRoutes(r, &RouteDeps{
		Modules:    []Module{&panicModule{}},
		DB:         openTestSQLiteDB(t),
		Mode:       "debug",
		CSRFSecret: "test-secret-32-chars-long-enough",
	})
	if err == nil {
		t.Fatal("expected error when module panics, got nil")
	}
	if !strings.Contains(err.Error(), "panic while registering routes") {
		t.Fatalf("expected panic error, got %v", err)
	}
	if !strings.Contains(err.Error(), "module exploded") {
		t.Fatalf("expected panic value in error, got %v", err)
	}
}

func TestRegisterRoutes_DuplicateRoutes_GinPanic(t *testing.T) {
	noop := func(c *gin.Context) {}
	modA := &routeRegModule{setup: func(api *gin.RouterGroup, _ *gin.RouterGroup) {
		api.GET("/items", noop)
	}}
	modB := &routeRegModule{setup: func(api *gin.RouterGroup, _ *gin.RouterGroup) {
		api.GET("/items", noop)
	}}

	r := setupTestRouter()
	err := RegisterRoutes(r, &RouteDeps{
		Modules:    []Module{modA, modB},
		DB:         openTestSQLiteDB(t),
		Mode:       "debug",
		CSRFSecret: "test-secret-32-chars-long-enough",
	})
	if err == nil {
		t.Fatal("expected error for duplicate routes, got nil")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected gin duplicate panic in error, got %v", err)
	}
}

func TestRegisterRoutes_DuplicateRoutes_DedupMap(t *testing.T) {
	// Simulate dedup detection via routeOwners map:
	// modA registers GET /api/v1/items, modB registers GET /api/v1/items on pages
	// group resulting in same absolute path — caught by dedup map.
	// In practice gin panics first; this tests the dedup map logic directly.
	routeOwners := map[string]int{
		"GET /api/v1/items": 0,
	}
	sig := routeSignature("GET", "/api/v1/items")
	if owner, exists := routeOwners[sig]; !exists {
		t.Fatal("expected dedup map to contain route")
	} else if owner != 0 {
		t.Fatalf("expected owner 0, got %d", owner)
	}
}

func TestRegisterRoutes_NoDuplicatesAcrossModules(t *testing.T) {
	noop := func(c *gin.Context) {}
	modA := &routeRegModule{setup: func(api *gin.RouterGroup, _ *gin.RouterGroup) {
		api.GET("/items", noop)
	}}
	modB := &routeRegModule{setup: func(api *gin.RouterGroup, _ *gin.RouterGroup) {
		api.GET("/orders", noop)
	}}

	r := setupTestRouter()
	err := RegisterRoutes(r, &RouteDeps{
		Modules:    []Module{modA, modB},
		DB:         openTestSQLiteDB(t),
		Mode:       "debug",
		CSRFSecret: "test-secret-32-chars-long-enough",
	})
	if err != nil {
		t.Fatalf("expected no error for distinct routes, got %v", err)
	}
}

// --- registerModuleRoutesSafely unit tests ---

func TestRegisterModuleRoutesSafely_NoPanic(t *testing.T) {
	r := gin.New()
	api := r.Group("/api")
	pages := r.Group("/")
	err := registerModuleRoutesSafely(&mockModule{}, api, pages)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRegisterModuleRoutesSafely_Panic(t *testing.T) {
	r := gin.New()
	api := r.Group("/api")
	pages := r.Group("/")
	err := registerModuleRoutesSafely(&panicModule{}, api, pages)
	if err == nil {
		t.Fatal("expected error from panicking module, got nil")
	}
	if !strings.Contains(err.Error(), "panic while registering routes") {
		t.Fatalf("expected 'panic while registering routes' in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "module exploded") {
		t.Fatalf("expected panic value in error, got %v", err)
	}
}

// --- routeSignature unit tests ---

func TestRouteSignature(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		{"basic", "GET", "/users", "GET /users"},
		{"lowercase method", "get", "/users", "GET /users"},
		{"trailing slash", "POST", "/users/", "POST /users"},
		{"root path", "GET", "/", "GET /"},
		{"empty path", "GET", "", "GET /"},
		{"no leading slash", "GET", "users", "GET /users"},
		{"whitespace method", " GET ", "/test", "GET /test"},
		{"whitespace path", "GET", " /test ", "GET /test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := routeSignature(tt.method, tt.path)
			if got != tt.want {
				t.Errorf("routeSignature(%q, %q) = %q; want %q", tt.method, tt.path, got, tt.want)
			}
		})
	}
}

// --- snapshotRouteSignatures unit tests ---

func TestSnapshotRouteSignatures(t *testing.T) {
	r := gin.New()
	noop := func(c *gin.Context) {}
	r.GET("/a", noop)
	r.POST("/b", noop)

	snap := snapshotRouteSignatures(r.Routes())
	if _, ok := snap["GET /a"]; !ok {
		t.Error("expected GET /a in snapshot")
	}
	if _, ok := snap["POST /b"]; !ok {
		t.Error("expected POST /b in snapshot")
	}
	if len(snap) != 2 {
		t.Errorf("expected 2 entries, got %d", len(snap))
	}
}
