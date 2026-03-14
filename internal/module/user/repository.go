package user

import (
	"context"

	"github.com/simp-lee/gobase/internal/domain"
	"github.com/simp-lee/gobase/internal/pkg"
	"gorm.io/gorm"
)

// Allowed fields for sorting and filtering in List queries.
var (
	allowedSortFields   = []string{"id", "username", "email", "status", "created_at", "updated_at"}
	allowedFilterFields = []string{"username", "email", "status"}
)

// userRepository implements domain.UserRepository using GORM.
type userRepository struct {
	db *gorm.DB
}

var _ domain.UserRepository = (*userRepository)(nil)

// NewUserRepository creates a new UserRepository backed by the given GORM database.
func NewUserRepository(db *gorm.DB) domain.UserRepository {
	return &userRepository{db: db}
}

// Create inserts a new user into the database.
func (r *userRepository) Create(ctx context.Context, user *domain.User) error {
	if err := r.db.WithContext(ctx).Create(user).Error; err != nil {
		return pkg.MapDBError(err)
	}
	return nil
}

// GetByID retrieves a user by its primary key.
func (r *userRepository) GetByID(ctx context.Context, id uint) (*domain.User, error) {
	var user domain.User
	if err := r.db.WithContext(ctx).First(&user, id).Error; err != nil {
		return nil, pkg.MapDBError(err)
	}
	return &user, nil
}

// GetByEmail retrieves a user by email address.
func (r *userRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		return nil, pkg.MapDBError(err)
	}
	return &user, nil
}

// List returns a paginated, sorted, and filtered list of users.
func (r *userRepository) List(ctx context.Context, req domain.PageRequest) (*domain.PageResult[domain.User], error) {
	result, err := pkg.PaginateGORM[domain.User](ctx, r.db.WithContext(ctx).Model(&domain.User{}), req, pkg.ListOptions{
		SortFields:   allowedSortFields,
		FilterFields: allowedFilterFields,
	})
	if err != nil {
		return nil, pkg.MapDBError(err)
	}
	return pkg.ToPageResult(result), nil
}

// Update saves changes to an existing user.
func (r *userRepository) Update(ctx context.Context, user *domain.User) error {
	if err := r.db.WithContext(ctx).Save(user).Error; err != nil {
		return pkg.MapDBError(err)
	}
	return nil
}

// Delete removes a user by ID.
func (r *userRepository) Delete(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&domain.User{}, id)
	if result.Error != nil {
		return pkg.MapDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
