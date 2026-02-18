package pkg

import (
	"errors"
	"net/http"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/simp-lee/gobase/internal/domain"
)

// Response is the standard JSON envelope for API responses.
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// ValidationErrorResponse is the JSON envelope for validation error responses.
type ValidationErrorResponse struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Errors  map[string]string `json:"errors"`
}

// Success sends a 200 JSON response with the given data.
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    data,
	})
}

// Error sends a JSON error response. If err is a *domain.AppError, its code is
// mapped to the appropriate HTTP status; otherwise 500 is returned.
func Error(c *gin.Context, err error) {
	status := domain.HTTPStatusCode(err)

	var appErr *domain.AppError
	msg := "internal error"
	if errors.As(err, &appErr) {
		msg = appErr.Message
	}

	c.JSON(status, Response{
		Code:    status,
		Message: msg,
		Data:    nil,
	})
}

// List sends a 200 JSON response intended for paginated list results.
// result should typically be a PageResult[T] containing items and pagination metadata.
func List(c *gin.Context, result any) {
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    result,
	})
}

// ValidationError sends a 400 JSON response with per-field validation error details.
// It detects validator.ValidationErrors and extracts field-level messages.
func ValidationError(c *gin.Context, err error) {
	validationErrorWithType(c, err, nil)
}

// BindAndValidate binds the request body to obj and validates it.
// On failure it automatically sends a ValidationError response and returns false.
// Because obj is available, JSON struct tags are used for field names when possible.
// Usage in handlers:
//
//	if !pkg.BindAndValidate(c, &req) { return }
func BindAndValidate(c *gin.Context, obj any) bool {
	if err := c.ShouldBind(obj); err != nil {
		validationErrorWithType(c, err, obj)
		return false
	}
	return true
}

// validationErrorWithType sends a 400 validation error response.
// When obj is non-nil, it reflects on the struct to prefer JSON tag names.
func validationErrorWithType(c *gin.Context, err error, obj any) {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		// Not a validation error; send a generic bad request.
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
			Data:    nil,
		})
		return
	}

	// Build a struct-field → json-tag map when the concrete type is available.
	jsonTags := buildJSONTagMap(obj)

	fieldErrors := make(map[string]string, len(ve))
	for _, fe := range ve {
		name := fe.Field()
		if tag, ok := jsonTags[fe.StructField()]; ok {
			name = tag
		} else {
			name = strings.ToLower(name)
		}
		msg := fe.Tag()
		if fe.Param() != "" {
			msg += "=" + fe.Param()
		}
		fieldErrors[name] = msg
	}

	c.JSON(http.StatusBadRequest, ValidationErrorResponse{
		Code:    http.StatusBadRequest,
		Message: "validation error",
		Errors:  fieldErrors,
	})
}

// buildJSONTagMap returns a map from struct field name to its JSON tag name.
// If obj is nil or not a struct (pointer), it returns an empty map.
func buildJSONTagMap(obj any) map[string]string {
	if obj == nil {
		return nil
	}
	t := reflect.TypeOf(obj)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	m := make(map[string]string, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if name := parseJSONTagName(tag); name != "" {
			m[f.Name] = name
		}
	}
	return m
}

// parseJSONTagName extracts the field name from a JSON struct tag value.
func parseJSONTagName(tag string) string {
	if tag == "" || tag == "-" {
		return ""
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" || name == "-" {
		return ""
	}
	return name
}
