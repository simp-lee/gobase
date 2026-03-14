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

	"github.com/simp-lee/gobase/internal/domain"
)

// --- fakes ---

// fakeJWTService implements jwt.Service for testing.
type fakeJWTService struct {
	token        string
	err          error
	parsedToken  *jwt.Token
	parseErr     error
	revokeErr    error
	refreshToken string
	refreshErr   error
}

func (f *fakeJWTService) GenerateToken(_ string, _ []string, _ time.Duration) (string, error) {
	return f.token, f.err
}
func (f *fakeJWTService) ValidateToken(string) (*jwt.Token, error)    { return nil, nil }
func (f *fakeJWTService) ValidateAndParse(string) (*jwt.Token, error) { return nil, nil }
func (f *fakeJWTService) RefreshToken(string) (string, error) {
	return f.refreshToken, f.refreshErr
}
func (f *fakeJWTService) RefreshTokenExtend(string, time.Duration) (string, error) { return "", nil }
func (f *fakeJWTService) RevokeToken(string) error                                 { return f.revokeErr }
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
	getByEmailFn func(ctx context.Context, email string) (*domain.User, error)
	user         *domain.User
	getErr       error
	createErr    error
}

func (f *fakeUserRepo) Create(_ context.Context, u *domain.User) error {
	if f.createErr != nil {
		return f.createErr
	}
	u.ID = 1
	return nil
}
func (f *fakeUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	if f.getByEmailFn != nil {
		return f.getByEmailFn(ctx, email)
	}
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.user, nil
}
func (f *fakeUserRepo) GetByID(context.Context, uint) (*domain.User, error) { return nil, nil }
func (f *fakeUserRepo) List(context.Context, domain.PageRequest) (*domain.PageResult[domain.User], error) {
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
	user := &domain.User{Username: "Alice", Email: "alice@example.com", PasswordHash: hashPassword(t, pw), Status: domain.StatusActive}
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
	user := &domain.User{Username: "Alice", Email: "alice@example.com", PasswordHash: hashPassword(t, "correct"), Status: domain.StatusActive}
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

func TestLogin_DisabledUser(t *testing.T) {
	pw := "secret1234"
	user := &domain.User{Username: "Alice", Email: "alice@example.com", PasswordHash: hashPassword(t, pw), Status: domain.StatusDisabled}
	user.ID = 1

	svc := NewService(
		&fakeJWTService{},
		&fakeUserRepo{user: user},
		time.Hour,
	)

	_, err := svc.Login(context.Background(), "alice@example.com", pw)
	if !domain.IsForbidden(err) {
		t.Errorf("expected forbidden error, got: %v", err)
	}
	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		if appErr.Message != "your account has been disabled" {
			t.Errorf("message = %q; want %q", appErr.Message, "your account has been disabled")
		}
	}
}

func TestLogin_PendingUser(t *testing.T) {
	pw := "secret1234"
	user := &domain.User{Username: "Alice", Email: "alice@example.com", PasswordHash: hashPassword(t, pw), Status: domain.StatusPending}
	user.ID = 1

	svc := NewService(
		&fakeJWTService{},
		&fakeUserRepo{user: user},
		time.Hour,
	)

	_, err := svc.Login(context.Background(), "alice@example.com", pw)
	if !domain.IsForbidden(err) {
		t.Errorf("expected forbidden error, got: %v", err)
	}
	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		if appErr.Message != "your account is pending activation" {
			t.Errorf("message = %q; want %q", appErr.Message, "your account is pending activation")
		}
	}
}

func TestLogin_JWTError(t *testing.T) {
	pw := "secret1234"
	user := &domain.User{Username: "Alice", Email: "alice@example.com", PasswordHash: hashPassword(t, pw)}
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
	user := &domain.User{Username: "Bob", Email: "bob@example.com", PasswordHash: hashPassword(t, pw), Role: domain.RoleAdmin}
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
	if len(fake.capturedRoles) != 1 || fake.capturedRoles[0] != domain.RoleAdmin {
		t.Errorf("roles passed to GenerateToken = %v; want [%q]", fake.capturedRoles, domain.RoleAdmin)
	}
}

