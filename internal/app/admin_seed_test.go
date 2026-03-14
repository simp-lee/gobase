package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/simp-lee/gobase/internal/config"
	"github.com/simp-lee/gobase/internal/domain"
	"github.com/simp-lee/gobase/internal/pkg"
)

func TestNew_AuthEnabled_BootstrapsSeedAdminLogin(t *testing.T) {
	t.Setenv(seedAdminPasswordEnv, "")
	t.Setenv(seedAdminEmailEnv, "")

	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 8080,
			Mode: gin.DebugMode,
		},
		Database: config.DatabaseConfig{
			Driver: "sqlite",
			SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "seed-admin-login.db")},
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "text",
		},
		Auth: config.AuthConfig{
			Enabled:     true,
			JWTSecret:   "test-secret-key-must-be-at-least-32-chars-long!",
			TokenExpiry: "24h",
			PublicPaths: []string{"/api/v1/auth/login", "/api/v1/auth/register"},
		},
	}

	app, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer cleanupTestApp(t, app)

	if got := strings.TrimSpace(os.Getenv(seedAdminPasswordEnv)); got != defaultSeedAdminPassord {
		t.Fatalf("seed admin password env = %q, want %q", got, defaultSeedAdminPassord)
	}

	var admin domain.User
	if err := app.db.Where("email = ?", defaultSeedAdminEmail).First(&admin).Error; err != nil {
		t.Fatalf("query seed admin: %v", err)
	}
	if admin.Role != domain.RoleAdmin {
		t.Fatalf("admin role = %q, want %q", admin.Role, domain.RoleAdmin)
	}
	if admin.Status != domain.StatusActive {
		t.Fatalf("admin status = %q, want %q", admin.Status, domain.StatusActive)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"admin@example.com","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")
	app.engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/auth/login status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp pkg.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal login response: %v", err)
	}
	if resp.Code != http.StatusOK {
		t.Fatalf("response code = %d, want %d", resp.Code, http.StatusOK)
	}
	if cookie := w.Header().Get("Set-Cookie"); !strings.Contains(cookie, "access_token=") {
		t.Fatalf("expected access_token cookie, got %q", cookie)
	}
}

func TestNew_AuthEnabled_ReconcilesExistingSeedAdminForLogin(t *testing.T) {
	t.Setenv(seedAdminPasswordEnv, "password123")
	t.Setenv(seedAdminEmailEnv, "admin@example.com")

	dbPath := filepath.Join(t.TempDir(), "seed-admin-reconcile.db")
	db := openSeedAdminTestDB(t, dbPath)
	wrongHash, err := bcrypt.GenerateFromPassword([]byte("wrong-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword() error = %v", err)
	}
	staleAdmin := domain.User{
		Username:     "Admin",
		Email:        "admin@example.com",
		PasswordHash: string(wrongHash),
		Role:         domain.RoleUser,
		Status:       domain.StatusDisabled,
	}
	if err := db.Create(&staleAdmin).Error; err != nil {
		t.Fatalf("create stale admin: %v", err)
	}
	closeSeedAdminTestDB(t, db)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 8080,
			Mode: gin.DebugMode,
		},
		Database: config.DatabaseConfig{
			Driver: "sqlite",
			SQLite: config.SQLiteConfig{Path: dbPath},
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "text",
		},
		Auth: config.AuthConfig{
			Enabled:     true,
			JWTSecret:   "test-secret-key-must-be-at-least-32-chars-long!",
			TokenExpiry: "24h",
			PublicPaths: []string{"/api/v1/auth/login", "/api/v1/auth/register"},
		},
	}

	app, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer cleanupTestApp(t, app)

	var admin domain.User
	if err := app.db.Where("email = ?", "admin@example.com").First(&admin).Error; err != nil {
		t.Fatalf("query reconciled admin: %v", err)
	}
	if admin.Role != domain.RoleAdmin {
		t.Fatalf("admin role = %q, want %q", admin.Role, domain.RoleAdmin)
	}
	if admin.Status != domain.StatusActive {
		t.Fatalf("admin status = %q, want %q", admin.Status, domain.StatusActive)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte("password123")); err != nil {
		t.Fatalf("seed admin password was not reconciled: %v", err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"admin@example.com","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")
	app.engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/auth/login status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if cookie := w.Header().Get("Set-Cookie"); !strings.Contains(cookie, "access_token=") {
		t.Fatalf("expected access_token cookie, got %q", cookie)
	}
}

func openSeedAdminTestDB(t *testing.T, dbPath string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.AutoMigrate(&domain.User{}); err != nil {
		t.Fatalf("auto migrate users: %v", err)
	}
	return db
}

func closeSeedAdminTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	if db == nil {
		return
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB() error = %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close sqlite db: %v", err)
	}
}
