package pkg

import (
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
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
		"page":      {"3"},
		"page_size": {"50"},
		"sort":      {"name:asc"},
		"status":    {"active"},
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

func TestNewPageResult(t *testing.T) {
	tests := []struct {
		name       string
		items      []string
		total      int64
		page       int
		pageSize   int
		wantPages  int
		wantItems  int
	}{
		{
			name:      "exact division",
			items:     []string{"a", "b"},
			total:     10,
			page:      1,
			pageSize:  5,
			wantPages: 2,
			wantItems: 2,
		},
		{
			name:      "with remainder",
			items:     []string{"a"},
			total:     11,
			page:      3,
			pageSize:  5,
			wantPages: 3,
			wantItems: 1,
		},
		{
			name:      "zero total",
			items:     nil,
			total:     0,
			page:      1,
			pageSize:  20,
			wantPages: 0,
			wantItems: 0,
		},
		{
			name:      "single page",
			items:     []string{"a", "b", "c"},
			total:     3,
			page:      1,
			pageSize:  20,
			wantPages: 1,
			wantItems: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := domain.PageRequest{Page: tt.page, PageSize: tt.pageSize}
			result := NewPageResult(tt.items, tt.total, req)

			if result.TotalPages != tt.wantPages {
				t.Errorf("TotalPages: want %d, got %d", tt.wantPages, result.TotalPages)
			}
			if len(result.Items) != tt.wantItems {
				t.Errorf("Items count: want %d, got %d", tt.wantItems, len(result.Items))
			}
			if result.Total != tt.total {
				t.Errorf("Total: want %d, got %d", tt.total, result.Total)
			}
			if result.Page != tt.page {
				t.Errorf("Page: want %d, got %d", tt.page, result.Page)
			}
			if result.PageSize != tt.pageSize {
				t.Errorf("PageSize: want %d, got %d", tt.pageSize, result.PageSize)
			}
		})
	}
}

func TestNewPageResult_NilItemsBecomesEmptySlice(t *testing.T) {
	req := domain.PageRequest{Page: 1, PageSize: 10}
	result := NewPageResult[string](nil, 0, req)

	if result.Items == nil {
		t.Error("expected non-nil Items slice")
	}
	if len(result.Items) != 0 {
		t.Errorf("expected empty Items, got %d items", len(result.Items))
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

// --------------- NewPageResult: additional TotalPages calculations ---------------

func TestNewPageResult_AdditionalCalculations(t *testing.T) {
	tests := []struct {
		name      string
		total     int64
		pageSize  int
		wantPages int
	}{
		{"25 items / 10 per page = 3 pages", 25, 10, 3},
		{"20 items / 10 per page = 2 pages", 20, 10, 2},
		{"1 item / 10 per page = 1 page", 1, 10, 1},
		{"99 items / 10 per page = 10 pages", 99, 10, 10},
		{"100 items / 100 per page = 1 page", 100, 100, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := domain.PageRequest{Page: 1, PageSize: tt.pageSize}
			result := NewPageResult([]string{}, tt.total, req)
			if result.TotalPages != tt.wantPages {
				t.Errorf("TotalPages: want %d, got %d", tt.wantPages, result.TotalPages)
			}
		})
	}
}
