package domain

import (
	"context"
)

// User role constants.
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// User status constants.
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
	StatusPending  = "pending"
)

// User represents a user in the system.
type User struct {
	BaseModel
	Username     string `gorm:"size:100;not null" json:"username"`
	Email        string `gorm:"size:255;uniqueIndex;not null" json:"email"`
	PasswordHash string `gorm:"size:255" json:"-"`
	Role         string `gorm:"size:20;not null;default:'user';check:chk_user_role,role IN ('admin','user')" json:"role"`
	Status       string `gorm:"size:20;not null;default:'active';check:chk_user_status,status IN ('active','disabled','pending')" json:"status"`
}

// UserRepository defines the data access interface for users.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id uint) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	List(ctx context.Context, req PageRequest) (*PageResult[User], error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id uint) error
}

// UserService defines the business logic interface for users.
type UserService interface {
	CreateUser(ctx context.Context, username, email string) (*User, error)
	GetUser(ctx context.Context, id uint) (*User, error)
	ListUsers(ctx context.Context, req PageRequest) (*PageResult[User], error)
	UpdateUser(ctx context.Context, id uint, username, email, role, status string) (*User, error)
	DeleteUser(ctx context.Context, id uint) error
}
