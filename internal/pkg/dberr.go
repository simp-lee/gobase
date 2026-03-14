package pkg

import (
	"errors"
	"strings"

	"github.com/simp-lee/gobase/internal/domain"
	"gorm.io/gorm"
)

// MapDBError converts common GORM errors to domain errors.
// It is intended to be used by all repository implementations so that error
// mapping logic is defined in one place.
func MapDBError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNotFound
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) || IsDuplicateDBError(err) {
		return domain.NewAppError(domain.CodeAlreadyExists, "already exists", err)
	}
	return domain.NewAppError(domain.CodeInternal, "database error", err)
}

// IsDuplicateDBError detects unique constraint violations by examining the
// error message. This covers drivers that do not set gorm.ErrDuplicatedKey.
func IsDuplicateDBError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "duplicate entry") ||
		strings.Contains(msg, "sqlstate 23505") ||
		strings.Contains(msg, "违反唯一约束")
}
