package user

import (
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/simp-lee/gobase/internal/domain"
	"github.com/simp-lee/gobase/internal/middleware"
	"github.com/simp-lee/gobase/internal/pkg"
)

type userPaginationView struct {
	TotalPages       int
	HasPreviousPage  bool
	PreviousPage     int
	FirstPageInRange int
	FirstPage        int
	ItemsPerPage     int
	CurrentPage      int
	Pages            []int
	LastPageInRange  int
	LastPage         int
	HasNextPage      bool
	NextPage         int
}

const (
	userListPageTemplate     = "user/list.html"
	userListFragmentTemplate = "user/list_fragment.html"
	flashToastCookieName     = "gobase_flash_toast"
)

func toUserPaginationView(page *domain.PageResult[domain.User]) userPaginationView {
	if page == nil {
		return userPaginationView{}
	}

	currentPage := page.CurrentPage
	if currentPage < 1 {
		currentPage = 1
	}

	totalPages := page.TotalPages
	if totalPages < 1 {
		totalPages = 1
	}
	if currentPage > totalPages {
		currentPage = totalPages
	}

	const pageWindowRadius = 2
	firstInRange := currentPage - pageWindowRadius
	if firstInRange < 1 {
		firstInRange = 1
	}
	lastInRange := currentPage + pageWindowRadius
	if lastInRange > totalPages {
		lastInRange = totalPages
	}

	pages := make([]int, 0, lastInRange-firstInRange+1)
	for i := firstInRange; i <= lastInRange; i++ {
		pages = append(pages, i)
	}

	hasPrevious := currentPage > 1
	hasNext := currentPage < totalPages

	previousPage := currentPage - 1
	if previousPage < 1 {
		previousPage = 1
	}

	nextPage := currentPage + 1
	if nextPage > totalPages {
		nextPage = totalPages
	}

	return userPaginationView{
		TotalPages:       totalPages,
		HasPreviousPage:  hasPrevious,
		PreviousPage:     previousPage,
		FirstPageInRange: firstInRange,
		FirstPage:        1,
		ItemsPerPage:     page.PageSize,
		CurrentPage:      currentPage,
		Pages:            pages,
		LastPageInRange:  lastInRange,
		LastPage:         totalPages,
		HasNextPage:      hasNext,
		NextPage:         nextPage,
	}
}

// UserPageHandler handles page rendering and htmx endpoints for the user module.
type UserPageHandler struct {
	svc domain.UserService
}

// NewUserPageHandler creates a new UserPageHandler with the given service.
func NewUserPageHandler(svc domain.UserService) *UserPageHandler {
	return &UserPageHandler{svc: svc}
}

// statusOptions lists the selectable user statuses for page filters and forms.
var statusOptions = []string{domain.StatusActive, domain.StatusDisabled, domain.StatusPending}

func buildListQuery(req domain.PageRequest, overrides map[string]string) string {
	v := url.Values{}
	v.Set("page", strconv.Itoa(req.Page))
	v.Set("page_size", strconv.Itoa(req.PageSize))
	for key, val := range req.Filter {
		v.Set(key, val)
	}
	if req.Sort != "" {
		v.Set("sort", req.Sort)
	}
	for key, val := range overrides {
		if val == "" {
			v.Del(key)
			continue
		}
		v.Set(key, val)
	}
	return v.Encode()
}

func nextStatusSort(req domain.PageRequest) (nextSort string, currentDirection string) {
	switch req.Sort {
	case "status:asc":
		return "status:desc", "asc"
	case "status:desc":
		return "status:asc", "desc"
	default:
		return "status:asc", ""
	}
}

// buildFilterQuery returns a query-string suffix (e.g. "&status=active&sort=id%3Adesc")
// from the active filter and sort parameters so pagination links preserve context.
func buildFilterQuery(req domain.PageRequest) string {
	v := url.Values{}
	for key, val := range req.Filter {
		v.Set(key, val)
	}
	if req.Sort != "" {
		v.Set("sort", req.Sort)
	}
	if encoded := v.Encode(); encoded != "" {
		return "&" + encoded
	}
	return ""
}

func isHTMXRequest(c *gin.Context) bool {
	return c.GetHeader("HX-Request") == "true"
}

func (h *UserPageHandler) renderListPage(c *gin.Context, req domain.PageRequest, result *domain.PageResult[domain.User]) {
	nextStatusSortValue, statusSortDirection := nextStatusSort(req)
	flash := consumeFlashToast(c)
	if isHTMXRequest(c) {
		flash = nil
	}
	data := gin.H{
		"Users":         result.Items,
		"Pagination":    toUserPaginationView(result),
		"BaseURL":       "/users",
		"CSRFToken":     middleware.GetCSRFToken(c),
		"StatusFilter":  req.Filter["status"],
		"StatusOptions": statusOptions,
		"FilterQuery":   buildFilterQuery(req),
		"CurrentSort":   req.Sort,
		"PageSize":      req.PageSize,
		"StatusSortQuery": buildListQuery(req, map[string]string{
			"sort": nextStatusSortValue,
		}),
		"StatusSortDirection": statusSortDirection,
		"Flash":               flash,
	}

	if isHTMXRequest(c) {
		c.Header("HX-Push-Url", c.Request.URL.RequestURI())
		c.HTML(http.StatusOK, userListFragmentTemplate, data)
		return
	}

	c.HTML(http.StatusOK, userListPageTemplate, data)
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

	h.renderListPage(c, req, result)
}

