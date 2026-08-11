package rbac

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNotFound indicates that the requested record does not exist.
var ErrNotFound = errors.New("rbac: not found")

var (
	ErrScopedRepositoryUnavailable = errors.New("rbac: scoped repository unavailable")
	ErrInvalidScope                = errors.New("rbac: invalid access scope")
	ErrInvalidEffectiveTime        = errors.New("rbac: effective time required")
	ErrInvalidAccessReviewDecision = errors.New("rbac: invalid access review decision")
	ErrAccessReviewClosed          = errors.New("rbac: access review already completed")
)

// Repository is the database-neutral RBAC persistence boundary.
type Repository interface {
	ListRoles(ctx context.Context) ([]Role, error)
	GetRole(ctx context.Context, id int64) (Role, error)
	CreateRole(ctx context.Context, name, description string) (Role, error)
	UpdateRole(ctx context.Context, id int64, name, description string) (Role, error)
	DeleteRole(ctx context.Context, id int64) (bool, error)
	ListPermissions(ctx context.Context) ([]Permission, error)
	EnsurePermission(ctx context.Context, name, description string) (Permission, error)
	ListRolePermissions(ctx context.Context, roleID int64) ([]Permission, error)
	AttachPermissionToRole(ctx context.Context, roleID, permissionID int64) error
	DetachPermissionFromRole(ctx context.Context, roleID, permissionID int64) error
	AssignRoleToUser(ctx context.Context, userID, roleID int64) error
	RemoveRoleFromUser(ctx context.Context, userID, roleID int64) error
	EffectivePermissions(ctx context.Context, userID int64) ([]string, error)
}

// ScopedRepository is the tenant-safe extension of Repository. It is kept
// separate so existing repository fakes and global callers remain compatible.
type ScopedRepository interface {
	Repository
	CreateScopedRoleAssignment(ctx context.Context, input ScopedRoleAssignmentInput) (ScopedRoleAssignment, error)
	ListScopedRoleAssignments(ctx context.Context, companyID, userID int64) ([]ScopedRoleAssignment, error)
	DeleteScopedRoleAssignment(ctx context.Context, companyID, assignmentID int64) (bool, error)
	EffectivePermissionsInScope(ctx context.Context, userID int64, scope AccessScope, at time.Time) ([]string, error)
	OpenAccessReview(ctx context.Context, input OpenAccessReviewInput) (AccessReview, error)
	GetAccessReview(ctx context.Context, companyID, reviewID int64) (AccessReview, error)
	ListOpenAccessReviews(ctx context.Context, companyID int64) ([]AccessReview, error)
	CompleteAccessReview(ctx context.Context, companyID, reviewID, decidedByUserID int64, decision AccessReviewDecision) (AccessReview, error)
}

// Service orchestrates RBAC operations and validation.
type Service struct {
	repo       Repository
	scopedRepo ScopedRepository
}

func NewService(repo Repository) *Service {
	scopedRepo, _ := repo.(ScopedRepository)
	return &Service{repo: repo, scopedRepo: scopedRepo}
}

func (s *Service) ListRoles(ctx context.Context) ([]Role, error) {
	return s.repo.ListRoles(ctx)
}

func (s *Service) GetRole(ctx context.Context, id int64) (Role, error) {
	return s.repo.GetRole(ctx, id)
}

func (s *Service) CreateRole(ctx context.Context, name, description string) (Role, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Role{}, errors.New("rbac: role name required")
	}
	return s.repo.CreateRole(ctx, name, strings.TrimSpace(description))
}

func (s *Service) UpdateRole(ctx context.Context, id int64, name, description string) (Role, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Role{}, errors.New("rbac: role name required")
	}
	return s.repo.UpdateRole(ctx, id, name, strings.TrimSpace(description))
}

func (s *Service) DeleteRole(ctx context.Context, id int64) error {
	deleted, err := s.repo.DeleteRole(ctx, id)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrNotFound
	}
	return nil
}

func (s *Service) ListPermissions(ctx context.Context) ([]Permission, error) {
	return s.repo.ListPermissions(ctx)
}

