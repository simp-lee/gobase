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

	user := &domain.User{Name: "Alice", Email: "alice@example.com"}
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
	if got.Name != "Alice" || got.Email != "alice@example.com" {
		t.Errorf("got %+v; want Name=Alice, Email=alice@example.com", got)
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

func TestCreate_DuplicateEmail(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	u1 := &domain.User{Name: "Alice", Email: "dup@example.com"}
	if err := repo.Create(ctx, u1); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	u2 := &domain.User{Name: "Bob", Email: "dup@example.com"}
	err := repo.Create(ctx, u2)
	if !domain.IsAlreadyExists(err) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestUpdate(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	user := &domain.User{Name: "Alice", Email: "alice@example.com"}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	user.Name = "Alice Updated"
	if err := repo.Update(ctx, user); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := repo.GetByID(ctx, user.ID)
	if got.Name != "Alice Updated" {
		t.Errorf("Name=%q; want Alice Updated", got.Name)
	}
}

func TestDelete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	user := &domain.User{Name: "Alice", Email: "alice@example.com"}
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
			Name:  "User" + string(rune('A'-1+i)),
			Email: "user" + string(rune('a'-1+i)) + "@example.com",
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

	if result.Total != 5 {
		t.Errorf("Total=%d; want 5", result.Total)
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
		{Name: "Alice", Email: "alice@example.com"},
		{Name: "Bob", Email: "bob@example.com"},
		{Name: "Charlie", Email: "charlie@example.com"},
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
		Filter:   map[string]string{"name": "Alice"},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("Total=%d; want 1", result.Total)
	}
	if len(result.Items) != 1 || result.Items[0].Name != "Alice" {
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
	if result.Total != 0 {
		t.Errorf("Total=%d; want 0", result.Total)
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
			Name:  fmt.Sprintf("User%02d", i),
			Email: fmt.Sprintf("user%02d@example.com", i),
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
	if result.Total != 25 {
		t.Errorf("Total=%d; want 25", result.Total)
	}
	if len(result.Items) != 10 {
		t.Errorf("Items count=%d; want 10", len(result.Items))
	}
	if result.TotalPages != 3 {
		t.Errorf("TotalPages=%d; want 3", result.TotalPages)
	}
	if result.Page != 2 {
		t.Errorf("Page=%d; want 2", result.Page)
	}
	// Page 2 with id:asc should start at User11 (ID offset 11)
	if result.Items[0].Name != "User11" {
		t.Errorf("first item Name=%q; want User11", result.Items[0].Name)
	}
	if result.Items[9].Name != "User20" {
		t.Errorf("last item Name=%q; want User20", result.Items[9].Name)
	}
}

func TestList_Sort(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	names := []string{"Charlie", "Alice", "Bob"}
	for _, n := range names {
		u := &domain.User{Name: n, Email: strings.ToLower(n) + "@example.com"}
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
		{"name_asc", "name:asc", "Alice", "Charlie"},
		{"name_desc", "name:desc", "Charlie", "Alice"},
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
			if result.Items[0].Name != tt.wantFirst {
				t.Errorf("first=%q; want %q", result.Items[0].Name, tt.wantFirst)
			}
			last := result.Items[len(result.Items)-1]
			if last.Name != tt.wantLast {
				t.Errorf("last=%q; want %q", last.Name, tt.wantLast)
			}
		})
	}
}

func TestList_FilterLike(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	users := []domain.User{
		{Name: "Alice Smith", Email: "alice@example.com"},
		{Name: "Alice Jones", Email: "alice.jones@example.com"},
		{Name: "Bob Smith", Email: "bob@example.com"},
	}
	for i := range users {
		if err := repo.Create(ctx, &users[i]); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	// __like on name
	result, err := repo.List(ctx, domain.PageRequest{
		Page:     1,
		PageSize: 20,
		Sort:     "id:asc",
		Filter:   map[string]string{"name__like": "Alice"},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("Total=%d; want 2", result.Total)
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
	if result.Total != 2 {
		t.Errorf("Total=%d; want 2 (alice@, alice.jones@)", result.Total)
	}

	// __like with no match
	result, err = repo.List(ctx, domain.PageRequest{
		Page:     1,
		PageSize: 20,
		Sort:     "id:asc",
		Filter:   map[string]string{"name__like": "Zara"},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("Total=%d; want 0", result.Total)
	}
}
