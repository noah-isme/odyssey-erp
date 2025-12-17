package roles

import "time"

// Role represents a role for management.
type Role struct {
	ID          int64
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
