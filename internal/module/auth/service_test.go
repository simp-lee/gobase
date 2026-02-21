package auth

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/simp-lee/jwt"
	"github.com/simp-lee/pagination"

	"github.com/simp-lee/gobase/internal/domain"
)

// --- fakes ---

// fakeJWTService implements jwt.Service for testing.
type fakeJWTService struct {
	token       string
	err         error
	parsedToken *jwt.Token
	parseErr    error
}

func (f *fakeJWTService) GenerateToken(_ string, _ []string, _ time.Duration) (string, error) {
	return f.token, f.err
}
func (f *fakeJWTService) ValidateToken(string) (*jwt.Token, error)                 { return nil, nil }
func (f *fakeJWTService) ValidateAndParse(string) (*jwt.Token, error)              { return nil, nil }
func (f *fakeJWTService) RefreshToken(string) (string, error)                      { return "", nil }
func (f *fakeJWTService) RefreshTokenExtend(string, time.Duration) (string, error) { return "", nil }
func (f *fakeJWTService) RevokeToken(string) error                                 { return nil }
func (f *fakeJWTService) IsTokenRevoked(string) bool                               { return false }
func (f *fakeJWTService) ParseToken(string) (*jwt.Token, error) {
	if f.parseErr != nil {
		return nil, f.parseErr
	}
	if f.parsedToken != nil {
		return f.parsedToken, nil
	}
	return &jwt.Token{ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (f *fakeJWTService) RevokeAllUserTokens(string) error { return nil }
func (f *fakeJWTService) Close()                           {}

// capturingJWTService captures args passed to GenerateToken.
type capturingJWTService struct {
	fakeJWTService
	token          string
	capturedUserID string
	capturedRoles  []string
}

func (c *capturingJWTService) GenerateToken(userID string, roles []string, _ time.Duration) (string, error) {
	c.capturedUserID = userID
	c.capturedRoles = roles
	return c.token, nil
}

// fakeUserRepo implements domain.UserRepository for testing.
type fakeUserRepo struct {
	user      *domain.User
	getErr    error
	createErr error
}

func (f *fakeUserRepo) Create(_ context.Context, u *domain.User) error {
	if f.createErr != nil {
		return f.createErr
	}
	u.ID = 1
	return nil
}
func (f *fakeUserRepo) GetByEmail(_ context.Context, _ string) (*domain.User, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.user, nil
}
func (f *fakeUserRepo) GetByID(context.Context, uint) (*domain.User, error) { return nil, nil }
func (f *fakeUserRepo) List(context.Context, domain.PageRequest) (*pagination.Pagination[domain.User], error) {
	return nil, nil
}
func (f *fakeUserRepo) Update(context.Context, *domain.User) error { return nil }
func (f *fakeUserRepo) Delete(context.Context, uint) error         { return nil }

// --- helpers ---

func hashPassword(t *testing.T, pw string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return string(h)
}

// --- Login tests ---

func TestLogin_Success(t *testing.T) {
	pw := "secret1234"
	user := &domain.User{Name: "Alice", Email: "alice@example.com", PasswordHash: hashPassword(t, pw)}
	user.ID = 42

	svc := NewService(
		&fakeJWTService{token: "jwt-token-abc"},
		&fakeUserRepo{user: user},
		time.Hour,
	)

	resp, err := svc.Login(context.Background(), "alice@example.com", pw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Token != "jwt-token-abc" {
		t.Errorf("token = %q; want %q", resp.Token, "jwt-token-abc")
	}
	if resp.ExpiresAt == 0 {
		t.Error("ExpiresAt should be non-zero")
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	svc := NewService(
		&fakeJWTService{},
		&fakeUserRepo{getErr: domain.ErrNotFound},
		time.Hour,
	)

	_, err := svc.Login(context.Background(), "nobody@example.com", "password")
	if !domain.IsUnauthorized(err) {
		t.Errorf("expected unauthorized error, got: %v", err)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	user := &domain.User{Name: "Alice", Email: "alice@example.com", PasswordHash: hashPassword(t, "correct")}
	user.ID = 1

	svc := NewService(
		&fakeJWTService{},
		&fakeUserRepo{user: user},
		time.Hour,
	)

	_, err := svc.Login(context.Background(), "alice@example.com", "wrong")
	if !domain.IsUnauthorized(err) {
		t.Errorf("expected unauthorized error, got: %v", err)
	}
}

func TestLogin_JWTError(t *testing.T) {
	pw := "secret1234"
	user := &domain.User{Name: "Alice", Email: "alice@example.com", PasswordHash: hashPassword(t, pw)}
	user.ID = 1

	svc := NewService(
		&fakeJWTService{err: errors.New("jwt broken")},
		&fakeUserRepo{user: user},
		time.Hour,
	)

	_, err := svc.Login(context.Background(), "alice@example.com", pw)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestLogin_GenerateTokenReceivesCorrectArgs(t *testing.T) {
	pw := "secret1234"
	user := &domain.User{Name: "Bob", Email: "bob@example.com", PasswordHash: hashPassword(t, pw)}
	user.ID = 99

	fake := &capturingJWTService{token: "tok"}
	svc := NewService(fake, &fakeUserRepo{user: user}, time.Hour)

	_, err := svc.Login(context.Background(), "bob@example.com", pw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := strconv.FormatUint(uint64(user.ID), 10)
	if fake.capturedUserID != want {
		t.Errorf("userID passed to GenerateToken = %q; want %q", fake.capturedUserID, want)
	}
	if fake.capturedRoles != nil {
		t.Errorf("roles passed to GenerateToken = %v; want nil", fake.capturedRoles)
	}
}

func TestLogin_ParseTokenError(t *testing.T) {
	pw := "secret1234"
	user := &domain.User{Name: "Alice", Email: "alice@example.com", PasswordHash: hashPassword(t, pw)}
	user.ID = 1

	svc := NewService(
		&fakeJWTService{token: "jwt-token", parseErr: errors.New("parse failed")},
		&fakeUserRepo{user: user},
		time.Hour,
	)

	_, err := svc.Login(context.Background(), "alice@example.com", pw)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var appErr *domain.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *domain.AppError, got %T", err)
	}
	if appErr.Code != domain.CodeInternal {
		t.Errorf("expected CodeInternal, got %v", appErr.Code)
	}
}

// --- Register tests ---

func TestRegister_Success(t *testing.T) {
	svc := NewService(
		&fakeJWTService{},
		&fakeUserRepo{},
		time.Hour,
	)

	user, err := svc.Register(context.Background(), "Alice", "alice@example.com", "password123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Name != "Alice" {
		t.Errorf("name = %q; want %q", user.Name, "Alice")
	}
	if user.Email != "alice@example.com" {
		t.Errorf("email = %q; want %q", user.Email, "alice@example.com")
	}
	if user.PasswordHash == "" {
		t.Error("PasswordHash should be set")
	}
	// Verify the hash is valid bcrypt
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("password123")); err != nil {
		t.Errorf("stored hash does not match password: %v", err)
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	svc := NewService(
		&fakeJWTService{},
		&fakeUserRepo{createErr: domain.ErrAlreadyExists},
		time.Hour,
	)

	_, err := svc.Register(context.Background(), "Alice", "alice@example.com", "password123")
	if !domain.IsAlreadyExists(err) {
		t.Errorf("expected already-exists error, got: %v", err)
	}
}

// --- validateRegisterInput tests ---

func TestValidateRegisterInput(t *testing.T) {
	tests := []struct {
		name     string
		inName   string
		email    string
		password string
		wantErr  bool
	}{
		{"valid input", "Alice", "alice@example.com", "password123", false},
		{"empty name", "", "alice@example.com", "password123", true},
		{"whitespace-only name", "  ", "alice@example.com", "password123", true},
		{"empty email", "Alice", "", "password123", true},
		{"invalid email format", "Alice", "notanemail", "password123", true},
		{"malformed email", "Alice", "a@", "password123", true},
		{"password too short", "Alice", "alice@example.com", "short", true},
		{"password exactly 8 chars", "Alice", "alice@example.com", "exactly8", false},
		{"password exceeds 72 chars", "Alice", "alice@example.com", strings.Repeat("A", 73), true},
		{"password exactly 72 chars", "Alice", "alice@example.com", strings.Repeat("A", 72), false},
		{"name exceeds 100 characters", strings.Repeat("A", 101), "alice@example.com", "password123", true},
		{"name exactly 100 characters", strings.Repeat("A", 100), "alice@example.com", "password123", false},
		{"display-name format rejected", "Alice", "Alice <alice@example.com>", "password123", true},
		{"angle-bracket format rejected", "Alice", "<alice@example.com>", "password123", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRegisterInput(tt.inName, tt.email, tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v, got err=%v", tt.wantErr, err)
			}
		})
	}
}
