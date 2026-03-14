package domain

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestUserStatusField_DefaultAndConstraintTags(t *testing.T) {
	field, ok := reflect.TypeOf(User{}).FieldByName("Status")
	if !ok {
		t.Fatal("Status field not found")
	}

	gormTag := field.Tag.Get("gorm")
	if !strings.Contains(gormTag, "default:'active'") {
		t.Fatalf("gorm tag %q should declare default active", gormTag)
	}
	if !strings.Contains(gormTag, "status IN ('active','disabled','pending')") {
		t.Fatalf("gorm tag %q should declare status constraint", gormTag)
	}
}

func TestUserStatusConstants(t *testing.T) {
	if StatusActive != "active" {
		t.Errorf("StatusActive = %q; want %q", StatusActive, "active")
	}
	if StatusDisabled != "disabled" {
		t.Errorf("StatusDisabled = %q; want %q", StatusDisabled, "disabled")
	}
	if StatusPending != "pending" {
		t.Errorf("StatusPending = %q; want %q", StatusPending, "pending")
	}
}

func TestUserJSON_PasswordHashHidden(t *testing.T) {
	user := User{
		Username:     "Alice",
		Email:        "alice@example.com",
		PasswordHash: "$2a$10$examplehash",
		Role:         RoleUser,
		Status:       StatusActive,
	}

	raw, err := json.Marshal(user)
	if err != nil {
		t.Fatalf("marshal user: %v", err)
	}

	body := string(raw)
	if strings.Contains(body, "password_hash") {
		t.Fatalf("json should not contain password_hash, got: %s", body)
	}
	if strings.Contains(body, "$2a$10$examplehash") {
		t.Fatalf("json should not contain PasswordHash value, got: %s", body)
	}
	if !strings.Contains(body, "\"username\":\"Alice\"") {
		t.Fatalf("json should include username field, got: %s", body)
	}
	if !strings.Contains(body, "\"role\":\"user\"") {
		t.Fatalf("json should include role field, got: %s", body)
	}
	if !strings.Contains(body, "\"email\":\"alice@example.com\"") {
		t.Fatalf("json should include email field, got: %s", body)
	}
	if !strings.Contains(body, "\"status\":\"active\"") {
		t.Fatalf("json should include status field, got: %s", body)
	}
}

func TestUserJSON_UnmarshalIgnoresPasswordHashField(t *testing.T) {
	input := `{"username":"Alice","email":"alice@example.com","password_hash":"attacker-controlled","role":"user"}`

	var user User
	if err := json.Unmarshal([]byte(input), &user); err != nil {
		t.Fatalf("unmarshal user: %v", err)
	}

	if user.Username != "Alice" {
		t.Fatalf("Username = %q, want %q", user.Username, "Alice")
	}
	if user.Email != "alice@example.com" {
		t.Fatalf("Email = %q, want %q", user.Email, "alice@example.com")
	}
	if user.PasswordHash != "" {
		t.Fatalf("PasswordHash = %q, want empty", user.PasswordHash)
	}
}
