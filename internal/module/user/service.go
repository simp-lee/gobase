package user

import (
	"context"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/simp-lee/gobase/internal/domain"
	"github.com/simp-lee/pagination"
)

// userService implements domain.UserService.
type userService struct {
	repo domain.UserRepository
}

// NewUserService creates a new UserService with the given repository.
func NewUserService(repo domain.UserRepository) domain.UserService {
	return &userService{repo: repo}
}

// CreateUser validates input, builds a User, and persists it via the repository.
func (s *userService) CreateUser(ctx context.Context, name, email string) (*domain.User, error) {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)

	if err := validateNameEmail(name, email); err != nil {
		return nil, err
	}

	user := &domain.User{
		Name:  name,
		Email: email,
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// GetUser retrieves a user by ID.
func (s *userService) GetUser(ctx context.Context, id uint) (*domain.User, error) {
	return s.repo.GetByID(ctx, id)
}

// ListUsers returns a paginated list of users.
func (s *userService) ListUsers(ctx context.Context, req domain.PageRequest) (*pagination.Pagination[domain.User], error) {
	return s.repo.List(ctx, req)
}

// UpdateUser loads the existing user, applies changes, and persists them.
func (s *userService) UpdateUser(ctx context.Context, id uint, name, email string) (*domain.User, error) {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)

	if err := validateNameEmail(name, email); err != nil {
		return nil, err
	}

	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	user.Name = name
	user.Email = email

	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// DeleteUser removes a user by ID.
func (s *userService) DeleteUser(ctx context.Context, id uint) error {
	return s.repo.Delete(ctx, id)
}

// validateNameEmail checks that name and email are non-empty.
func validateNameEmail(name, email string) error {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return domain.NewAppError(domain.CodeValidation, "name is required", nil)
	}
	if utf8.RuneCountInString(trimmedName) < 2 {
		return domain.NewAppError(domain.CodeValidation, "name must be at least 2 characters", nil)
	}
	if utf8.RuneCountInString(trimmedName) > 100 {
		return domain.NewAppError(domain.CodeValidation, "name must be at most 100 characters", nil)
	}

	trimmedEmail := strings.TrimSpace(email)
	if trimmedEmail == "" {
		return domain.NewAppError(domain.CodeValidation, "email is required", nil)
	}
	if _, err := mail.ParseAddress(trimmedEmail); err != nil {
		return domain.NewAppError(domain.CodeValidation, "email must be a valid email address", nil)
	}
	return nil
}
