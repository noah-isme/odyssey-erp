package rbac

import "time"

// Role represents a high-level permission grouping.
type Role struct {
	ID          int64
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Permission represents an atomic capability.
type Permission struct {
	ID          int64
	Name        string
	Description string
}

// Assignment ties a permission to a role.
type Assignment struct {
	RoleID       int64
	PermissionID int64
	CreatedAt    time.Time
}

// UserRole links a user to a role.
type UserRole struct {
	UserID    int64
	RoleID    int64
	CreatedAt time.Time
}

// Principal describes the authenticated actor.
type Principal interface {
	GetID() int64
	IsSuperUser() bool
}
