package user

import (
	"context"
	"errors"
	"testing"

	"github.com/simp-lee/gobase/internal/domain"
)

// --- mock repository ---

type mockUserRepo struct {
	users  map[uint]*domain.User
	nextID uint
	// hooks for error injection
	createErr error
	updateErr error
	deleteErr error
}

func newMockRepo() *mockUserRepo {
	return &mockUserRepo{users: make(map[uint]*domain.User), nextID: 1}
}

func (m *mockUserRepo) Create(_ context.Context, user *domain.User) error {
	if m.createErr != nil {
		return m.createErr
	}
	user.ID = m.nextID
	m.nextID++
	m.users[user.ID] = user
	return nil
}

func (m *mockUserRepo) GetByID(_ context.Context, id uint) (*domain.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return u, nil
}

func (m *mockUserRepo) List(_ context.Context, req domain.PageRequest) (*domain.PageResult[domain.User], error) {
	items := make([]domain.User, 0, len(m.users))
	for _, u := range m.users {
		items = append(items, *u)
	}
	return &domain.PageResult[domain.User]{
		Items:    items,
		Total:    int64(len(items)),
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}

func (m *mockUserRepo) Update(_ context.Context, user *domain.User) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	if _, ok := m.users[user.ID]; !ok {
		return domain.ErrNotFound
	}
	m.users[user.ID] = user
	return nil
}

func (m *mockUserRepo) Delete(_ context.Context, id uint) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if _, ok := m.users[id]; !ok {
		return domain.ErrNotFound
	}
	delete(m.users, id)
	return nil
}

// --- tests ---

