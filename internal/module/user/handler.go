package user

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/simp-lee/gobase/internal/domain"
	"github.com/simp-lee/gobase/internal/pkg"
)

// UserHandler handles REST API requests for the user resource.
type UserHandler struct {
	svc domain.UserService
}

// NewUserHandler creates a new UserHandler with the given service.
func NewUserHandler(svc domain.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

// Create handles POST /api/v1/users.
func (h *UserHandler) Create(c *gin.Context) {
	var req CreateUserRequest
	if !pkg.BindAndValidate(c, &req) {
		return
	}

	user, err := h.svc.CreateUser(c.Request.Context(), req.Name, req.Email)
	if err != nil {
		pkg.Error(c, err)
		return
	}

	c.JSON(http.StatusCreated, pkg.Response{
		Code:    http.StatusCreated,
		Message: "success",
		Data:    user,
	})
}

// Get handles GET /api/v1/users/:id.
func (h *UserHandler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		pkg.Error(c, domain.NewAppError(domain.CodeValidation, err.Error(), nil))
		return
	}

	user, err := h.svc.GetUser(c.Request.Context(), id)
	if err != nil {
		pkg.Error(c, err)
		return
	}

	pkg.Success(c, user)
}

// List handles GET /api/v1/users.
func (h *UserHandler) List(c *gin.Context) {
	req := pkg.ParsePageRequest(c)

	result, err := h.svc.ListUsers(c.Request.Context(), req)
	if err != nil {
		pkg.Error(c, err)
		return
	}

	pkg.List(c, result)
}

// Update handles PUT /api/v1/users/:id.
func (h *UserHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		pkg.Error(c, domain.NewAppError(domain.CodeValidation, err.Error(), nil))
		return
	}

	var req UpdateUserRequest
	if !pkg.BindAndValidate(c, &req) {
		return
	}

	user, err := h.svc.UpdateUser(c.Request.Context(), id, req.Name, req.Email)
	if err != nil {
		pkg.Error(c, err)
		return
	}

	pkg.Success(c, user)
}

// Delete handles DELETE /api/v1/users/:id.
func (h *UserHandler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		pkg.Error(c, domain.NewAppError(domain.CodeValidation, err.Error(), nil))
		return
	}

	if err := h.svc.DeleteUser(c.Request.Context(), id); err != nil {
		pkg.Error(c, err)
		return
	}

	pkg.Success(c, nil)
}
