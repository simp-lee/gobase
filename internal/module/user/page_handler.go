package user

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/simp-lee/gobase/internal/domain"
	"github.com/simp-lee/gobase/internal/middleware"
	"github.com/simp-lee/gobase/internal/pkg"
)

// UserPageHandler handles page rendering and htmx endpoints for the user module.
type UserPageHandler struct {
	svc domain.UserService
}

// NewUserPageHandler creates a new UserPageHandler with the given service.
func NewUserPageHandler(svc domain.UserService) *UserPageHandler {
	return &UserPageHandler{svc: svc}
}

// ListPage renders the user list page with pagination.
// GET /users
func (h *UserPageHandler) ListPage(c *gin.Context) {
	req := pkg.ParsePageRequest(c)

	result, err := h.svc.ListUsers(c.Request.Context(), req)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "errors/500.html", gin.H{})
		return
	}

	c.HTML(http.StatusOK, "user/list.html", gin.H{
		"Users":      result.Items,
		"Pagination": result,
		"BaseURL":    "/users",
		"CSRFToken":  middleware.GetCSRFToken(c),
	})
}

// NewPage renders the new user form.
// GET /users/new
func (h *UserPageHandler) NewPage(c *gin.Context) {
	c.HTML(http.StatusOK, "user/form.html", gin.H{
		"IsEdit":    false,
		"CSRFToken": middleware.GetCSRFToken(c),
	})
}

// EditPage renders the edit user form.
// GET /users/:id/edit
func (h *UserPageHandler) EditPage(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.HTML(http.StatusBadRequest, "errors/400.html", gin.H{})
		return
	}

	user, err := h.svc.GetUser(c.Request.Context(), id)
	if err != nil {
		if domain.IsNotFound(err) {
			c.HTML(http.StatusNotFound, "errors/404.html", gin.H{})
			return
		}
		c.HTML(http.StatusInternalServerError, "errors/500.html", gin.H{})
		return
	}

	c.HTML(http.StatusOK, "user/form.html", gin.H{
		"User":      user,
		"IsEdit":    true,
		"CSRFToken": middleware.GetCSRFToken(c),
	})
}

// CreateHTMX handles user creation via htmx form submission.
// POST /users
func (h *UserPageHandler) CreateHTMX(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBind(&req); err != nil {
		slog.Debug("create user: bind error", "error", err)
		c.HTML(http.StatusOK, "user/form.html", gin.H{
			"IsEdit":    false,
			"Error":     "请检查输入格式",
			"CSRFToken": middleware.GetCSRFToken(c),
		})
		return
	}

	_, err := h.svc.CreateUser(c.Request.Context(), req.Name, req.Email)
	if err != nil {
		c.HTML(http.StatusOK, "user/form.html", gin.H{
			"IsEdit":    false,
			"Error":     safePageErrorMessage(err, "创建用户失败，请稍后重试"),
			"CSRFToken": middleware.GetCSRFToken(c),
		})
		return
	}

	setShowToastHeader(c, "用户创建成功", "success")
	c.Header("HX-Redirect", "/users")
	c.Status(http.StatusOK)
}

// UpdateHTMX handles user update via htmx form submission.
// PUT /users/:id
func (h *UserPageHandler) UpdateHTMX(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.HTML(http.StatusBadRequest, "errors/400.html", gin.H{})
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBind(&req); err != nil {
		slog.Debug("update user: bind error", "error", err, "id", id)
		user, getErr := h.svc.GetUser(c.Request.Context(), id)
		if getErr != nil {
			if domain.IsNotFound(getErr) {
				c.HTML(http.StatusNotFound, "errors/404.html", gin.H{})
				return
			}
			c.HTML(http.StatusInternalServerError, "errors/500.html", gin.H{})
			return
		}
		c.HTML(http.StatusOK, "user/form.html", gin.H{
			"User":      user,
			"IsEdit":    true,
			"Error":     "请检查输入格式",
			"CSRFToken": middleware.GetCSRFToken(c),
		})
		return
	}

	_, err = h.svc.UpdateUser(c.Request.Context(), id, req.Name, req.Email)
	if err != nil {
		user, getErr := h.svc.GetUser(c.Request.Context(), id)
		if getErr != nil {
			if domain.IsNotFound(getErr) {
				c.HTML(http.StatusNotFound, "errors/404.html", gin.H{})
				return
			}
			c.HTML(http.StatusInternalServerError, "errors/500.html", gin.H{})
			return
		}
		c.HTML(http.StatusOK, "user/form.html", gin.H{
			"User":      user,
			"IsEdit":    true,
			"Error":     safePageErrorMessage(err, "更新用户失败，请稍后重试"),
			"CSRFToken": middleware.GetCSRFToken(c),
		})
		return
	}

	setShowToastHeader(c, "用户更新成功", "success")
	c.Header("HX-Redirect", "/users")
	c.Status(http.StatusOK)
}

// DeleteHTMX handles user deletion via htmx.
// DELETE /users/:id
func (h *UserPageHandler) DeleteHTMX(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.Header("HX-Reswap", "none")
		setShowToastHeader(c, "无效的用户ID", "error")
		c.Status(http.StatusOK)
		return
	}

	if err := h.svc.DeleteUser(c.Request.Context(), id); err != nil {
		if domain.IsNotFound(err) {
			c.Header("HX-Reswap", "none")
			setShowToastHeader(c, "用户不存在或已删除", "error")
			c.Status(http.StatusOK)
			return
		}
		c.Header("HX-Reswap", "none")
		setShowToastHeader(c, "删除失败，请稍后重试", "error")
		c.Status(http.StatusOK)
		return
	}

	setShowToastHeader(c, "用户删除成功", "success")
	c.Status(http.StatusOK)
}

// parseID extracts and validates the "id" URL parameter.
func parseID(c *gin.Context) (uint, error) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("invalid id: %s", idStr)
	}
	if id > uint64(^uint(0)) {
		return 0, fmt.Errorf("invalid id: %s", idStr)
	}
	return uint(id), nil
}

// setShowToastHeader sets the HX-Trigger response header with a showToast event.
func setShowToastHeader(c *gin.Context, message, toastType string) {
	trigger, _ := json.Marshal(map[string]any{
		"showToast": map[string]string{
			"message": message,
			"type":    toastType,
		},
	})
	c.Header("HX-Trigger", string(trigger))
}

// safePageErrorMessage extracts a user-safe error message from an AppError.
// Only messages from user-facing error codes (NotFound, AlreadyExists, Validation)
// are returned. Internal or unknown error codes always return the fallback to
// prevent leaking technical details to end users.
func safePageErrorMessage(err error, fallback string) string {
	var appErr *domain.AppError
	if errors.As(err, &appErr) && appErr.Message != "" {
		switch appErr.Code {
		case domain.CodeNotFound, domain.CodeAlreadyExists, domain.CodeValidation:
			return appErr.Message
		}
	}
	return fallback
}
