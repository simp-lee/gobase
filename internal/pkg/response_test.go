package pkg

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/simp-lee/gobase/internal/domain"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// testInput is used to generate real validator.ValidationErrors.
type testInput struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
}

// newResponseTestContext creates a gin context backed by an httptest.ResponseRecorder.
func newResponseTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return c, w
}

// newResponseTestContextWithBody creates a gin context with a JSON request body.
func newResponseTestContextWithBody(body string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

// makeValidationErrors validates an empty testInput and returns the resulting
// validator.ValidationErrors.
func makeValidationErrors(t *testing.T) validator.ValidationErrors {
	t.Helper()
	validate := validator.New()
	err := validate.Struct(testInput{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		t.Fatalf("expected validator.ValidationErrors, got %T", err)
	}
	return ve
}

func TestSuccess(t *testing.T) {
	c, w := newResponseTestContext()

	data := map[string]string{"greeting": "hello"}
	Success(c, data)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Code != http.StatusOK {
		t.Errorf("expected code %d, got %d", http.StatusOK, resp.Code)
	}
	if resp.Message != "success" {
		t.Errorf("expected message %q, got %q", "success", resp.Message)
	}
	if resp.Data == nil {
		t.Error("expected non-nil data")
	}
}

func TestSuccess_NilData(t *testing.T) {
	c, w := newResponseTestContext()

	Success(c, nil)

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Code != http.StatusOK {
		t.Errorf("expected code %d, got %d", http.StatusOK, resp.Code)
	}
	if resp.Data != nil {
		t.Errorf("expected nil data, got %v", resp.Data)
	}
}

func TestError_AppError_NotFound(t *testing.T) {
	c, w := newResponseTestContext()

	appErr := domain.NewAppError(domain.CodeNotFound, "user not found", nil)
	Error(c, appErr)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Code != http.StatusNotFound {
		t.Errorf("expected code %d, got %d", http.StatusNotFound, resp.Code)
	}
	if resp.Message != "user not found" {
		t.Errorf("expected message %q, got %q", "user not found", resp.Message)
	}
	if resp.Data != nil {
		t.Errorf("expected nil data, got %v", resp.Data)
	}
}

func TestError_AppError_AlreadyExists(t *testing.T) {
	c, w := newResponseTestContext()

	appErr := domain.NewAppError(domain.CodeAlreadyExists, "email taken", nil)
	Error(c, appErr)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, w.Code)
	}

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Code != http.StatusConflict {
		t.Errorf("expected code %d, got %d", http.StatusConflict, resp.Code)
	}
	if resp.Message != "email taken" {
		t.Errorf("expected message %q, got %q", "email taken", resp.Message)
	}
}

func TestError_AppError_Validation(t *testing.T) {
	c, w := newResponseTestContext()

	appErr := domain.NewAppError(domain.CodeValidation, "bad input", nil)
	Error(c, appErr)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestError_GenericError(t *testing.T) {
	c, w := newResponseTestContext()

	Error(c, errors.New("something broke"))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Code != http.StatusInternalServerError {
		t.Errorf("expected code %d, got %d", http.StatusInternalServerError, resp.Code)
	}
	if resp.Message != "internal error" {
		t.Errorf("expected message %q, got %q", "internal error", resp.Message)
	}
}

func TestList(t *testing.T) {
	c, w := newResponseTestContext()

	type item struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	result := struct {
		Items      []item `json:"items"`
		Total      int64  `json:"total"`
		Page       int    `json:"page"`
		PageSize   int    `json:"page_size"`
		TotalPages int    `json:"total_pages"`
	}{
		Items:      []item{{ID: 1, Name: "Alice"}, {ID: 2, Name: "Bob"}},
		Total:      2,
		Page:       1,
		PageSize:   20,
		TotalPages: 1,
	}
	List(c, result)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Code != http.StatusOK {
		t.Errorf("expected code %d, got %d", http.StatusOK, resp.Code)
	}
	if resp.Message != "success" {
		t.Errorf("expected message %q, got %q", "success", resp.Message)
	}
	if resp.Data == nil {
		t.Error("expected data to be non-nil")
	}

	// Verify nested structure by re-marshaling data.
	dataBytes, _ := json.Marshal(resp.Data)
	var pageResult struct {
		Items      []item `json:"items"`
		Total      int64  `json:"total"`
		Page       int    `json:"page"`
		PageSize   int    `json:"page_size"`
		TotalPages int    `json:"total_pages"`
	}
	if err := json.Unmarshal(dataBytes, &pageResult); err != nil {
		t.Fatalf("failed to unmarshal page result: %v", err)
	}
	if len(pageResult.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(pageResult.Items))
	}
	if pageResult.Total != 2 {
		t.Errorf("expected total 2, got %d", pageResult.Total)
	}
}

