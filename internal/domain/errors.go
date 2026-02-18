package domain

import (
	"errors"
	"net/http"
)

// Error codes for business logic errors.
const (
	CodeNotFound      = 1
	CodeAlreadyExists = 2
	CodeValidation    = 3
	CodeInternal      = 4
)

// AppError represents a business logic error with a code, message, and optional wrapped error.
type AppError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

// Unwrap returns the wrapped error for use with errors.Is and errors.As.
func (e *AppError) Unwrap() error {
	return e.Err
}

// Predefined business errors.
//
// To check whether an error matches one of these categories, use the
// corresponding helper function (IsNotFound, IsAlreadyExists, etc.)
// instead of errors.Is. The helpers use errors.As with error-code
// comparison, so they correctly match any *AppError that carries the
// same code — including freshly constructed instances from NewAppError
// and wrapped errors — whereas errors.Is only matches by pointer
// identity with the specific sentinel below.
var (
	ErrNotFound      = &AppError{Code: CodeNotFound, Message: "not found"}
	ErrAlreadyExists = &AppError{Code: CodeAlreadyExists, Message: "already exists"}
	ErrValidation    = &AppError{Code: CodeValidation, Message: "validation error"}
	ErrInternal      = &AppError{Code: CodeInternal, Message: "internal error"}
)

// NewAppError creates a new AppError with the given code, message, and wrapped error.
func NewAppError(code int, message string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// IsNotFound reports whether err is or wraps an AppError with CodeNotFound.
func IsNotFound(err error) bool {
	return hasCode(err, CodeNotFound)
}

// IsAlreadyExists reports whether err is or wraps an AppError with CodeAlreadyExists.
func IsAlreadyExists(err error) bool {
	return hasCode(err, CodeAlreadyExists)
}

// IsValidation reports whether err is or wraps an AppError with CodeValidation.
func IsValidation(err error) bool {
	return hasCode(err, CodeValidation)
}

// IsInternal reports whether err is or wraps an AppError with CodeInternal.
func IsInternal(err error) bool {
	return hasCode(err, CodeInternal)
}

// hasCode checks whether err is or wraps an *AppError with the given code.
func hasCode(err error, code int) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code == code
	}
	return false
}

// HTTPStatusCode maps an error to an HTTP status code.
// If the error is an *AppError, the code is mapped; otherwise http.StatusInternalServerError is returned.
func HTTPStatusCode(err error) int {
	var appErr *AppError
	if err != nil && errors.As(err, &appErr) {
		switch appErr.Code {
		case CodeNotFound:
			return http.StatusNotFound
		case CodeAlreadyExists:
			return http.StatusConflict
		case CodeValidation:
			return http.StatusBadRequest
		case CodeInternal:
			return http.StatusInternalServerError
		}
	}
	return http.StatusInternalServerError
}
