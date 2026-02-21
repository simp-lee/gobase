package auth

import (
	"context"
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
	Register(ctx context.Context, name, email, password string) (*domain.User, error)
}

// authService implements Service.
type authService struct {
	jwtSvc      jwt.Service
	userRepo    domain.UserRepository
	tokenExpiry time.Duration
}

// NewService creates a new auth Service.
func NewService(jwtSvc jwt.Service, userRepo domain.UserRepository, tokenExpiry time.Duration) Service {
	return &authService{
		jwtSvc:      jwtSvc,
		userRepo:    userRepo,
		tokenExpiry: tokenExpiry,
	}
}

// Login authenticates a user by email and password and returns a JWT token.
func (s *authService) Login(ctx context.Context, email, password string) (*TokenResponse, error) {
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		// Don't reveal whether the user exists — always return unauthorized.
		if domain.IsNotFound(err) {
			return nil, domain.ErrUnauthorized
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, domain.ErrUnauthorized
	}

	token, err := s.jwtSvc.GenerateToken(
		strconv.FormatUint(uint64(user.ID), 10),
		nil, // roles — RBAC uses a separate service
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

// validateRegisterInput validates registration input. name and email are expected
// to be pre-trimmed by callers; TrimSpace here ensures the validator is self-contained.
func validateRegisterInput(name, email, password string) error {
	nameLen := utf8.RuneCountInString(strings.TrimSpace(name))
	if nameLen == 0 {
		return domain.NewAppError(domain.CodeValidation, "name is required", nil)
	}
	if nameLen > 100 {
		return domain.NewAppError(domain.CodeValidation, "name must not exceed 100 characters", nil)
	}
	trimmedEmail := strings.TrimSpace(email)
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
func (s *authService) Register(ctx context.Context, name, email, password string) (*domain.User, error) {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	if err := validateRegisterInput(name, email, password); err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, domain.NewAppError(domain.CodeInternal, "failed to hash password", err)
	}

	user := domain.User{
		Name:         name,
		Email:        email,
		PasswordHash: string(hash),
	}

	if err := s.userRepo.Create(ctx, &user); err != nil {
		return nil, err
	}

	return &user, nil
}
