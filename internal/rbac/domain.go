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

// AccessScope identifies the tenant context in which permissions are checked.
// A nil BranchID means the company-wide scope; it never means "all branches"
// when the caller is asking for company-level permissions.
type AccessScope struct {
	CompanyID int64
	BranchID  *int64
}

// ScopedRoleAssignment is a legacy role assigned to a user for one company
// and, optionally, one branch. ValidFrom is inclusive and ValidTo is exclusive.
type ScopedRoleAssignment struct {
	ID        int64
	CompanyID int64
	UserID    int64
	RoleID    int64
	BranchID  *int64
	ValidFrom time.Time
	ValidTo   *time.Time
	CreatedAt time.Time
}

// ScopedRoleAssignmentInput is the write shape for a tenant-safe assignment.
// A zero ValidFrom is replaced with the current UTC time by the service.
type ScopedRoleAssignmentInput struct {
	CompanyID int64
	UserID    int64
	RoleID    int64
	BranchID  *int64
	ValidFrom time.Time
	ValidTo   *time.Time
}

type AccessReviewStatus string

const (
	AccessReviewOpen      AccessReviewStatus = "OPEN"
	AccessReviewCompleted AccessReviewStatus = "COMPLETED"
)

type AccessReviewDecision string

const (
	AccessReviewApprove AccessReviewDecision = "APPROVE"
	AccessReviewRevoke  AccessReviewDecision = "REVOKE"
)

// AccessReview is a company-scoped review of one user's access. ReviewKey is
// the caller-supplied idempotency key for that user's review cycle.
type AccessReview struct {
	ID              int64
	CompanyID       int64
	SubjectUserID   int64
	ReviewKey       string
	Status          AccessReviewStatus
	Decision        AccessReviewDecision
	OpenedByUserID  int64
	DecidedByUserID *int64
	DecidedAt       *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type OpenAccessReviewInput struct {
	CompanyID      int64
	SubjectUserID  int64
	ReviewKey      string
	OpenedByUserID int64
}

// Principal describes the authenticated actor.
type Principal interface {
	GetID() int64
	IsSuperUser() bool
}
