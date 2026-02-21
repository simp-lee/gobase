package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUserJSON_PasswordHashHidden(t *testing.T) {
	user := User{
		Name:         "Alice",
		Email:        "alice@example.com",
		PasswordHash: "$2a$10$examplehash",
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
	if !strings.Contains(body, "\"name\":\"Alice\"") {
		t.Fatalf("json should include name field, got: %s", body)
	}
	if !strings.Contains(body, "\"email\":\"alice@example.com\"") {
		t.Fatalf("json should include email field, got: %s", body)
	}
}

func TestUserJSON_UnmarshalIgnoresPasswordHashField(t *testing.T) {
	input := `{"name":"Alice","email":"alice@example.com","password_hash":"attacker-controlled"}`

	var user User
	if err := json.Unmarshal([]byte(input), &user); err != nil {
		t.Fatalf("unmarshal user: %v", err)
	}

	if user.Name != "Alice" {
		t.Fatalf("Name = %q, want %q", user.Name, "Alice")
	}
	if user.Email != "alice@example.com" {
		t.Fatalf("Email = %q, want %q", user.Email, "alice@example.com")
	}
	if user.PasswordHash != "" {
		t.Fatalf("PasswordHash = %q, want empty", user.PasswordHash)
	}
}