func TestLogin_ParseTokenError(t *testing.T) {
	pw := "secret1234"
	user := &domain.User{Username: "Alice", Email: "alice@example.com", PasswordHash: hashPassword(t, pw)}
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
	if user.Username != "Alice" {
		t.Errorf("username = %q; want %q", user.Username, "Alice")
	}
	if user.Email != "alice@example.com" {
		t.Errorf("email = %q; want %q", user.Email, "alice@example.com")
	}
	if user.Role != domain.RoleUser {
		t.Errorf("role = %q; want %q", user.Role, domain.RoleUser)
	}
	if user.Status != domain.StatusActive {
		t.Errorf("status = %q; want %q", user.Status, domain.StatusActive)
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

// --- normalizeEmail tests ---

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Alice@Example.COM", "alice@example.com"},
		{"  bob@test.org  ", "bob@test.org"},
		{"  UPPER@CASE.IO  ", "upper@case.io"},
		{"already@lower.com", "already@lower.com"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeEmail(tt.input)
			if got != tt.want {
				t.Errorf("normalizeEmail(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLogin_NormalizesEmail(t *testing.T) {
	pw := "secret1234"
	user := &domain.User{Username: "Alice", Email: "alice@example.com", PasswordHash: hashPassword(t, pw)}
	user.ID = 42

	repo := &fakeUserRepo{user: user}
	svc := NewService(
		&fakeJWTService{token: "jwt-token-abc"},
		repo,
		time.Hour,
	)

	// Login with mixed-case email should succeed because it's normalized.
	resp, err := svc.Login(context.Background(), "  Alice@Example.COM  ", pw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Token != "jwt-token-abc" {
		t.Errorf("token = %q; want %q", resp.Token, "jwt-token-abc")
	}
}

func TestRegister_NormalizesEmail(t *testing.T) {
	svc := NewService(
		&fakeJWTService{},
		&fakeUserRepo{},
		time.Hour,
	)

	user, err := svc.Register(context.Background(), "Alice", "  Alice@Example.COM  ", "password123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Email != "alice@example.com" {
		t.Errorf("email = %q; want %q", user.Email, "alice@example.com")
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
		{"empty username", "", "alice@example.com", "password123", true},
		{"whitespace-only username", "  ", "alice@example.com", "password123", true},
		{"empty email", "Alice", "", "password123", true},
		{"invalid email format", "Alice", "notanemail", "password123", true},
		{"malformed email", "Alice", "a@", "password123", true},
		{"password too short", "Alice", "alice@example.com", "short", true},
		{"password exactly 8 chars", "Alice", "alice@example.com", "exactly8", false},
		{"password exceeds 72 chars", "Alice", "alice@example.com", strings.Repeat("A", 73), true},
		{"password exactly 72 chars", "Alice", "alice@example.com", strings.Repeat("A", 72), false},
		{"username exceeds 100 characters", strings.Repeat("A", 101), "alice@example.com", "password123", true},
		{"username exactly 100 characters", strings.Repeat("A", 100), "alice@example.com", "password123", false},
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

// --- Logout tests ---

func TestLogout_Success(t *testing.T) {
	svc := NewService(
		&fakeJWTService{},
		&fakeUserRepo{},
		time.Hour,
	)

	err := svc.Logout(context.Background(), "some-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLogout_InvalidToken(t *testing.T) {
	svc := NewService(
		&fakeJWTService{revokeErr: jwt.ErrInvalidToken},
		&fakeUserRepo{},
		time.Hour,
	)

	err := svc.Logout(context.Background(), "bad-token")
	if !domain.IsUnauthorized(err) {
		t.Errorf("expected unauthorized error, got: %v", err)
	}
}

func TestLogout_ExpiredToken(t *testing.T) {
	svc := NewService(
		&fakeJWTService{revokeErr: jwt.ErrExpiredToken},
		&fakeUserRepo{},
		time.Hour,
	)

	err := svc.Logout(context.Background(), "expired-token")
	if !domain.IsUnauthorized(err) {
		t.Errorf("expected unauthorized error, got: %v", err)
	}
}

func TestLogout_RevokedToken(t *testing.T) {
	svc := NewService(
		&fakeJWTService{revokeErr: jwt.ErrRevokedToken},
		&fakeUserRepo{},
		time.Hour,
	)

	err := svc.Logout(context.Background(), "revoked-token")
	if !domain.IsUnauthorized(err) {
		t.Errorf("expected unauthorized error, got: %v", err)
	}
}

func TestLogout_InternalError(t *testing.T) {
	svc := NewService(
		&fakeJWTService{revokeErr: errors.New("storage failure")},
		&fakeUserRepo{},
		time.Hour,
	)

	err := svc.Logout(context.Background(), "some-token")
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

// --- RefreshToken tests ---

func TestRefreshToken_Success(t *testing.T) {
	svc := NewService(
		&fakeJWTService{refreshToken: "new-jwt-token"},
		&fakeUserRepo{},
		time.Hour,
	)

	resp, err := svc.RefreshToken(context.Background(), "old-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Token != "new-jwt-token" {
		t.Errorf("token = %q; want %q", resp.Token, "new-jwt-token")
	}
	if resp.ExpiresAt == 0 {
		t.Error("ExpiresAt should be non-zero")
	}
}

func TestRefreshToken_InvalidToken(t *testing.T) {
	svc := NewService(
		&fakeJWTService{refreshErr: jwt.ErrInvalidToken},
		&fakeUserRepo{},
		time.Hour,
	)

	_, err := svc.RefreshToken(context.Background(), "bad-token")
	if !domain.IsUnauthorized(err) {
		t.Errorf("expected unauthorized error, got: %v", err)
	}
}

func TestRefreshToken_ExpiredToken(t *testing.T) {
	svc := NewService(
		&fakeJWTService{refreshErr: jwt.ErrExpiredToken},
		&fakeUserRepo{},
		time.Hour,
	)

	_, err := svc.RefreshToken(context.Background(), "expired-token")
	if !domain.IsUnauthorized(err) {
		t.Errorf("expected unauthorized error, got: %v", err)
	}
}

func TestRefreshToken_RevokedToken(t *testing.T) {
	svc := NewService(
		&fakeJWTService{refreshErr: jwt.ErrRevokedToken},
		&fakeUserRepo{},
		time.Hour,
	)

	_, err := svc.RefreshToken(context.Background(), "revoked-token")
	if !domain.IsUnauthorized(err) {
		t.Errorf("expected unauthorized error, got: %v", err)
	}
}

func TestRefreshToken_InternalError(t *testing.T) {
	svc := NewService(
		&fakeJWTService{refreshErr: errors.New("jwt broken")},
		&fakeUserRepo{},
		time.Hour,
	)

	_, err := svc.RefreshToken(context.Background(), "some-token")
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

func TestRefreshToken_ParseError(t *testing.T) {
	svc := NewService(
		&fakeJWTService{refreshToken: "new-token", parseErr: errors.New("parse failed")},
		&fakeUserRepo{},
		time.Hour,
	)

	_, err := svc.RefreshToken(context.Background(), "old-token")
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

// --- isJWTUnauthorizedError tests ---

func TestIsJWTUnauthorizedError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"ErrInvalidToken", jwt.ErrInvalidToken, true},
		{"ErrExpiredToken", jwt.ErrExpiredToken, true},
		{"ErrRevokedToken", jwt.ErrRevokedToken, true},
		{"generic error", errors.New("generic"), false},
		{"nil error", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isJWTUnauthorizedError(tt.err); got != tt.want {
				t.Errorf("isJWTUnauthorizedError() = %v; want %v", got, tt.want)
			}
		})
	}
}

// --- dummyHash tests ---

func TestDummyHash_IsValidBcrypt(t *testing.T) {
	// Verify the pre-computed dummyHash constant is a valid bcrypt hash.
	if err := bcrypt.CompareHashAndPassword(dummyHash, []byte("timing-safe-dummy")); err != nil {
		t.Errorf("dummyHash should match 'timing-safe-dummy': %v", err)
	}
}

func TestLogin_UserNotFound_ExecutesDummyBcryptCompare(t *testing.T) {
	originalCompare := bcryptCompareHashAndPassword
	t.Cleanup(func() {
		bcryptCompareHashAndPassword = originalCompare
	})

	called := 0
	bcryptCompareHashAndPassword = func(hashedPassword, password []byte) error {
		called++
		if string(hashedPassword) != string(dummyHash) {
			t.Fatalf("unexpected hash used for dummy compare")
		}
		if string(password) != "pw-for-timing" {
			t.Fatalf("unexpected password used in dummy compare: %q", string(password))
		}
		return nil
	}

	svc := NewService(
		&fakeJWTService{},
		&fakeUserRepo{getErr: domain.ErrNotFound},
		time.Hour,
	)

	_, err := svc.Login(context.Background(), "nobody@example.com", "pw-for-timing")
	if !domain.IsUnauthorized(err) {
		t.Fatalf("expected unauthorized error, got: %v", err)
	}
	if called != 1 {
		t.Fatalf("dummy compare call count = %d; want 1", called)
	}
}

// --- Login email normalization capture test ---

func TestLogin_NormalizesEmailBeforeLookup(t *testing.T) {
	pw := "secret1234"
	user := &domain.User{Username: "Alice", Email: "alice@example.com", PasswordHash: hashPassword(t, pw)}
	user.ID = 7

	var capturedEmail string
	svc := NewService(
		&fakeJWTService{token: "jwt-token"},
		&fakeUserRepo{getByEmailFn: func(_ context.Context, email string) (*domain.User, error) {
			capturedEmail = email
			return user, nil
		}},
		time.Hour,
	)

	_, err := svc.Login(context.Background(), "  Alice@Example.COM  ", pw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedEmail != "alice@example.com" {
		t.Errorf("lookup email = %q; want %q", capturedEmail, "alice@example.com")
	}
}
