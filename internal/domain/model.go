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
