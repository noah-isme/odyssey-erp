package products

import (
	"time"
)

// Product represents a product entity
// Note: DB uses 'sku' column, maps to 'Code' field for backward compatibility
// Note: DB does not have 'cost', 'created_at', 'updated_at' columns but has 'deleted_at'
type Product struct {
	ID         int64      `json:"id"`
	Code       string     `json:"code"` // maps to 'sku' in database
	Name       string     `json:"name"`
	CategoryID int64      `json:"category_id"`
	UnitID     int64      `json:"unit_id"`
	Price      float64    `json:"price"`
	Cost       float64    `json:"cost"` // not in DB, kept for backward compat
	TaxID      int64      `json:"tax_id"`
	IsActive   bool       `json:"is_active"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"` // not in DB, kept for backward compat
	UpdatedAt  time.Time  `json:"updated_at"` // not in DB, kept for backward compat
}
