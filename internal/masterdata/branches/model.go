package branches

import (
	"time"
)

// Branch represents a branch entity
type Branch struct {
	ID        int64     `json:"id"`
	CompanyID int64     `json:"company_id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
