package domain

import "context"

// User represents a user in the system.
type User struct {
	BaseModel
	Name  string `gorm:"size:100;not null" json:"name"`
	Email string `gorm:"size:255;uniqueIndex;not null" json:"email"`
}

// UserRepository defines the data access interface for users.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id uint) (*User, error)
	List(ctx context.Context, req PageRequest) (*PageResult[User], error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id uint) error
}

// UserService defines the business logic interface for users.
type UserService interface {
	CreateUser(ctx context.Context, name, email string) (*User, error)
	GetUser(ctx context.Context, id uint) (*User, error)
	ListUsers(ctx context.Context, req PageRequest) (*PageResult[User], error)
	UpdateUser(ctx context.Context, id uint, name, email string) (*User, error)
	DeleteUser(ctx context.Context, id uint) error
}
