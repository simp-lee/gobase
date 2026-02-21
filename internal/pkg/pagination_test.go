package pkg

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/simp-lee/gobase/internal/domain"
	"gorm.io/gorm"
	dbtest "gorm.io/gorm/utils/tests"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestContext(queryParams url.Values) *gin.Context {
	req := httptest.NewRequest(http.MethodGet, "/?"+queryParams.Encode(), nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return c
}

func TestParsePageRequest_Defaults(t *testing.T) {
	c := newTestContext(url.Values{})
	pr := ParsePageRequest(c)

	if pr.Page != 1 {
		t.Errorf("expected Page=1, got %d", pr.Page)
	}
	if pr.PageSize != 20 {
		t.Errorf("expected PageSize=20, got %d", pr.PageSize)
	}
	if pr.Sort != "id:desc" {
		t.Errorf("expected Sort=id:desc, got %s", pr.Sort)
	}
	if len(pr.Filter) != 0 {
		t.Errorf("expected empty Filter, got %v", pr.Filter)
	}
}

func TestParsePageRequest_CustomValues(t *testing.T) {
	c := newTestContext(url.Values{
		"page":       {"3"},
		"page_size":  {"50"},
		"sort":       {"name:asc"},
		"status":     {"active"},
		"name__like": {"john"},
	})
	pr := ParsePageRequest(c)

	if pr.Page != 3 {
		t.Errorf("expected Page=3, got %d", pr.Page)
	}
	if pr.PageSize != 50 {
		t.Errorf("expected PageSize=50, got %d", pr.PageSize)
	}
	if pr.Sort != "name:asc" {
		t.Errorf("expected Sort=name:asc, got %s", pr.Sort)
	}
	if pr.Filter["status"] != "active" {
		t.Errorf("expected Filter[status]=active, got %s", pr.Filter["status"])
	}
	if pr.Filter["name__like"] != "john" {
		t.Errorf("expected Filter[name__like]=john, got %s", pr.Filter["name__like"])
	}
}

func TestParsePageRequest_Clamping(t *testing.T) {
	t.Run("page below minimum", func(t *testing.T) {
		c := newTestContext(url.Values{"page": {"0"}})
		pr := ParsePageRequest(c)
		if pr.Page != 1 {
			t.Errorf("expected Page=1, got %d", pr.Page)
		}
	})

	t.Run("negative page", func(t *testing.T) {
		c := newTestContext(url.Values{"page": {"-5"}})
		pr := ParsePageRequest(c)
		if pr.Page != 1 {
			t.Errorf("expected Page=1, got %d", pr.Page)
		}
	})

	t.Run("page_size below minimum", func(t *testing.T) {
		c := newTestContext(url.Values{"page_size": {"0"}})
		pr := ParsePageRequest(c)
		if pr.PageSize != 20 {
			t.Errorf("expected PageSize=20, got %d", pr.PageSize)
		}
	})

	t.Run("page_size above maximum", func(t *testing.T) {
		c := newTestContext(url.Values{"page_size": {"200"}})
		pr := ParsePageRequest(c)
		if pr.PageSize != 100 {
			t.Errorf("expected PageSize=100, got %d", pr.PageSize)
		}
	})

	t.Run("invalid page_size defaults", func(t *testing.T) {
		c := newTestContext(url.Values{"page_size": {"abc"}})
		pr := ParsePageRequest(c)
		if pr.PageSize != 20 {
			t.Errorf("expected PageSize=20, got %d", pr.PageSize)
		}
	})
}

func TestParsePageRequest_EmptyFilterValuesIgnored(t *testing.T) {
	c := newTestContext(url.Values{
		"status": {""},
		"name":   {"john"},
	})
	pr := ParsePageRequest(c)

	if _, ok := pr.Filter["status"]; ok {
		t.Error("expected empty filter value to be excluded")
	}
	if pr.Filter["name"] != "john" {
		t.Errorf("expected Filter[name]=john, got %s", pr.Filter["name"])
	}
}

func TestTotalPagesCalculation(t *testing.T) {
	tests := []struct {
		total    int64
		pageSize int
		want     int
	}{
		{0, 10, 0},
		{1, 10, 1},
		{10, 10, 1},
		{11, 10, 2},
		{100, 10, 10},
		{101, 10, 11},
	}

	for _, tt := range tests {
		got := int(math.Ceil(float64(tt.total) / float64(tt.pageSize)))
		if got != tt.want {
			t.Errorf("Ceil(%d/%d): want %d, got %d", tt.total, tt.pageSize, tt.want, got)
		}
	}
}

func TestIsAllowed(t *testing.T) {
	allowed := []string{"name", "email", "status"}

	if !isAllowed("name", allowed) {
		t.Error("expected 'name' to be allowed")
	}
	if isAllowed("password", allowed) {
		t.Error("expected 'password' to not be allowed")
	}
	if isAllowed("", allowed) {
		t.Error("expected empty string to not be allowed")
	}
}

func TestValidFieldName(t *testing.T) {
	valid := []string{"id", "name", "created_at", "user_name", "_private"}
	invalid := []string{"", "1field", "name;DROP", "field name", "a.b", "a-b"}

	for _, f := range valid {
		if !validFieldName.MatchString(f) {
			t.Errorf("expected %q to be valid", f)
		}
	}
	for _, f := range invalid {
		if validFieldName.MatchString(f) {
			t.Errorf("expected %q to be invalid", f)
		}
	}
}

// --------------- helpers for GORM scope tests ---------------

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(dbtest.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	return db
}

// --------------- ParsePageRequest: negative page_size ---------------

func TestParsePageRequest_NegativePageSize(t *testing.T) {
	c := newTestContext(url.Values{"page_size": {"-5"}})
	pr := ParsePageRequest(c)
	if pr.PageSize != 20 {
		t.Errorf("expected PageSize=20 for negative page_size, got %d", pr.PageSize)
	}
}

// --------------- Sort scope ---------------

func TestSort(t *testing.T) {
	tests := []struct {
		name    string
		sort    string
		allowed []string
		applied bool
	}{
		{"valid field asc", "name:asc", []string{"name", "email"}, true},
		{"valid field desc", "id:desc", []string{"id", "name"}, true},
		{"field not in allowed list", "password:asc", []string{"name", "email"}, false},
		{"malformed no colon", "name", []string{"name"}, false},
		{"empty direction", "name:", []string{"name"}, false},
		{"invalid direction", "name:up", []string{"name"}, false},
		{"sql injection in field", "name;DROP TABLE users--:asc", []string{"name"}, false},
		{"sql injection attempt", "1=1;--:asc", []string{"name"}, false},
		{"empty field", ":asc", []string{"name"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := domain.PageRequest{Sort: tt.sort}
			scope := Sort(req, tt.allowed)
			db := newTestDB(t)
			result := scope(db)
			_, hasOrder := result.Statement.Clauses["ORDER BY"]
			if hasOrder != tt.applied {
				t.Errorf("Order clause applied=%v, want %v", hasOrder, tt.applied)
			}
		})
	}
}

// --------------- Filter scope ---------------

func TestFilter(t *testing.T) {
	tests := []struct {
		name    string
		filter  map[string]string
		allowed []string
		applied bool
	}{
		{"valid exact match", map[string]string{"status": "active"}, []string{"status", "name"}, true},
		{"valid like match", map[string]string{"name__like": "john"}, []string{"name"}, true},
		{"field not in allowed", map[string]string{"password": "secret"}, []string{"name", "email"}, false},
		{"like field not in allowed", map[string]string{"password__like": "secret"}, []string{"name"}, false},
		{"sql injection in key", map[string]string{"name;DROP TABLE--": "val"}, []string{"name"}, false},
		{"sql injection with spaces", map[string]string{"name OR 1=1": "val"}, []string{"name"}, false},
		{"empty filter map", map[string]string{}, []string{"name"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := domain.PageRequest{Filter: tt.filter}
			scope := Filter(req, tt.allowed)
			db := newTestDB(t)
			result := scope(db)
			_, hasWhere := result.Statement.Clauses["WHERE"]
			if hasWhere != tt.applied {
				t.Errorf("Where clause applied=%v, want %v", hasWhere, tt.applied)
			}
		})
	}
}

func TestFilter_MultipleFields(t *testing.T) {
	req := domain.PageRequest{
		Filter: map[string]string{
			"status":     "active",
			"name__like": "john",
		},
	}
	allowed := []string{"status", "name"}
	scope := Filter(req, allowed)
	db := newTestDB(t)
	result := scope(db)
	_, hasWhere := result.Statement.Clauses["WHERE"]
	if !hasWhere {
		t.Error("expected Where clause with multiple valid filters")
	}
}

func TestFilter_MixedValidAndInvalid(t *testing.T) {
	req := domain.PageRequest{
		Filter: map[string]string{
			"status":   "active",
			"password": "secret",
		},
	}
	allowed := []string{"status", "name"}
	scope := Filter(req, allowed)
	db := newTestDB(t)
	result := scope(db)
	_, hasWhere := result.Statement.Clauses["WHERE"]
	if !hasWhere {
		t.Error("expected Where clause for the valid filter field")
	}
}

// --------------- Paginate scope ---------------

func TestPaginate(t *testing.T) {
	tests := []struct {
		name     string
		page     int
		pageSize int
	}{
		{"first page", 1, 10},
		{"second page", 2, 20},
		{"large page number", 100, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := domain.PageRequest{Page: tt.page, PageSize: tt.pageSize}
			scope := Paginate(req)
			db := newTestDB(t)
			result := scope(db)
			_, hasLimit := result.Statement.Clauses["LIMIT"]
			if !hasLimit {
				t.Error("expected LIMIT clause to be applied")
			}
		})
	}
}

