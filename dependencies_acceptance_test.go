package gobase_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestModuleDependencies_PaginationPresent(t *testing.T) {
	testModulePresence(t, "github.com/simp-lee/pagination")
}

func TestModuleDependencies_JWTPresent(t *testing.T) {
	testModulePresence(t, "github.com/simp-lee/jwt")
}

func TestModuleDependencies_RBACPresent(t *testing.T) {
	testModulePresence(t, "github.com/simp-lee/rbac")
}

func TestModuleDependencies_XCryptoPresent(t *testing.T) {
	testModulePresence(t, "golang.org/x/crypto")
}

func TestPaginationAPI_NoLegacyNewPageResult(t *testing.T) {
	t.Run("happy_repo_has_no_legacy_symbol", func(t *testing.T) {
		matches, err := findLegacyNewPageResultUsages(".")
		if err != nil {
			t.Fatalf("scan repository: %v", err)
		}
		if len(matches) != 0 {
			t.Fatalf("expected no NewPageResult usages, found in: %v", matches)
		}
	})

	t.Run("error_fixture_with_legacy_symbol_is_detected", func(t *testing.T) {
		fixture := `package pkg
func NewPageResult[T any]() {}`
		if !hasLegacyNewPageResult(fixture) {
			t.Fatal("expected legacy symbol to be detected in fixture")
		}
	})
}

func testModulePresence(t *testing.T, module string) {
	t.Helper()

	t.Run("happy_present_in_real_go_mod", func(t *testing.T) {
		goMod, err := os.ReadFile("go.mod")
		if err != nil {
			t.Fatalf("read go.mod: %v", err)
		}
		if !moduleRequired(string(goMod), module) {
			t.Fatalf("expected module %q to be present in go.mod", module)
		}
	})

	t.Run("error_missing_module_in_fixture", func(t *testing.T) {
		fixture := `module example.com/demo

go 1.25.0

require (
	github.com/gin-gonic/gin v1.11.0
)`
		if moduleRequired(fixture, module) {
			t.Fatalf("expected fixture to not contain module %q", module)
		}
	})
}

func moduleRequired(goModContent, module string) bool {
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(module) + `\s+v\S+`)
	return re.MatchString(goModContent)
}

func findLegacyNewPageResultUsages(root string) ([]string, error) {
	matches := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".agents-work" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if hasLegacyNewPageResult(string(b)) {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return matches, nil
}

func hasLegacyNewPageResult(content string) bool {
	re := regexp.MustCompile(`\bNewPageResult\s*(\[[^\]]+\])?\s*\(`)
	return re.MatchString(content)
}
