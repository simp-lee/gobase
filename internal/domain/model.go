package domain

import "time"

// BaseModel is the common base struct for all domain models.
// It replaces gorm.Model to avoid the implicit soft delete behavior of DeletedAt.
type BaseModel struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PageRequest holds pagination, sorting, and filtering parameters.
type PageRequest struct {
	Page     int
	PageSize int
	Sort     string
	Filter   map[string]string
}

// PageResult holds a page of items together with pagination metadata.
type PageResult[T any] struct {
	Items       []T   `json:"items"`
	TotalItems  int64 `json:"total_items"`
	TotalPages  int   `json:"total_pages"`
	CurrentPage int   `json:"current_page"`
	PageSize    int   `json:"page_size"`
}
