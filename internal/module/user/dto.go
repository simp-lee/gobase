package user

// CreateUserRequest represents the input for creating a new user.
// Role is not accepted here — new users always get the default role ("user").
type CreateUserRequest struct {
	Username string `json:"username" form:"username" binding:"required,min=2,max=100"`
	Email    string `json:"email" form:"email" binding:"required,email"`
}

// UpdateUserRequest represents the input for updating an existing user.
type UpdateUserRequest struct {
	Username string `json:"username" form:"username" binding:"required,min=2,max=100"`
	Email    string `json:"email" form:"email" binding:"required,email"`
	Role     string `json:"role" form:"role" binding:"omitempty,oneof=admin user"`
	Status   string `json:"status" form:"status" binding:"omitempty,oneof=active disabled pending"`
}
