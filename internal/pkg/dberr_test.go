package pkg

import (
	"errors"
	"fmt"
	"testing"

	"github.com/simp-lee/gobase/internal/domain"
	"gorm.io/gorm"
)

func TestMapDBError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantNil  bool
		wantCode int
		wantMsg  string
	}{
		{
			name:    "nil error returns nil",
			err:     nil,
			wantNil: true,
		},
		{
			name:     "gorm ErrRecordNotFound returns domain ErrNotFound",
			err:      gorm.ErrRecordNotFound,
			wantCode: domain.CodeNotFound,
		},
		{
			name:     "wrapped gorm ErrRecordNotFound returns domain ErrNotFound",
			err:      fmt.Errorf("repo: %w", gorm.ErrRecordNotFound),
			wantCode: domain.CodeNotFound,
		},
		{
			name:     "gorm ErrDuplicatedKey returns CodeAlreadyExists",
			err:      gorm.ErrDuplicatedKey,
			wantCode: domain.CodeAlreadyExists,
			wantMsg:  "already exists",
		},
		{
			name:     "unique constraint message returns CodeAlreadyExists",
			err:      errors.New("UNIQUE constraint failed: users.email"),
			wantCode: domain.CodeAlreadyExists,
			wantMsg:  "already exists",
		},
		{
			name:     "duplicate key message returns CodeAlreadyExists",
			err:      errors.New("duplicate key value violates unique constraint"),
			wantCode: domain.CodeAlreadyExists,
			wantMsg:  "already exists",
		},
		{
			name:     "duplicate entry message returns CodeAlreadyExists",
			err:      errors.New("Duplicate entry '1' for key 'PRIMARY'"),
			wantCode: domain.CodeAlreadyExists,
			wantMsg:  "already exists",
		},
		{
			name:     "sqlstate 23505 returns CodeAlreadyExists",
			err:      errors.New("ERROR: sqlstate 23505 unique_violation"),
			wantCode: domain.CodeAlreadyExists,
			wantMsg:  "already exists",
		},
		{
			name:     "chinese PG message returns CodeAlreadyExists",
			err:      errors.New("错误: 违反唯一约束"),
			wantCode: domain.CodeAlreadyExists,
			wantMsg:  "already exists",
		},
		{
			name:     "unknown error returns CodeInternal",
			err:      errors.New("connection refused"),
			wantCode: domain.CodeInternal,
			wantMsg:  "database error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapDBError(tt.err)

			if tt.wantNil {
				if got != nil {
					t.Fatalf("MapDBError() = %v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Fatal("MapDBError() = nil, want non-nil error")
			}

			// ErrNotFound is returned as the sentinel directly.
			if tt.wantCode == domain.CodeNotFound {
				if !errors.Is(got, domain.ErrNotFound) {
					t.Fatalf("MapDBError() = %v, want domain.ErrNotFound", got)
				}
				return
			}

			var appErr *domain.AppError
			if !errors.As(got, &appErr) {
				t.Fatalf("MapDBError() type = %T, want *domain.AppError", got)
			}
			if appErr.Code != tt.wantCode {
				t.Errorf("AppError.Code = %d, want %d", appErr.Code, tt.wantCode)
			}
			if tt.wantMsg != "" && appErr.Message != tt.wantMsg {
				t.Errorf("AppError.Message = %q, want %q", appErr.Message, tt.wantMsg)
			}
			// Verify the original error is wrapped.
			if appErr.Err == nil {
				t.Error("AppError.Err is nil, want original error wrapped")
			}
		})
	}
}

func TestIsDuplicateDBError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"unique constraint", errors.New("UNIQUE constraint failed"), true},
		{"duplicate key", errors.New("duplicate key value violates"), true},
		{"duplicate entry", errors.New("Duplicate entry for key"), true},
		{"mixed case", errors.New("DUPLICATE KEY violation"), true},
		{"sqlstate 23505", errors.New("ERROR: sqlstate 23505 unique violation"), true},
		{"chinese pg message", errors.New("错误: 违反唯一约束"), true},
		{"unrelated error", errors.New("connection timeout"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDuplicateDBError(tt.err); got != tt.want {
				t.Errorf("IsDuplicateDBError() = %v, want %v", got, tt.want)
			}
		})
	}
}