// --------------- PaginateGORM ---------------

// paginationTestItem is a minimal model for PaginateGORM tests.
type paginationTestItem struct {
	ID   uint   `gorm:"primaryKey"`
	Name string `gorm:"size:100"`
}

func newSQLiteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&paginationTestItem{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedItems(t *testing.T, db *gorm.DB, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		if err := db.Create(&paginationTestItem{Name: "item_" + strconv.Itoa(i)}).Error; err != nil {
			t.Fatalf("seed item %d: %v", i, err)
		}
	}
}

func TestPaginateGORM_BasicPagination(t *testing.T) {
	db := newSQLiteTestDB(t)
	seedItems(t, db, 25)
	ctx := context.Background()

	req := domain.PageRequest{Page: 1, PageSize: 10, Sort: "id:asc"}
	opts := ListOptions{SortFields: []string{"id", "name"}}

	result, err := PaginateGORM[paginationTestItem](ctx, db.Model(&paginationTestItem{}), req, opts)
	if err != nil {
		t.Fatalf("PaginateGORM: %v", err)
	}

	if result.TotalItems != 25 {
		t.Errorf("TotalItems: want 25, got %d", result.TotalItems)
	}
	if len(result.Items) != 10 {
		t.Errorf("Items count: want 10, got %d", len(result.Items))
	}
	if result.CurrentPage != 1 {
		t.Errorf("CurrentPage: want 1, got %d", result.CurrentPage)
	}
	if result.TotalPages != 3 {
		t.Errorf("TotalPages: want 3, got %d", result.TotalPages)
	}
	if result.ItemsPerPage != 10 {
		t.Errorf("ItemsPerPage: want 10, got %d", result.ItemsPerPage)
	}
}

