package user

// CreateUserRequest represents the input for creating a new user.
type CreateUserRequest struct {
	Name  string `json:"name" form:"name" binding:"required,min=2,max=100"`
	Email string `json:"email" form:"email" binding:"required,email"`
}

// UpdateUserRequest represents the input for updating an existing user.
type UpdateUserRequest struct {
	Name  string `json:"name" form:"name" binding:"required,min=2,max=100"`
	Email string `json:"email" form:"email" binding:"required,email"`
}
