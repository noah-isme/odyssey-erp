package users

import "time"

// User represents a user account for management.
type User struct {
	ID        int64
	Email     string
	Name      string
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