func TestPaginateGORM_SecondPage(t *testing.T) {
	db := newSQLiteTestDB(t)
	seedItems(t, db, 25)
	ctx := context.Background()

	req := domain.PageRequest{Page: 2, PageSize: 10, Sort: "id:asc"}
	opts := ListOptions{SortFields: []string{"id"}}

	result, err := PaginateGORM[paginationTestItem](ctx, db.Model(&paginationTestItem{}), req, opts)
	if err != nil {
		t.Fatalf("PaginateGORM: %v", err)
	}

	if len(result.Items) != 10 {
		t.Errorf("Items count: want 10, got %d", len(result.Items))
	}
	// Second page with id:asc should start at id=11
	if result.Items[0].ID != 11 {
		t.Errorf("first item ID: want 11, got %d", result.Items[0].ID)
	}
}

func TestPaginateGORM_LastPagePartial(t *testing.T) {
	db := newSQLiteTestDB(t)
	seedItems(t, db, 25)
	ctx := context.Background()

	req := domain.PageRequest{Page: 3, PageSize: 10, Sort: "id:asc"}
	opts := ListOptions{SortFields: []string{"id"}}

	result, err := PaginateGORM[paginationTestItem](ctx, db.Model(&paginationTestItem{}), req, opts)
	if err != nil {
		t.Fatalf("PaginateGORM: %v", err)
	}

	if len(result.Items) != 5 {
		t.Errorf("Items count: want 5, got %d", len(result.Items))
	}
	if result.TotalItems != 25 {
		t.Errorf("TotalItems: want 25, got %d", result.TotalItems)
	}
}