func TestCreateUser(t *testing.T) {
	tests := []struct {
		name      string
		userName  string
		email     string
		createErr error
		wantErr   bool
		errCode   int
	}{
		{"success", "Alice", "alice@example.com", nil, false, 0},
		{"empty name", "", "a@b.com", nil, true, domain.CodeValidation},
		{"whitespace name", "   ", "a@b.com", nil, true, domain.CodeValidation},
		{"short name", "A", "a@b.com", nil, true, domain.CodeValidation},
		{"empty email", "Alice", "", nil, true, domain.CodeValidation},
		{"whitespace email", "Alice", "   ", nil, true, domain.CodeValidation},
		{"invalid email format", "Alice", "not-an-email", nil, true, domain.CodeValidation},
		{"repo error", "Alice", "alice@example.com", errors.New("db error"), true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.createErr = tt.createErr
			svc := NewUserService(repo)

			user, err := svc.CreateUser(context.Background(), tt.userName, tt.email)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errCode != 0 {
					var appErr *domain.AppError
					if !errors.As(err, &appErr) || appErr.Code != tt.errCode {
						t.Errorf("expected error code %d, got %v", tt.errCode, err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if user.ID == 0 {
				t.Error("expected user ID to be set")
			}
			if user.Name != tt.userName {
				t.Errorf("name = %q; want %q", user.Name, tt.userName)
			}
			if user.Email != tt.email {
				t.Errorf("email = %q; want %q", user.Email, tt.email)
			}
		})
	}
}

func TestGetUser(t *testing.T) {
	repo := newMockRepo()
	svc := NewUserService(repo)

	// seed
	created, err := svc.CreateUser(context.Background(), "Bob", "bob@example.com")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	t.Run("found", func(t *testing.T) {
		user, err := svc.GetUser(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.Name != "Bob" {
			t.Errorf("name = %q; want Bob", user.Name)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := svc.GetUser(context.Background(), 9999)
		if !domain.IsNotFound(err) {
			t.Errorf("expected not found error, got %v", err)
		}
	})
}

func TestListUsers(t *testing.T) {
	t.Run("with items", func(t *testing.T) {
		repo := newMockRepo()
		svc := NewUserService(repo)

		_, _ = svc.CreateUser(context.Background(), "Al", "a@example.com")
		_, _ = svc.CreateUser(context.Background(), "Bo", "b@example.com")

		result, err := svc.ListUsers(context.Background(), domain.PageRequest{Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Total != 2 {
			t.Errorf("total = %d; want 2", result.Total)
		}
		if len(result.Items) != 2 {
			t.Errorf("items count = %d; want 2", len(result.Items))
		}
	})

	t.Run("empty list", func(t *testing.T) {
		repo := newMockRepo()
		svc := NewUserService(repo)

		result, err := svc.ListUsers(context.Background(), domain.PageRequest{Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Total != 0 {
			t.Errorf("total = %d; want 0", result.Total)
		}
		if len(result.Items) != 0 {
			t.Errorf("items count = %d; want 0", len(result.Items))
		}
	})

	t.Run("page request values passed through", func(t *testing.T) {
		repo := newMockRepo()
		svc := NewUserService(repo)

		_, _ = svc.CreateUser(context.Background(), "Al", "a@example.com")

		result, err := svc.ListUsers(context.Background(), domain.PageRequest{Page: 3, PageSize: 25})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Page != 3 {
			t.Errorf("page = %d; want 3", result.Page)
		}
		if result.PageSize != 25 {
			t.Errorf("pageSize = %d; want 25", result.PageSize)
		}
	})
}

func TestUpdateUser(t *testing.T) {
	repo := newMockRepo()
	svc := NewUserService(repo)

	created, _ := svc.CreateUser(context.Background(), "Old", "old@example.com")

	t.Run("success", func(t *testing.T) {
		updated, err := svc.UpdateUser(context.Background(), created.ID, "New", "new@example.com")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updated.Name != "New" {
			t.Errorf("name = %q; want New", updated.Name)
		}
		if updated.Email != "new@example.com" {
			t.Errorf("email = %q; want new@example.com", updated.Email)
		}
	})

	t.Run("empty name", func(t *testing.T) {
		_, err := svc.UpdateUser(context.Background(), created.ID, "", "new@example.com")
		if !domain.IsValidation(err) {
			t.Errorf("expected validation error, got %v", err)
		}
	})

	t.Run("whitespace name", func(t *testing.T) {
		_, err := svc.UpdateUser(context.Background(), created.ID, "   ", "new@example.com")
		if !domain.IsValidation(err) {
			t.Errorf("expected validation error, got %v", err)
		}
	})

	t.Run("empty email", func(t *testing.T) {
		_, err := svc.UpdateUser(context.Background(), created.ID, "New", "")
		if !domain.IsValidation(err) {
			t.Errorf("expected validation error, got %v", err)
		}
	})

	t.Run("short name", func(t *testing.T) {
		_, err := svc.UpdateUser(context.Background(), created.ID, "A", "new@example.com")
		if !domain.IsValidation(err) {
			t.Errorf("expected validation error, got %v", err)
		}
	})

	t.Run("invalid email format", func(t *testing.T) {
		_, err := svc.UpdateUser(context.Background(), created.ID, "New", "not-an-email")
		if !domain.IsValidation(err) {
			t.Errorf("expected validation error, got %v", err)
		}
	})

	t.Run("whitespace email", func(t *testing.T) {
		_, err := svc.UpdateUser(context.Background(), created.ID, "New", "   ")
		if !domain.IsValidation(err) {
			t.Errorf("expected validation error, got %v", err)
		}
	})

	t.Run("repo update error", func(t *testing.T) {
		repo.updateErr = errors.New("db error")
		_, err := svc.UpdateUser(context.Background(), created.ID, "New", "new@example.com")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		repo.updateErr = nil
	})

	t.Run("not found", func(t *testing.T) {
		_, err := svc.UpdateUser(context.Background(), 9999, "Xi", "x@example.com")
		if !domain.IsNotFound(err) {
			t.Errorf("expected not found error, got %v", err)
		}
	})
}

func TestDeleteUser(t *testing.T) {
	repo := newMockRepo()
	svc := NewUserService(repo)

	created, _ := svc.CreateUser(context.Background(), "Del", "del@example.com")

	t.Run("success", func(t *testing.T) {
		err := svc.DeleteUser(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err = svc.GetUser(context.Background(), created.ID)
		if !domain.IsNotFound(err) {
			t.Errorf("expected not found after delete, got %v", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		err := svc.DeleteUser(context.Background(), 9999)
		if !domain.IsNotFound(err) {
			t.Errorf("expected not found error, got %v", err)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repo.deleteErr = errors.New("db error")
		err := svc.DeleteUser(context.Background(), created.ID)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		repo.deleteErr = nil
	})
}

func TestCreateUser_TrimsWhitespace(t *testing.T) {
	repo := newMockRepo()
	svc := NewUserService(repo)

	user, err := svc.CreateUser(context.Background(), "  Alice  ", "  alice@example.com  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Name != "Alice" {
		t.Errorf("name = %q; want %q", user.Name, "Alice")
	}
	if user.Email != "alice@example.com" {
		t.Errorf("email = %q; want %q", user.Email, "alice@example.com")
	}
}

func TestUpdateUser_TrimsWhitespace(t *testing.T) {
	repo := newMockRepo()
	svc := NewUserService(repo)

	created, _ := svc.CreateUser(context.Background(), "Old", "old@example.com")

	updated, err := svc.UpdateUser(context.Background(), created.ID, "  New  ", "  new@example.com  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Name != "New" {
		t.Errorf("name = %q; want %q", updated.Name, "New")
	}
	if updated.Email != "new@example.com" {
		t.Errorf("email = %q; want %q", updated.Email, "new@example.com")
	}
}
