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

// PageResult holds a page of results with pagination metadata.
type PageResult[T any] struct {
	Items      []T   `json:"items"`
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalPages int   `json:"total_pages"`
}
