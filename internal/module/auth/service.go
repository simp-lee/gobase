package auth

import (
	"context"
	"errors"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"github.com/simp-lee/jwt"

	"github.com/simp-lee/gobase/internal/domain"
)

// Service defines the authentication operations.
type Service interface {
	Login(ctx context.Context, email, password string) (*TokenResponse, error)
	Register(ctx context.Context, username, email, password string) (*domain.User, error)
	Logout(ctx context.Context, token string) error
	RefreshToken(ctx context.Context, oldToken string) (*TokenResponse, error)
}

// authService implements Service.
type authService struct {
	jwtSvc      jwt.Service
	userRepo    domain.UserRepository
	tokenExpiry time.Duration
}

var _ Service = (*authService)(nil)

// NewService creates a new auth Service.
func NewService(jwtSvc jwt.Service, userRepo domain.UserRepository, tokenExpiry time.Duration) Service {
	return &authService{
		jwtSvc:      jwtSvc,
		userRepo:    userRepo,
		tokenExpiry: tokenExpiry,
	}
}

// dummyHash is a pre-computed bcrypt hash (cost 10) used to perform a constant-time
// dummy comparison when the user is not found, preventing timing side-channel attacks
// that could reveal whether an email is registered.
// Generated via: bcrypt.GenerateFromPassword([]byte("timing-safe-dummy"), bcrypt.DefaultCost)
var dummyHash = []byte("$2a$10$PTvOjdO/sIXsLrkc0hwSmuCvcW1JPRkbKUyNj0e1DyINAUnSFnrVC")

var bcryptCompareHashAndPassword = bcrypt.CompareHashAndPassword

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Login authenticates a user by email and password and returns a JWT token.
func (s *authService) Login(ctx context.Context, email, password string) (*TokenResponse, error) {
	email = normalizeEmail(email)
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if domain.IsNotFound(err) {
			// Perform a dummy bcrypt comparison to eliminate timing differences,
			// preventing user enumeration via response-time analysis.
			bcryptCompareHashAndPassword(dummyHash, []byte(password)) //nolint:errcheck
			return nil, domain.ErrUnauthorized
		}
		return nil, err
	}

	if err := bcryptCompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, domain.ErrUnauthorized
	}

	switch user.Status {
	case domain.StatusDisabled:
		return nil, domain.NewAppError(domain.CodeForbidden, "your account has been disabled", nil)
	case domain.StatusPending:
		return nil, domain.NewAppError(domain.CodeForbidden, "your account is pending activation", nil)
	}

	role := strings.TrimSpace(user.Role)
	if role == "" {
		role = domain.RoleUser
	}

	token, err := s.jwtSvc.GenerateToken(
		strconv.FormatUint(uint64(user.ID), 10),
		[]string{role},
		s.tokenExpiry,
	)
	if err != nil {
		return nil, domain.NewAppError(domain.CodeInternal, "failed to generate token", err)
	}

	parsedToken, parseErr := s.jwtSvc.ParseToken(token)
	if parseErr != nil {
		return nil, domain.NewAppError(domain.CodeInternal, "failed to parse generated token", parseErr)
	}

	return &TokenResponse{
		Token:     token,
		ExpiresAt: parsedToken.ExpiresAt.Unix(),
	}, nil
}

// validateRegisterInput validates registration input. username and email are expected
// to be pre-trimmed by callers; TrimSpace here ensures the validator is self-contained.
func validateRegisterInput(username, email, password string) error {
	nameLen := utf8.RuneCountInString(strings.TrimSpace(username))
	if nameLen == 0 {
		return domain.NewAppError(domain.CodeValidation, "username is required", nil)
	}
	if nameLen > 100 {
		return domain.NewAppError(domain.CodeValidation, "username must not exceed 100 characters", nil)
	}
	trimmedEmail := normalizeEmail(email)
	if len(trimmedEmail) == 0 {
		return domain.NewAppError(domain.CodeValidation, "email is required", nil)
	}
	addr, err := mail.ParseAddress(trimmedEmail)
	if err != nil || addr.Name != "" || addr.Address != trimmedEmail {
		return domain.NewAppError(domain.CodeValidation, "email must be a valid email address", nil)
	}
	if len(password) < 8 {
		return domain.NewAppError(domain.CodeValidation, "password must be at least 8 characters", nil)
	}
	if len(password) > 72 {
		return domain.NewAppError(domain.CodeValidation, "password must not exceed 72 characters", nil)
	}
	return nil
}

// Register creates a new user with the given credentials.
func (s *authService) Register(ctx context.Context, username, email, password string) (*domain.User, error) {
	username = strings.TrimSpace(username)
	email = normalizeEmail(email)
	if err := validateRegisterInput(username, email, password); err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, domain.NewAppError(domain.CodeInternal, "failed to hash password", err)
	}

	user := domain.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
		Role:         domain.RoleUser,
		Status:       domain.StatusActive,
	}

	if err := s.userRepo.Create(ctx, &user); err != nil {
		return nil, err
	}

	return &user, nil
}

// Logout revokes the given JWT token so it can no longer be used.
func (s *authService) Logout(_ context.Context, token string) error {
	if err := s.jwtSvc.RevokeToken(token); err != nil {
		if isJWTUnauthorizedError(err) {
			return domain.ErrUnauthorized
		}
		return domain.NewAppError(domain.CodeInternal, "failed to revoke token", err)
	}
	return nil
}

// RefreshToken exchanges the old JWT for a new one with a fresh expiry.
func (s *authService) RefreshToken(_ context.Context, oldToken string) (*TokenResponse, error) {
	newToken, err := s.jwtSvc.RefreshToken(oldToken)
	if err != nil {
		if isJWTUnauthorizedError(err) {
			return nil, domain.ErrUnauthorized
		}
		return nil, domain.NewAppError(domain.CodeInternal, "failed to refresh token", err)
	}

	parsed, err := s.jwtSvc.ParseToken(newToken)
	if err != nil {
		return nil, domain.NewAppError(domain.CodeInternal, "failed to parse refreshed token", err)
	}

	return &TokenResponse{
		Token:     newToken,
		ExpiresAt: parsed.ExpiresAt.Unix(),
	}, nil
}

// isJWTUnauthorizedError returns true for jwt errors that map to 401 Unauthorized.
func isJWTUnauthorizedError(err error) bool {
	return errors.Is(err, jwt.ErrInvalidToken) ||
		errors.Is(err, jwt.ErrExpiredToken) ||
		errors.Is(err, jwt.ErrRevokedToken)
}
