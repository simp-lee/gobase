package user

import (
	"context"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/simp-lee/gobase/internal/domain"
)

// userService implements domain.UserService.
type userService struct {
	repo domain.UserRepository
}

var _ domain.UserService = (*userService)(nil)

// NewUserService creates a new UserService with the given repository.
func NewUserService(repo domain.UserRepository) domain.UserService {
	return &userService{repo: repo}
}

// CreateUser validates input, builds a User with the default role, and persists it via the repository.
func (s *userService) CreateUser(ctx context.Context, username, email string) (*domain.User, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)

	if err := validateUsernameEmail(username, email); err != nil {
		return nil, err
	}

	user := &domain.User{
		Username: username,
		Email:    email,
		Role:     domain.RoleUser,
		Status:   domain.StatusActive,
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
func (s *userService) ListUsers(ctx context.Context, req domain.PageRequest) (*domain.PageResult[domain.User], error) {
	return s.repo.List(ctx, req)
}

// UpdateUser loads the existing user, applies changes, and persists them.
// If role or status is empty, it is left unchanged.
func (s *userService) UpdateUser(ctx context.Context, id uint, username, email, role, status string) (*domain.User, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	role = strings.TrimSpace(role)
	status = strings.TrimSpace(status)

	if err := validateUsernameEmail(username, email); err != nil {
		return nil, err
	}

	if role != "" {
		if err := validateRole(role); err != nil {
			return nil, err
		}
		if !isAdminFieldAuthorized(ctx) {
			role = "" // silently ignore for non-admin
		}
	}

	if status != "" {
		if err := validateStatus(status); err != nil {
			return nil, err
		}
		if !isAdminFieldAuthorized(ctx) {
			status = "" // silently ignore for non-admin
		}
	}

	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	user.Username = username
	user.Email = email
	if role != "" {
		user.Role = role
	}
	if status != "" {
		user.Status = status
	}

	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// DeleteUser removes a user by ID.
func (s *userService) DeleteUser(ctx context.Context, id uint) error {
	return s.repo.Delete(ctx, id)
}

// validateUsernameEmail checks that username and email are valid.
func validateUsernameEmail(username, email string) error {
	trimmedUsername := strings.TrimSpace(username)
	if trimmedUsername == "" {
		return domain.NewAppError(domain.CodeValidation, "username is required", nil)
	}
	if utf8.RuneCountInString(trimmedUsername) < 2 {
		return domain.NewAppError(domain.CodeValidation, "username must be at least 2 characters", nil)
	}
	if utf8.RuneCountInString(trimmedUsername) > 100 {
		return domain.NewAppError(domain.CodeValidation, "username must be at most 100 characters", nil)
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

// validateRole checks that role is a valid role value.
func validateRole(role string) error {
	switch role {
	case domain.RoleAdmin, domain.RoleUser:
		return nil
	default:
		return domain.NewAppError(domain.CodeValidation, "role must be 'admin' or 'user'", nil)
	}
}

func validateStatus(status string) error {
	switch status {
	case domain.StatusActive, domain.StatusDisabled, domain.StatusPending:
		return nil
	default:
		return domain.NewAppError(domain.CodeValidation, "status must be 'active', 'disabled', or 'pending'", nil)
	}
}
