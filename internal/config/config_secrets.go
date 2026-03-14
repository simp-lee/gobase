package config

import (
	"strings"
	"unicode"
)

// CountSecretClasses counts how many character classes (lowercase, uppercase,
// digit, symbol) are present in the given secret string.
func CountSecretClasses(secret string) int {
	hasLower := false
	hasUpper := false
	hasDigit := false
	hasSymbol := false

	for _, r := range secret {
		switch {
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		default:
			hasSymbol = true
		}
	}

	classes := 0
	if hasLower {
		classes++
	}
	if hasUpper {
		classes++
	}
	if hasDigit {
		classes++
	}
	if hasSymbol {
		classes++
	}

	return classes
}

// IsPlaceholderJWTSecret reports whether the provided JWT secret is a known
// placeholder/default value that must not be used in release mode.
func IsPlaceholderJWTSecret(secret string) bool {
	normalized := strings.ToLower(strings.TrimSpace(secret))

	switch normalized {
	case "",
		"change-me-to-a-random-secret",
		"change-me-in-env",
		"gobase-dev-jwt-secret-key-change-me":
		return true
	default:
		return false
	}
}

// IsPlaceholderPostgresPassword reports whether the provided Postgres password
// is a known placeholder/default value that must not be used in release mode.
func IsPlaceholderPostgresPassword(password string) bool {
	normalized := strings.ToLower(strings.TrimSpace(password))

	switch normalized {
	case "",
		"password",
		"postgres",
		"gobase",
		"change-me",
		"change-me-in-env":
		return true
	default:
		return false
	}
}

// IsPlaceholderCSRFSecret reports whether the provided CSRF secret is a known
// placeholder/default value that must not be used in release mode.
func IsPlaceholderCSRFSecret(secret string) bool {
	normalized := strings.ToLower(strings.TrimSpace(secret))

	switch normalized {
	case "",
		"change-me-to-a-random-secret",
		"change-me-in-env":
		return true
	default:
		return false
	}
}