func (s *Service) EnsurePermission(ctx context.Context, name, description string) (Permission, error) {
	return s.repo.EnsurePermission(ctx, strings.TrimSpace(name), strings.TrimSpace(description))
}

func (s *Service) SetRolePermissions(ctx context.Context, roleID int64, permissionIDs []int64) error {
	perms, err := s.repo.ListRolePermissions(ctx, roleID)
	if err != nil {
		return err
	}
	existing := make(map[int64]struct{}, len(perms))
	for _, permission := range perms {
		existing[permission.ID] = struct{}{}
	}
	keep := make(map[int64]struct{}, len(permissionIDs))
	for _, id := range permissionIDs {
		keep[id] = struct{}{}
		if _, ok := existing[id]; !ok {
			if err := s.repo.AttachPermissionToRole(ctx, roleID, id); err != nil {
				return err
			}
		}
	}
	for id := range existing {
		if _, ok := keep[id]; !ok {
			if err := s.repo.DetachPermissionFromRole(ctx, roleID, id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) AssignRole(ctx context.Context, userID, roleID int64) error {
	return s.repo.AssignRoleToUser(ctx, userID, roleID)
}

func (s *Service) RemoveRole(ctx context.Context, userID, roleID int64) error {
	return s.repo.RemoveRoleFromUser(ctx, userID, roleID)
}

func (s *Service) EffectivePermissions(ctx context.Context, userID int64) ([]string, error) {
	return s.repo.EffectivePermissions(ctx, userID)
}

// AssignRoleInScope creates an effective-dated assignment in one company and
// optional branch. It does not alter the existing global user_roles API.
func (s *Service) AssignRoleInScope(ctx context.Context, input ScopedRoleAssignmentInput) (ScopedRoleAssignment, error) {
	if err := validatePositive("company", input.CompanyID); err != nil {
		return ScopedRoleAssignment{}, err
	}
	if err := validatePositive("user", input.UserID); err != nil {
		return ScopedRoleAssignment{}, err
	}
	if err := validatePositive("role", input.RoleID); err != nil {
		return ScopedRoleAssignment{}, err
	}
	if err := validateBranch(input.BranchID); err != nil {
		return ScopedRoleAssignment{}, err
	}
	validFrom := input.ValidFrom
	if validFrom.IsZero() {
		validFrom = time.Now().UTC()
	} else {
		validFrom = validFrom.UTC()
	}
	input.ValidFrom = validFrom
	if input.ValidTo != nil {
		validTo := input.ValidTo.UTC()
		if !validTo.After(validFrom) {
			return ScopedRoleAssignment{}, fmt.Errorf("rbac: valid_to must be after valid_from")
		}
		input.ValidTo = &validTo
	}
	if s.scopedRepo == nil {
		return ScopedRoleAssignment{}, ErrScopedRepositoryUnavailable
	}
	return s.scopedRepo.CreateScopedRoleAssignment(ctx, input)
}

// ListRoleAssignmentsInCompany never returns assignments from another company.
func (s *Service) ListRoleAssignmentsInCompany(ctx context.Context, companyID, userID int64) ([]ScopedRoleAssignment, error) {
	if err := validatePositive("company", companyID); err != nil {
		return nil, err
	}
	if err := validatePositive("user", userID); err != nil {
		return nil, err
	}
	if s.scopedRepo == nil {
		return nil, ErrScopedRepositoryUnavailable
	}
	return s.scopedRepo.ListScopedRoleAssignments(ctx, companyID, userID)
}

// RemoveRoleAssignmentInCompany deletes only an assignment belonging to the
// supplied company, preventing an ID from crossing tenant boundaries.
func (s *Service) RemoveRoleAssignmentInCompany(ctx context.Context, companyID, assignmentID int64) error {
	if err := validatePositive("company", companyID); err != nil {
		return err
	}
	if err := validatePositive("assignment", assignmentID); err != nil {
		return err
	}
	if s.scopedRepo == nil {
		return ErrScopedRepositoryUnavailable
	}
	deleted, err := s.scopedRepo.DeleteScopedRoleAssignment(ctx, companyID, assignmentID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrNotFound
	}
	return nil
}

// EffectivePermissionsInScope evaluates only active scoped assignments. The
// interval is [valid_from, valid_to), and a company-level lookup excludes
// branch-only assignments. Legacy global permissions remain on EffectivePermissions.
func (s *Service) EffectivePermissionsInScope(ctx context.Context, userID int64, scope AccessScope, at time.Time) ([]string, error) {
	if err := validatePositive("user", userID); err != nil {
		return nil, err
	}
	if err := validateScope(scope); err != nil {
		return nil, err
	}
	if at.IsZero() {
		return nil, ErrInvalidEffectiveTime
	}
	if s.scopedRepo == nil {
		return nil, ErrScopedRepositoryUnavailable
	}
	return s.scopedRepo.EffectivePermissionsInScope(ctx, userID, scope, at.UTC())
}

// OpenAccessReview is idempotent for (company, subject user, review key): a
// retry returns the original review, including if it has already completed.
func (s *Service) OpenAccessReview(ctx context.Context, input OpenAccessReviewInput) (AccessReview, error) {
	if err := validatePositive("company", input.CompanyID); err != nil {
		return AccessReview{}, err
	}
	if err := validatePositive("subject user", input.SubjectUserID); err != nil {
		return AccessReview{}, err
	}
	if err := validatePositive("opened by user", input.OpenedByUserID); err != nil {
		return AccessReview{}, err
	}
	input.ReviewKey = strings.TrimSpace(input.ReviewKey)
	if input.ReviewKey == "" {
		return AccessReview{}, errors.New("rbac: access review key required")
	}
	if s.scopedRepo == nil {
		return AccessReview{}, ErrScopedRepositoryUnavailable
	}
	return s.scopedRepo.OpenAccessReview(ctx, input)
}

func (s *Service) GetAccessReview(ctx context.Context, companyID, reviewID int64) (AccessReview, error) {
	if err := validatePositive("company", companyID); err != nil {
		return AccessReview{}, err
	}
	if err := validatePositive("review", reviewID); err != nil {
		return AccessReview{}, err
	}
	if s.scopedRepo == nil {
		return AccessReview{}, ErrScopedRepositoryUnavailable
	}
	return s.scopedRepo.GetAccessReview(ctx, companyID, reviewID)
}

func (s *Service) ListOpenAccessReviews(ctx context.Context, companyID int64) ([]AccessReview, error) {
	if err := validatePositive("company", companyID); err != nil {
		return nil, err
	}
	if s.scopedRepo == nil {
		return nil, ErrScopedRepositoryUnavailable
	}
	return s.scopedRepo.ListOpenAccessReviews(ctx, companyID)
}

// DecideAccessReview atomically completes an open review. Repeating the same
// decision by the same reviewer is idempotent; a conflicting repeat is closed.
func (s *Service) DecideAccessReview(ctx context.Context, companyID, reviewID, decidedByUserID int64, decision AccessReviewDecision) (AccessReview, error) {
	if err := validatePositive("company", companyID); err != nil {
		return AccessReview{}, err
	}
	if err := validatePositive("review", reviewID); err != nil {
		return AccessReview{}, err
	}
	if err := validatePositive("decided by user", decidedByUserID); err != nil {
		return AccessReview{}, err
	}
	decision = AccessReviewDecision(strings.ToUpper(strings.TrimSpace(string(decision))))
	if decision != AccessReviewApprove && decision != AccessReviewRevoke {
		return AccessReview{}, ErrInvalidAccessReviewDecision
	}
	if s.scopedRepo == nil {
		return AccessReview{}, ErrScopedRepositoryUnavailable
	}
	return s.scopedRepo.CompleteAccessReview(ctx, companyID, reviewID, decidedByUserID, decision)
}

func validateScope(scope AccessScope) error {
	if err := validatePositive("company", scope.CompanyID); err != nil {
		return ErrInvalidScope
	}
	if err := validateBranch(scope.BranchID); err != nil {
		return ErrInvalidScope
	}
	return nil
}

func validateBranch(branchID *int64) error {
	if branchID != nil && *branchID <= 0 {
		return ErrInvalidScope
	}
	return nil
}

func validatePositive(name string, value int64) error {
	if value <= 0 {
		return fmt.Errorf("rbac: %s id must be positive", name)
	}
	return nil
}
