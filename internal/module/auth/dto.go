package auth

import "time"

// LoginRequest represents the input for user login.
type LoginRequest struct {
	Email    string `json:"email" form:"email" binding:"required,email"`
	Password string `json:"password" form:"password" binding:"required,min=8"`
}

// RegisterRequest represents the input for user registration.
type RegisterRequest struct {
	Name     string `json:"name" form:"name" binding:"required,min=1,max=100"`
	Email    string `json:"email" form:"email" binding:"required,email"`
	Password string `json:"password" form:"password" binding:"required,min=8,max=72"`
}

// TokenResponse represents the authentication token returned after login or registration.
type TokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

// RegisterResponse represents the public user data returned after registration.
type RegisterResponse struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}