func TestPaginateGORM_WithFilter(t *testing.T) {
	db := newSQLiteTestDB(t)
	seedItems(t, db, 10)
	// Add specific items to filter on
	db.Create(&paginationTestItem{Name: "special"})
	db.Create(&paginationTestItem{Name: "special"})
	ctx := context.Background()

	req := domain.PageRequest{
		Page:     1,
		PageSize: 10,
		Sort:     "id:asc",
		Filter:   map[string]string{"name": "special"},
	}
	opts := ListOptions{
		SortFields:   []string{"id", "name"},
		FilterFields: []string{"name"},
	}

	result, err := PaginateGORM[paginationTestItem](ctx, db.Model(&paginationTestItem{}), req, opts)
	if err != nil {
		t.Fatalf("PaginateGORM: %v", err)
	}

	if result.TotalItems != 2 {
		t.Errorf("TotalItems: want 2, got %d", result.TotalItems)
	}
	if len(result.Items) != 2 {
		t.Errorf("Items count: want 2, got %d", len(result.Items))
	}
}

func TestPaginateGORM_EmptyResult(t *testing.T) {
	db := newSQLiteTestDB(t)
	ctx := context.Background()

	req := domain.PageRequest{Page: 1, PageSize: 10, Sort: "id:asc"}
	opts := ListOptions{SortFields: []string{"id"}}

	result, err := PaginateGORM[paginationTestItem](ctx, db.Model(&paginationTestItem{}), req, opts)
	if err != nil {
		t.Fatalf("PaginateGORM: %v", err)
	}

	if result.TotalItems != 0 {
		t.Errorf("TotalItems: want 0, got %d", result.TotalItems)
	}
	if len(result.Items) != 0 {
		t.Errorf("Items count: want 0, got %d", len(result.Items))
	}
}

func TestPaginateGORM_SortDesc(t *testing.T) {
	db := newSQLiteTestDB(t)
	seedItems(t, db, 5)
	ctx := context.Background()

	req := domain.PageRequest{Page: 1, PageSize: 10, Sort: "id:desc"}
	opts := ListOptions{SortFields: []string{"id"}}

	result, err := PaginateGORM[paginationTestItem](ctx, db.Model(&paginationTestItem{}), req, opts)
	if err != nil {
		t.Fatalf("PaginateGORM: %v", err)
	}

	if len(result.Items) != 5 {
		t.Fatalf("Items count: want 5, got %d", len(result.Items))
	}
	// First item should have highest ID
	if result.Items[0].ID != 5 {
		t.Errorf("first item ID: want 5, got %d", result.Items[0].ID)
	}
}

