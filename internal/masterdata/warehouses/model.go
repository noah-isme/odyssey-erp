package warehouses

import (
	"time"
)

// Warehouse represents a warehouse entity
type Warehouse struct {
	ID        int64     `json:"id"`
	BranchID  int64     `json:"branch_id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