func TestValidationError_WithValidatorErrors(t *testing.T) {
	c, w := newResponseTestContext()

	ve := makeValidationErrors(t)
	ValidationError(c, ve)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp ValidationErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Code != http.StatusBadRequest {
		t.Errorf("expected code %d, got %d", http.StatusBadRequest, resp.Code)
	}
	if resp.Message != "validation error" {
		t.Errorf("expected message %q, got %q", "validation error", resp.Message)
	}
	if len(resp.Errors) == 0 {
		t.Fatal("expected field errors, got none")
	}

	// Without obj, ValidationError falls back to lowercased struct field names.
	if msg, ok := resp.Errors["name"]; !ok {
		t.Error("expected error for field 'name'")
	} else if msg != "This field is required" {
		t.Errorf("expected message %q for name, got %q", "This field is required", msg)
	}

	if msg, ok := resp.Errors["email"]; !ok {
		t.Error("expected error for field 'email'")
	} else if msg != "This field is required" {
		t.Errorf("expected message %q for email, got %q", "This field is required", msg)
	}
}

func TestValidationError_NonValidationError(t *testing.T) {
	c, w := newResponseTestContext()

	ValidationError(c, errors.New("bad json"))

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Code != http.StatusBadRequest {
		t.Errorf("expected code %d, got %d", http.StatusBadRequest, resp.Code)
	}
	if resp.Message != "bad request" {
		t.Errorf("expected message %q, got %q", "bad request", resp.Message)
	}
}

func TestBindAndValidate_InvalidJSON(t *testing.T) {
	c, w := newResponseTestContextWithBody(`{"invalid json`)

	// bindInput uses gin binding tags.
	type bindInput struct {
		Name  string `json:"name" binding:"required"`
		Email string `json:"email" binding:"required,email"`
	}

	var input bindInput
	ok := BindAndValidate(c, &input)

	if ok {
		t.Error("expected BindAndValidate to return false for invalid JSON")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Message != "bad request" {
		t.Errorf("expected message %q, got %q", "bad request", resp.Message)
	}
}

func TestBindAndValidate_MissingFields(t *testing.T) {
	c, w := newResponseTestContextWithBody(`{}`)

	type bindInput struct {
		Name  string `json:"name" binding:"required"`
		Email string `json:"email" binding:"required,email"`
	}

	var input bindInput
	ok := BindAndValidate(c, &input)

	if ok {
		t.Error("expected BindAndValidate to return false for missing required fields")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp ValidationErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Code != http.StatusBadRequest {
		t.Errorf("expected code %d, got %d", http.StatusBadRequest, resp.Code)
	}
	if resp.Message != "validation error" {
		t.Errorf("expected message %q, got %q", "validation error", resp.Message)
	}

	// BindAndValidate has obj, so it should use JSON tag names.
	if _, ok := resp.Errors["name"]; !ok {
		t.Error("expected error for field 'name'")
	}
	if _, ok := resp.Errors["email"]; !ok {
		t.Error("expected error for field 'email'")
	}
}

func TestBindAndValidate_InvalidEmail(t *testing.T) {
	c, w := newResponseTestContextWithBody(`{"name":"Alice","email":"not-an-email"}`)

	type bindInput struct {
		Name  string `json:"name" binding:"required"`
		Email string `json:"email" binding:"required,email"`
	}

	var input bindInput
	ok := BindAndValidate(c, &input)

	if ok {
		t.Error("expected BindAndValidate to return false for invalid email")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp ValidationErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if msg, ok := resp.Errors["email"]; !ok {
		t.Error("expected error for field 'email'")
	} else if msg != "Must be a valid email address" {
		t.Errorf("expected message %q for email field, got %q", "Must be a valid email address", msg)
	}

	// name should NOT be in errors since it's valid.
	if _, ok := resp.Errors["name"]; ok {
		t.Error("did not expect error for field 'name'")
	}
}

func TestBindAndValidate_MinLength(t *testing.T) {
	c, w := newResponseTestContextWithBody(`{"name":"Al"}`)

	type bindInput struct {
		Name string `json:"name" binding:"required,min=3"`
	}

	var input bindInput
	ok := BindAndValidate(c, &input)

	if ok {
		t.Error("expected BindAndValidate to return false for too-short name")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp ValidationErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if msg, ok := resp.Errors["name"]; !ok {
		t.Error("expected error for field 'name'")
	} else if msg != "Must be at least 3 characters" {
		t.Errorf("expected message %q for name field, got %q", "Must be at least 3 characters", msg)
	}
}

func TestBindAndValidate_ValidInput(t *testing.T) {
	c, w := newResponseTestContextWithBody(`{"name":"Alice","email":"alice@example.com"}`)

	type bindInput struct {
		Name  string `json:"name" binding:"required"`
		Email string `json:"email" binding:"required,email"`
	}

	var input bindInput
	ok := BindAndValidate(c, &input)

	if !ok {
		t.Error("expected BindAndValidate to return true for valid input")
	}
	// No response should be written on success.
	if w.Code != http.StatusOK {
		t.Errorf("expected recorder default status 200, got %d", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("expected empty body on success, got %q", w.Body.String())
	}
	if input.Name != "Alice" {
		t.Errorf("expected Name='Alice', got %q", input.Name)
	}
	if input.Email != "alice@example.com" {
		t.Errorf("expected Email='alice@example.com', got %q", input.Email)
	}
}
