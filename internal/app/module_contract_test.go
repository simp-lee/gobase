package app

import (
	"testing"

	authmodule "github.com/simp-lee/gobase/internal/module/auth"
	usermodule "github.com/simp-lee/gobase/internal/module/user"
)

// TestModuleImplementationsCompile validates that concrete modules satisfy
// the real app.Module contract at compile-time.
func TestModuleImplementationsCompile(t *testing.T) {
	var _ Module = (*authmodule.AuthModule)(nil)
	var _ Module = (*usermodule.UserModule)(nil)
}
