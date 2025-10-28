package auth

import "time"

// User represents an authenticated user account.
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	IsActive     bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
