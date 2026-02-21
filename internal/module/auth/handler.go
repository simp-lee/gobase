package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/simp-lee/gobase/internal/pkg"
)

// AuthHandler handles REST API requests for authentication.
type AuthHandler struct {
	svc Service
}

// NewHandler creates a new AuthHandler with the given service.
func NewHandler(svc Service) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// Login handles POST /api/v1/auth/login.
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if !pkg.BindAndValidate(c, &req) {
		return
	}

	tokenResp, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		pkg.Error(c, err)
		return
	}

	pkg.Success(c, tokenResp)
}

// Register handles POST /api/v1/auth/register.
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if !pkg.BindAndValidate(c, &req) {
		return
	}

	user, err := h.svc.Register(c.Request.Context(), req.Name, req.Email, req.Password)
	if err != nil {
		pkg.Error(c, err)
		return
	}

	c.JSON(http.StatusCreated, pkg.Response{
		Code:    http.StatusCreated,
		Message: "user registered successfully",
		Data: RegisterResponse{
			ID:        user.ID,
			Name:      user.Name,
			Email:     user.Email,
			CreatedAt: user.CreatedAt,
		},
	})
}
