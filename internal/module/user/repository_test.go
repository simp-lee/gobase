package user

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/simp-lee/gobase/internal/domain"
	"gorm.io/gorm"
)

type legacyUser struct {
	domain.BaseModel
	Username     string `gorm:"size:100;not null"`
	Email        string `gorm:"size:255;uniqueIndex;not null"`
	PasswordHash string `gorm:"size:255"`
	Role         string `gorm:"size:20;not null;default:'user';check:chk_user_role,role IN ('admin','user')"`
}

func (legacyUser) TableName() string {
	return "users"
}

// setupTestDB creates an in-memory SQLite database with the User table.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&domain.User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestCreateAndGetByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	// Status left empty intentionally — verifies DB default fills "active".
	user := &domain.User{Username: "Alice", Email: "alice@example.com"}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if user.ID == 0 {
		t.Fatal("expected non-zero ID after Create")
	}

	got, err := repo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Username != "Alice" || got.Email != "alice@example.com" {
		t.Errorf("got %+v; want Username=Alice, Email=alice@example.com", got)
	}
	if got.Status != domain.StatusActive {
		t.Errorf("Status=%q; want %q", got.Status, domain.StatusActive)
	}
}

func TestAutoMigrate_LegacyUsersDefaultToActiveStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	if err := db.AutoMigrate(&legacyUser{}); err != nil {
		t.Fatalf("migrate legacy users table: %v", err)
	}
	if err := db.Create(&legacyUser{
		Username: "Legacy",
		Email:    "legacy@example.com",
		Role:     domain.RoleUser,
	}).Error; err != nil {
		t.Fatalf("insert legacy user: %v", err)
	}

	if err := db.AutoMigrate(&domain.User{}); err != nil {
		t.Fatalf("upgrade users table: %v", err)
	}

	repo := NewUserRepository(db)
	got, err := repo.GetByEmail(context.Background(), "legacy@example.com")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if got.Status != domain.StatusActive {
		t.Fatalf("Status=%q; want %q", got.Status, domain.StatusActive)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	_, err := repo.GetByID(context.Background(), 999)
	if !domain.IsNotFound(err) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetByEmail(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	user := &domain.User{Username: "Alice", Email: "alice@example.com"}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByEmail(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if got.Username != "Alice" || got.Email != "alice@example.com" {
		t.Errorf("got %+v; want Username=Alice, Email=alice@example.com", got)
	}
	if got.ID != user.ID {
		t.Errorf("ID=%d; want %d", got.ID, user.ID)
	}
}

func TestGetByEmail_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	_, err := repo.GetByEmail(context.Background(), "nobody@example.com")
	if !domain.IsNotFound(err) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestCreate_DuplicateEmail(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	u1 := &domain.User{Username: "Alice", Email: "dup@example.com"}
	if err := repo.Create(ctx, u1); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	u2 := &domain.User{Username: "Bob", Email: "dup@example.com"}
	err := repo.Create(ctx, u2)
	if !domain.IsAlreadyExists(err) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestUpdate(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	user := &domain.User{Username: "Alice", Email: "alice@example.com"}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	user.Username = "Alice Updated"
	if err := repo.Update(ctx, user); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := repo.GetByID(ctx, user.ID)
	if got.Username != "Alice Updated" {
		t.Errorf("Username=%q; want Alice Updated", got.Username)
	}
}

func TestDelete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	user := &domain.User{Username: "Alice", Email: "alice@example.com"}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Delete(ctx, user.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.GetByID(ctx, user.ID)
	if !domain.IsNotFound(err) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	err := repo.Delete(context.Background(), 999)
	if !domain.IsNotFound(err) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestList_Basic(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		u := &domain.User{
			Username: "User" + string(rune('A'-1+i)),
			Email:    "user" + string(rune('a'-1+i)) + "@example.com",
		}
		if err := repo.Create(ctx, u); err != nil {
			t.Fatalf("Create user %d: %v", i, err)
		}
	}

	result, err := repo.List(ctx, domain.PageRequest{
		Page:     1,
		PageSize: 3,
		Sort:     "id:asc",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if result.TotalItems != 5 {
		t.Errorf("TotalItems=%d; want 5", result.TotalItems)
	}
	if len(result.Items) != 3 {
		t.Errorf("Items count=%d; want 3", len(result.Items))
	}
	if result.TotalPages != 2 {
		t.Errorf("TotalPages=%d; want 2", result.TotalPages)
	}
}

func TestList_Filter(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	users := []domain.User{
		{Username: "Alice", Email: "alice@example.com"},
		{Username: "Bob", Email: "bob@example.com"},
		{Username: "Charlie", Email: "charlie@example.com"},
	}
	for i := range users {
		if err := repo.Create(ctx, &users[i]); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	result, err := repo.List(ctx, domain.PageRequest{
		Page:     1,
		PageSize: 20,
		Sort:     "id:asc",
		Filter:   map[string]string{"username": "Alice"},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.TotalItems != 1 {
		t.Errorf("TotalItems=%d; want 1", result.TotalItems)
	}
	if len(result.Items) != 1 || result.Items[0].Username != "Alice" {
		t.Errorf("expected Alice, got %+v", result.Items)
	}
}

func TestList_Empty(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	result, err := repo.List(context.Background(), domain.PageRequest{
		Page:     1,
		PageSize: 20,
		Sort:     "id:asc",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.TotalItems != 0 {
		t.Errorf("TotalItems=%d; want 0", result.TotalItems)
	}
	if result.Items == nil {
		t.Error("Items should not be nil")
	}
}

func TestList_Pagination25(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	for i := 1; i <= 25; i++ {
		u := &domain.User{
			Username: fmt.Sprintf("User%02d", i),
			Email:    fmt.Sprintf("user%02d@example.com", i),
		}
		if err := repo.Create(ctx, u); err != nil {
			t.Fatalf("Create user %d: %v", i, err)
		}
	}

	result, err := repo.List(ctx, domain.PageRequest{
		Page:     2,
		PageSize: 10,
		Sort:     "id:asc",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.TotalItems != 25 {
		t.Errorf("TotalItems=%d; want 25", result.TotalItems)
	}
	if len(result.Items) != 10 {
		t.Errorf("Items count=%d; want 10", len(result.Items))
	}
	if result.TotalPages != 3 {
		t.Errorf("TotalPages=%d; want 3", result.TotalPages)
	}
	if result.CurrentPage != 2 {
		t.Errorf("CurrentPage=%d; want 2", result.CurrentPage)
	}
	// Page 2 with id:asc should start at User11 (ID offset 11)
	if result.Items[0].Username != "User11" {
		t.Errorf("first item Username=%q; want User11", result.Items[0].Username)
	}
	if result.Items[9].Username != "User20" {
		t.Errorf("last item Username=%q; want User20", result.Items[9].Username)
	}
}

func TestList_Sort(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	names := []string{"Charlie", "Alice", "Bob"}
	for _, n := range names {
		u := &domain.User{Username: n, Email: strings.ToLower(n) + "@example.com"}
		if err := repo.Create(ctx, u); err != nil {
			t.Fatalf("Create %s: %v", n, err)
		}
	}

	tests := []struct {
		name      string
		sort      string
		wantFirst string
		wantLast  string
	}{
		{"username_asc", "username:asc", "Alice", "Charlie"},
		{"username_desc", "username:desc", "Charlie", "Alice"},
		{"email_asc", "email:asc", "Alice", "Charlie"},
		{"id_desc", "id:desc", "Bob", "Charlie"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.List(ctx, domain.PageRequest{
				Page:     1,
				PageSize: 10,
				Sort:     tt.sort,
			})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if result.Items[0].Username != tt.wantFirst {
				t.Errorf("first=%q; want %q", result.Items[0].Username, tt.wantFirst)
			}
			last := result.Items[len(result.Items)-1]
			if last.Username != tt.wantLast {
				t.Errorf("last=%q; want %q", last.Username, tt.wantLast)
			}
		})
	}
}

func TestList_FilterByStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	users := []domain.User{
		{Username: "Active1", Email: "a1@example.com", Status: domain.StatusActive},
		{Username: "Active2", Email: "a2@example.com", Status: domain.StatusActive},
		{Username: "Disabled1", Email: "d1@example.com", Status: domain.StatusDisabled},
		{Username: "Pending1", Email: "p1@example.com", Status: domain.StatusPending},
	}
	for i := range users {
		if err := repo.Create(ctx, &users[i]); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	tests := []struct {
		name      string
		status    string
		wantCount int64
	}{
		{"active only", domain.StatusActive, 2},
		{"disabled only", domain.StatusDisabled, 1},
		{"pending only", domain.StatusPending, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.List(ctx, domain.PageRequest{
				Page:     1,
				PageSize: 20,
				Sort:     "id:asc",
				Filter:   map[string]string{"status": tt.status},
			})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if result.TotalItems != tt.wantCount {
				t.Errorf("TotalItems=%d; want %d", result.TotalItems, tt.wantCount)
			}
			for _, u := range result.Items {
				if u.Status != tt.status {
					t.Errorf("got user with status=%q; want %q", u.Status, tt.status)
				}
			}
		})
	}
}

func TestList_SortByStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	// Create users with different statuses; SQLite sorts strings lexicographically.
	users := []domain.User{
		{Username: "Pending", Email: "p@example.com", Status: domain.StatusPending},
		{Username: "Active", Email: "a@example.com", Status: domain.StatusActive},
		{Username: "Disabled", Email: "d@example.com", Status: domain.StatusDisabled},
	}
	for i := range users {
		if err := repo.Create(ctx, &users[i]); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	t.Run("status_asc", func(t *testing.T) {
		result, err := repo.List(ctx, domain.PageRequest{
			Page: 1, PageSize: 20, Sort: "status:asc",
		})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		// Lexicographic: active < disabled < pending
		if result.Items[0].Status != domain.StatusActive {
			t.Errorf("first status=%q; want %q", result.Items[0].Status, domain.StatusActive)
		}
		last := result.Items[len(result.Items)-1]
		if last.Status != domain.StatusPending {
			t.Errorf("last status=%q; want %q", last.Status, domain.StatusPending)
		}
	})

	t.Run("status_desc", func(t *testing.T) {
		result, err := repo.List(ctx, domain.PageRequest{
			Page: 1, PageSize: 20, Sort: "status:desc",
		})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if result.Items[0].Status != domain.StatusPending {
			t.Errorf("first status=%q; want %q", result.Items[0].Status, domain.StatusPending)
		}
		last := result.Items[len(result.Items)-1]
		if last.Status != domain.StatusActive {
			t.Errorf("last status=%q; want %q", last.Status, domain.StatusActive)
		}
	})
}

func TestList_FilterLike(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	users := []domain.User{
		{Username: "Alice Smith", Email: "alice@example.com"},
		{Username: "Alice Jones", Email: "alice.jones@example.com"},
		{Username: "Bob Smith", Email: "bob@example.com"},
	}
	for i := range users {
		if err := repo.Create(ctx, &users[i]); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	// __like on username
	result, err := repo.List(ctx, domain.PageRequest{
		Page:     1,
		PageSize: 20,
		Sort:     "id:asc",
		Filter:   map[string]string{"username__like": "Alice"},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.TotalItems != 2 {
		t.Errorf("TotalItems=%d; want 2", result.TotalItems)
	}

	// __like on email
	result, err = repo.List(ctx, domain.PageRequest{
		Page:     1,
		PageSize: 20,
		Sort:     "id:asc",
		Filter:   map[string]string{"email__like": "alice"},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.TotalItems != 2 {
		t.Errorf("TotalItems=%d; want 2 (alice@, alice.jones@)", result.TotalItems)
	}

	// __like with no match
	result, err = repo.List(ctx, domain.PageRequest{
		Page:     1,
		PageSize: 20,
		Sort:     "id:asc",
		Filter:   map[string]string{"username__like": "Zara"},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.TotalItems != 0 {
		t.Errorf("TotalItems=%d; want 0", result.TotalItems)
	}
}
