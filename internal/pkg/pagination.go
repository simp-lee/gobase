package pkg

import (
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/simp-lee/gobase/internal/domain"
	"github.com/simp-lee/pagination"
	"gorm.io/gorm"
)

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 100
	defaultSort     = "id:desc"
)

// reservedParams lists query parameter names used for pagination/sorting, not for filtering.
var reservedParams = map[string]bool{
	"page":      true,
	"page_size": true,
	"sort":      true,
}

// validFieldName matches only alphanumeric characters and underscores.
var validFieldName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// likeEscaper escapes special SQL LIKE characters so user-supplied values
// are treated as literals. strings.NewReplacer performs a single-pass scan,
// so no double-escaping can occur regardless of pair order.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// ParsePageRequest extracts pagination, sorting, and filtering parameters from query params.
func ParsePageRequest(c *gin.Context) domain.PageRequest {
	page, _ := strconv.Atoi(c.DefaultQuery("page", strconv.Itoa(defaultPage)))
	if page < 1 {
		page = defaultPage
	}

	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(defaultPageSize)))
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	sort := c.DefaultQuery("sort", defaultSort)

	filter := make(map[string]string)
	for key, values := range c.Request.URL.Query() {
		if reservedParams[key] {
			continue
		}
		if len(values) > 0 && values[0] != "" {
			filter[key] = values[0]
		}
	}

	return domain.PageRequest{
		Page:     page,
		PageSize: pageSize,
		Sort:     sort,
		Filter:   filter,
	}
}

// Paginate returns a GORM scope that applies LIMIT and OFFSET based on the page request.
func Paginate(req domain.PageRequest) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		offset := (req.Page - 1) * req.PageSize
		return db.Offset(offset).Limit(req.PageSize)
	}
}

// Sort returns a GORM scope that applies ORDER BY based on the page request.
// Only field names present in the allowed list are accepted; others are silently ignored.
// Field names are validated against a strict pattern to prevent SQL injection.
func Sort(req domain.PageRequest, allowed []string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		parts := strings.SplitN(req.Sort, ":", 2)
		if len(parts) != 2 {
			return db
		}

		field := strings.TrimSpace(parts[0])
		direction := strings.TrimSpace(strings.ToLower(parts[1]))

		if direction != "asc" && direction != "desc" {
			return db
		}

		if !validFieldName.MatchString(field) {
			return db
		}

		if !isAllowed(field, allowed) {
			return db
		}

		return db.Order(field + " " + direction)
	}
}

// Filter returns a GORM scope that applies WHERE conditions based on the page request filters.
// Only filter keys present in the allowed list are applied; others are silently ignored.
// Keys ending with "__like" produce a LIKE '%value%' condition; others use exact match.
func Filter(req domain.PageRequest, allowed []string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		for key, value := range req.Filter {
			// Check for __like suffix.
			if before, ok := strings.CutSuffix(key, "__like"); ok {
				field := before
				if !validFieldName.MatchString(field) {
					continue
				}
				if !isAllowed(field, allowed) {
					continue
				}
				escaped := likeEscaper.Replace(value)
				db = db.Where(field+" LIKE ? ESCAPE '\\'", "%"+escaped+"%")
			} else {
				if !validFieldName.MatchString(key) {
					continue
				}
				if !isAllowed(key, allowed) {
					continue
				}
				db = db.Where(key+" = ?", value)
			}
		}
		return db
	}
}

// isAllowed checks if a field name is in the allowed list.
func isAllowed(field string, allowed []string) bool {
	return slices.Contains(allowed, field)
}

// ListOptions configures which fields are allowed for sorting and filtering
// in PaginateGORM.
type ListOptions struct {
	SortFields   []string
	FilterFields []string
}

// PaginateGORM executes a paginated GORM query using the simp-lee/pagination library.
// It applies filtering, sorting, and offset/limit via the existing scope helpers,
// and returns a fully populated Pagination result.
func PaginateGORM[T any](ctx context.Context, db *gorm.DB, req domain.PageRequest, opts ListOptions) (*pagination.Pagination[T], error) {
	// Apply filter scope to the base query.
	filtered := db.Scopes(Filter(req, opts.FilterFields))

	paginator := pagination.NewPaginator[T](
		pagination.WithItemsPerPage[T](req.PageSize),
		pagination.WithItemTotalCallback[T](func(ctx context.Context) (int64, error) {
			var count int64
			err := filtered.Session(&gorm.Session{}).WithContext(ctx).Count(&count).Error
			return count, err
		}),
		pagination.WithSliceCallback[T](func(ctx context.Context, offset, limit int) ([]T, error) {
			var items []T
			err := filtered.Session(&gorm.Session{}).WithContext(ctx).
				Scopes(Sort(req, opts.SortFields)).
				Offset(offset).Limit(limit).
				Find(&items).Error
			return items, err
		}),
	)

	return paginator.Paginate(ctx, req.Page)
}