// NewPage renders the new user form.
// GET /users/new
func (h *UserPageHandler) NewPage(c *gin.Context) {
	c.HTML(http.StatusOK, "user/form.html", gin.H{
		"IsEdit":        false,
		"CanEditRole":   isRequesterAdmin(c),
		"CanEditStatus": isRequesterAdmin(c),
		"CSRFToken":     middleware.GetCSRFToken(c),
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
		"User":          user,
		"IsEdit":        true,
		"CanEditRole":   isRequesterAdmin(c),
		"CanEditStatus": isRequesterAdmin(c),
		"CSRFToken":     middleware.GetCSRFToken(c),
	})
}

// CreateHTMX handles user creation via htmx form submission.
// POST /users
func (h *UserPageHandler) CreateHTMX(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBind(&req); err != nil {
		slog.Debug("create user: bind error", "error", err)
		c.HTML(http.StatusOK, "user/form.html", gin.H{
			"IsEdit":        false,
			"CanEditRole":   isRequesterAdmin(c),
			"CanEditStatus": isRequesterAdmin(c),
			"Error":         "请检查输入格式",
			"CSRFToken":     middleware.GetCSRFToken(c),
		})
		return
	}

	_, err := h.svc.CreateUser(c.Request.Context(), req.Username, req.Email)
	if err != nil {
		c.HTML(http.StatusOK, "user/form.html", gin.H{
			"IsEdit":        false,
			"CanEditRole":   isRequesterAdmin(c),
			"CanEditStatus": isRequesterAdmin(c),
			"Error":         safePageErrorMessage(err, "创建用户失败，请稍后重试"),
			"CSRFToken":     middleware.GetCSRFToken(c),
		})
		return
	}

	setFlashToast(c, "success", "用户创建成功")
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
			"User":          user,
			"IsEdit":        true,
			"CanEditRole":   isRequesterAdmin(c),
			"CanEditStatus": isRequesterAdmin(c),
			"Error":         "请检查输入格式",
			"CSRFToken":     middleware.GetCSRFToken(c),
		})
		return
	}

	ctx := withAdminFieldAuthorized(c.Request.Context(), isRequesterAdmin(c))

	_, err = h.svc.UpdateUser(ctx, id, req.Username, req.Email, req.Role, req.Status)
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
			"User":          user,
			"IsEdit":        true,
			"CanEditRole":   isRequesterAdmin(c),
			"CanEditStatus": isRequesterAdmin(c),
			"Error":         safePageErrorMessage(err, "更新用户失败，请稍后重试"),
			"CSRFToken":     middleware.GetCSRFToken(c),
		})
		return
	}

	setFlashToast(c, "success", "用户更新成功")
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

	c.Header("HX-Reswap", "delete")
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(renderToastOOB("用户删除成功", "success")))
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
	c.Header(
		"HX-Trigger",
		fmt.Sprintf(`{"showToast":{"message":%s,"type":%s}}`, strconv.QuoteToASCII(message), strconv.Quote(toastType)),
	)
}

func setShowToastAfterSettleHeader(c *gin.Context, message, toastType string) {
	c.Header(
		"HX-Trigger-After-Settle",
		fmt.Sprintf(`{"showToast":{"message":%s,"type":%s}}`, strconv.QuoteToASCII(message), strconv.Quote(toastType)),
	)
}

func renderToastOOB(message, toastType string) string {
	escapedMessage := html.EscapeString(message)
	toastClass := "bg-blue-500"

	switch toastType {
	case "success":
		toastClass = "bg-green-500"
	case "error":
		toastClass = "bg-red-500"
	}

	return fmt.Sprintf(
		`<div hx-swap-oob="beforeend:body"><div class="fixed top-4 right-4 z-[60] max-w-sm rounded-lg %s px-4 py-3 text-sm font-medium text-white shadow-lg" role="status" aria-live="polite" data-toast-type="%s">%s</div></div>`,
		toastClass,
		html.EscapeString(toastType),
		escapedMessage,
	)
}

func setFlashToast(c *gin.Context, toastType, message string) {
	value := toastType + ":" + url.QueryEscape(message)
	c.SetCookie(flashToastCookieName, value, 10, "/", "", false, true)
}

func consumeFlashToast(c *gin.Context) gin.H {
	raw, err := c.Cookie(flashToastCookieName)
	if err != nil || raw == "" {
		return nil
	}

	c.SetCookie(flashToastCookieName, "", -1, "/", "", false, true)

	toastType, encodedMessage, ok := strings.Cut(raw, ":")
	if !ok || toastType == "" || encodedMessage == "" {
		return nil
	}

	message, err := url.QueryUnescape(encodedMessage)
	if err != nil || message == "" {
		return nil
	}

	flash := gin.H{}
	switch toastType {
	case "success":
		flash["Success"] = message
	case "error":
		flash["Error"] = message
	case "info":
		flash["Info"] = message
	default:
		return nil
	}

	return flash
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
