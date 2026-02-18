package user

import (
	"context"
	"errors"
	"strings"

	"github.com/simp-lee/gobase/internal/domain"
	"github.com/simp-lee/gobase/internal/pkg"
	"gorm.io/gorm"
)

// Allowed fields for sorting and filtering in List queries.
var (
	allowedSortFields   = []string{"id", "name", "email", "created_at", "updated_at"}
	allowedFilterFields = []string{"name", "email"}
)

// userRepository implements domain.UserRepository using GORM.
type userRepository struct {
	db *gorm.DB
}

// NewUserRepository creates a new UserRepository backed by the given GORM database.
func NewUserRepository(db *gorm.DB) domain.UserRepository {
	return &userRepository{db: db}
}

// Create inserts a new user into the database.
func (r *userRepository) Create(ctx context.Context, user *domain.User) error {
	if err := r.db.WithContext(ctx).Create(user).Error; err != nil {
		return mapError(err)
	}
	return nil
}

// GetByID retrieves a user by its primary key.
func (r *userRepository) GetByID(ctx context.Context, id uint) (*domain.User, error) {
	var user domain.User
	if err := r.db.WithContext(ctx).First(&user, id).Error; err != nil {
		return nil, mapError(err)
	}
	return &user, nil
}

// List returns a paginated, sorted, and filtered list of users.
func (r *userRepository) List(ctx context.Context, req domain.PageRequest) (*domain.PageResult[domain.User], error) {
	var total int64
	base := r.db.WithContext(ctx).Model(&domain.User{}).
		Scopes(pkg.Filter(req, allowedFilterFields))

	if err := base.Count(&total).Error; err != nil {
		return nil, mapError(err)
	}

	var users []domain.User
	if err := base.Scopes(
		pkg.Paginate(req),
		pkg.Sort(req, allowedSortFields),
	).Find(&users).Error; err != nil {
		return nil, mapError(err)
	}

	return pkg.NewPageResult(users, total, req), nil
}

// Update saves changes to an existing user.
func (r *userRepository) Update(ctx context.Context, user *domain.User) error {
	if err := r.db.WithContext(ctx).Save(user).Error; err != nil {
		return mapError(err)
	}
	return nil
}

// Delete removes a user by ID.
func (r *userRepository) Delete(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&domain.User{}, id)
	if result.Error != nil {
		return mapError(result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// mapError converts GORM errors to domain errors.
func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNotFound
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) || isDuplicateKeyError(err) {
		return domain.NewAppError(domain.CodeAlreadyExists, "already exists", err)
	}
	return domain.NewAppError(domain.CodeInternal, "database error", err)
}

// isDuplicateKeyError detects unique constraint violations by examining the
// error message. This is needed because not all GORM dialectors translate
// driver-level errors to gorm.ErrDuplicatedKey (e.g. the pure-Go SQLite driver).
func isDuplicateKeyError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "duplicate entry")
}
