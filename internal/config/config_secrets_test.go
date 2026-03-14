package config

import "testing"

func TestIsPlaceholderCSRFSecret(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		want   bool
	}{
		{name: "empty string", secret: "", want: true},
		{name: "whitespace only", secret: "   ", want: true},
		{name: "change-me-to-a-random-secret", secret: "change-me-to-a-random-secret", want: true},
		{name: "change-me-in-env", secret: "change-me-in-env", want: true},
		{name: "case insensitive", secret: "CHANGE-ME-IN-ENV", want: true},
		{name: "with surrounding whitespace", secret: "  change-me-to-a-random-secret  ", want: true},
		{name: "real secret", secret: "a-real-csrf-secret-value-here-32chars!", want: false},
		{name: "random string", secret: "xK9mPqR2sT5uV8wY", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPlaceholderCSRFSecret(tt.secret)
			if got != tt.want {
				t.Errorf("IsPlaceholderCSRFSecret(%q) = %v, want %v", tt.secret, got, tt.want)
			}
		})
	}
}

func TestIsPlaceholderJWTSecret(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		want   bool
	}{
		{name: "empty string", secret: "", want: true},
		{name: "whitespace only", secret: "   ", want: true},
		{name: "change-me-to-a-random-secret", secret: "change-me-to-a-random-secret", want: true},
		{name: "change-me-in-env", secret: "change-me-in-env", want: true},
		{name: "gobase dev default", secret: "gobase-dev-jwt-secret-key-change-me", want: true},
		{name: "case insensitive", secret: "CHANGE-ME-IN-ENV", want: true},
		{name: "with surrounding whitespace", secret: "  change-me-in-env  ", want: true},
		{name: "real secret", secret: "a-real-jwt-secret-value-here-32c!", want: false},
		{name: "random string", secret: "xK9mPqR2sT5uV8wYzA3bC6dE9fG2hJ5", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPlaceholderJWTSecret(tt.secret)
			if got != tt.want {
				t.Errorf("IsPlaceholderJWTSecret(%q) = %v, want %v", tt.secret, got, tt.want)
			}
		})
	}
}

func TestIsPlaceholderPostgresPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		want     bool
	}{
		{name: "empty string", password: "", want: true},
		{name: "whitespace only", password: "   ", want: true},
		{name: "password", password: "password", want: true},
		{name: "postgres", password: "postgres", want: true},
		{name: "gobase", password: "gobase", want: true},
		{name: "change-me", password: "change-me", want: true},
		{name: "change-me-in-env", password: "change-me-in-env", want: true},
		{name: "case insensitive", password: "PASSWORD", want: true},
		{name: "with surrounding whitespace", password: "  postgres  ", want: true},
		{name: "real password", password: "s3cur3-db-p@ssw0rd!", want: false},
		{name: "random string", password: "xK9mPqR2sT5uV8wY", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPlaceholderPostgresPassword(tt.password)
			if got != tt.want {
				t.Errorf("IsPlaceholderPostgresPassword(%q) = %v, want %v", tt.password, got, tt.want)
			}
		})
	}
}

func TestCountSecretClasses(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		want   int
	}{
		{name: "empty string", secret: "", want: 0},
		{name: "lowercase only", secret: "abcdef", want: 1},
		{name: "uppercase only", secret: "ABCDEF", want: 1},
		{name: "digits only", secret: "123456", want: 1},
		{name: "symbols only", secret: "!@#$%^", want: 1},
		{name: "lower and upper", secret: "abcDEF", want: 2},
		{name: "lower upper digit", secret: "abcDEF123", want: 3},
		{name: "all four classes", secret: "abcDEF123!", want: 4},
		{name: "mixed with spaces", secret: "aA1 ", want: 4}, // space counts as symbol
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CountSecretClasses(tt.secret)
			if got != tt.want {
				t.Errorf("CountSecretClasses(%q) = %d, want %d", tt.secret, got, tt.want)
			}
		})
	}
}
