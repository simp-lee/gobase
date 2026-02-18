package domain

import (
	"errors"
	"net/http"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *AppError
		want string
	}{
		{
			name: "with wrapped error",
			err:  &AppError{Code: CodeNotFound, Message: "user not found", Err: errors.New("record not found")},
			want: "user not found: record not found",
		},
		{
			name: "without wrapped error",
			err:  &AppError{Code: CodeNotFound, Message: "user not found"},
			want: "user not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("Error() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestAppError_Unwrap(t *testing.T) {
	inner := errors.New("inner error")
	appErr := &AppError{Code: CodeInternal, Message: "something failed", Err: inner}

	if !errors.Is(appErr, inner) {
		t.Error("Unwrap() should allow errors.Is to find wrapped error")
	}

	appErr2 := &AppError{Code: CodeInternal, Message: "no wrap"}
	if appErr2.Unwrap() != nil {
		t.Error("Unwrap() should return nil when Err is nil")
	}
}

func TestNewAppError(t *testing.T) {
	inner := errors.New("db error")
	appErr := NewAppError(CodeInternal, "operation failed", inner)

	if appErr.Code != CodeInternal {
		t.Errorf("Code = %d; want %d", appErr.Code, CodeInternal)
	}
	if appErr.Message != "operation failed" {
		t.Errorf("Message = %q; want %q", appErr.Message, "operation failed")
	}
	if !errors.Is(appErr, inner) {
		t.Error("should wrap inner error")
	}
}

func TestPredefinedErrors(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		checkFn func(error) bool
		code    int
	}{
		{"ErrNotFound", ErrNotFound, IsNotFound, CodeNotFound},
		{"ErrAlreadyExists", ErrAlreadyExists, IsAlreadyExists, CodeAlreadyExists},
		{"ErrValidation", ErrValidation, IsValidation, CodeValidation},
		{"ErrInternal", ErrInternal, IsInternal, CodeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var appErr *AppError
			if !errors.As(tt.err, &appErr) {
				t.Fatal("should be *AppError")
			}
			if appErr.Code != tt.code {
				t.Errorf("Code = %d; want %d", appErr.Code, tt.code)
			}
			if !tt.checkFn(tt.err) {
				t.Errorf("check function should return true for %s", tt.name)
			}
		})
	}
}

func TestIsCheckers_WithWrappedErrors(t *testing.T) {
	wrapped := NewAppError(CodeNotFound, "user not found", ErrNotFound)
	if !IsNotFound(wrapped) {
		t.Error("IsNotFound should detect wrapped ErrNotFound")
	}
	if IsAlreadyExists(wrapped) {
		t.Error("IsAlreadyExists should return false for ErrNotFound")
	}
}

func TestIsCheckers_NonAppError(t *testing.T) {
	plainErr := errors.New("some error")
	if IsNotFound(plainErr) {
		t.Error("IsNotFound should return false for non-AppError")
	}
	if IsAlreadyExists(plainErr) {
		t.Error("IsAlreadyExists should return false for non-AppError")
	}
	if IsValidation(plainErr) {
		t.Error("IsValidation should return false for non-AppError")
	}
	if IsInternal(plainErr) {
		t.Error("IsInternal should return false for non-AppError")
	}
}

func TestHTTPStatusCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"not found", ErrNotFound, http.StatusNotFound},
		{"already exists", ErrAlreadyExists, http.StatusConflict},
		{"validation", ErrValidation, http.StatusBadRequest},
		{"internal", ErrInternal, http.StatusInternalServerError},
		{"custom not found", NewAppError(CodeNotFound, "custom", nil), http.StatusNotFound},
		{"unknown code", NewAppError(999, "unknown", nil), http.StatusInternalServerError},
		{"non-AppError", errors.New("plain"), http.StatusInternalServerError},
		{"nil error", nil, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HTTPStatusCode(tt.err)
			if got != tt.want {
				t.Errorf("HTTPStatusCode() = %d; want %d", got, tt.want)
			}
		})
	}
}