func TestPaginateGORM_LikeFilter_SpecialChars(t *testing.T) {
	db := newSQLiteTestDB(t)

	// Insert test records with special LIKE characters in names.
	// Discriminating rows: "1001 Bob" matches unescaped %100%% (contains "100") but not escaped %100\%%;
	// "BobXSmith" matches unescaped %Bob_Smith% (any char for _) but not escaped %Bob\_Smith%.
	items := []paginationTestItem{
		{Name: "100% Alice"},
		{Name: "Bob_Smith"},
		{Name: `Charlie\Backslash`},
		{Name: "Dave Plain"},
		{Name: "1001 Bob"},
		{Name: "BobXSmith"},
	}
	for i := range items {
		if err := db.Create(&items[i]).Error; err != nil {
			t.Fatalf("seed item: %v", err)
		}
	}

	ctx := context.Background()
	opts := ListOptions{
		SortFields:   []string{"id"},
		FilterFields: []string{"name"},
	}

	t.Run("percent sign is literal", func(t *testing.T) {
		req := domain.PageRequest{
			Page: 1, PageSize: 10, Sort: "id:asc",
			Filter: map[string]string{"name__like": "100%"},
		}
		result, err := PaginateGORM[paginationTestItem](ctx, db.Model(&paginationTestItem{}), req, opts)
		if err != nil {
			t.Fatalf("PaginateGORM: %v", err)
		}
		if result.TotalItems != 1 {
			t.Errorf("TotalItems: want 1 (only '100%% Alice'), got %d", result.TotalItems)
		}
		if result.TotalItems == 1 && result.Items[0].Name != "100% Alice" {
			t.Errorf("expected '100%%%% Alice', got %q", result.Items[0].Name)
		}
	})

	t.Run("underscore is literal", func(t *testing.T) {
		req := domain.PageRequest{
			Page: 1, PageSize: 10, Sort: "id:asc",
			Filter: map[string]string{"name__like": "Bob_Smith"},
		}
		result, err := PaginateGORM[paginationTestItem](ctx, db.Model(&paginationTestItem{}), req, opts)
		if err != nil {
			t.Fatalf("PaginateGORM: %v", err)
		}
		if result.TotalItems != 1 {
			t.Errorf("TotalItems: want 1 (only 'Bob_Smith'), got %d", result.TotalItems)
		}
		if result.TotalItems == 1 && result.Items[0].Name != "Bob_Smith" {
			t.Errorf("expected 'Bob_Smith', got %q", result.Items[0].Name)
		}
	})

	t.Run("backslash is literal", func(t *testing.T) {
		req := domain.PageRequest{
			Page: 1, PageSize: 10, Sort: "id:asc",
			Filter: map[string]string{"name__like": `Charlie\`},
		}
		result, err := PaginateGORM[paginationTestItem](ctx, db.Model(&paginationTestItem{}), req, opts)
		if err != nil {
			t.Fatalf("PaginateGORM: %v", err)
		}
		if result.TotalItems != 1 {
			t.Errorf(`TotalItems: want 1 (only 'Charlie\Backslash'), got %d`, result.TotalItems)
		}
	})
}

func TestPaginateGORM_CountError(t *testing.T) {
	db := newSQLiteTestDB(t)
	ctx := context.Background()

	req := domain.PageRequest{Page: 1, PageSize: 10, Sort: "id:asc"}
	opts := ListOptions{SortFields: []string{"id"}}

	result, err := PaginateGORM[paginationTestItem](ctx, db.Table("missing_table"), req, opts)
	if err == nil {
		t.Fatal("expected error when count query fails, got nil")
	}
	if result != nil {
		t.Fatalf("expected nil result when count query fails, got %+v", result)
	}
}

func TestPaginateGORM_FindError(t *testing.T) {
	db := newSQLiteTestDB(t)
	seedItems(t, db, 3)
	ctx := context.Background()

	callbackName := "test:force_find_error"
	errFind := errors.New("forced find error")
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if _, isCount := tx.Statement.Dest.(*int64); isCount {
			return
		}
		tx.AddError(errFind)
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove(callbackName)
	})

	req := domain.PageRequest{Page: 1, PageSize: 10, Sort: "id:asc"}
	opts := ListOptions{SortFields: []string{"id"}}

	result, err := PaginateGORM[paginationTestItem](ctx, db.Model(&paginationTestItem{}), req, opts)
	if err == nil {
		t.Fatal("expected error when find query fails, got nil")
	}
	if !errors.Is(err, errFind) {
		t.Fatalf("expected wrapped forced find error, got %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result when find query fails, got %+v", result)
	}
}
