package rbac

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// PGRepository adapts generated RBAC persistence types to domain types.
type PGRepository struct {
	queries *sqlc.Queries
}

func NewRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{queries: sqlc.New(pool)}
}

var _ ScopedRepository = (*PGRepository)(nil)

func (r *PGRepository) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := r.queries.RbacListRoles(ctx)
	if err != nil {
		return nil, err
	}
	roles := make([]Role, len(rows))
	for i, row := range rows {
		roles[i] = mapRole(row)
	}
	return roles, nil
}

func (r *PGRepository) GetRole(ctx context.Context, id int64) (Role, error) {
	row, err := r.queries.GetRole(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Role{}, ErrNotFound
	}
	if err != nil {
		return Role{}, err
	}
	return mapRole(row), nil
}

func (r *PGRepository) CreateRole(ctx context.Context, name, description string) (Role, error) {
	row, err := r.queries.RbacCreateRole(ctx, sqlc.RbacCreateRoleParams{Name: name, Description: description})
	if err != nil {
		return Role{}, err
	}
	return mapRole(row), nil
}

func (r *PGRepository) UpdateRole(ctx context.Context, id int64, name, description string) (Role, error) {
	row, err := r.queries.UpdateRole(ctx, sqlc.UpdateRoleParams{ID: id, Name: name, Description: description})
	if errors.Is(err, pgx.ErrNoRows) {
		return Role{}, ErrNotFound
	}
	if err != nil {
		return Role{}, err
	}
	return mapRole(row), nil
}

func (r *PGRepository) DeleteRole(ctx context.Context, id int64) (bool, error) {
	rows, err := r.queries.DeleteRole(ctx, id)
	return rows > 0, err
}

func (r *PGRepository) ListPermissions(ctx context.Context) ([]Permission, error) {
	rows, err := r.queries.ListPermissions(ctx)
	if err != nil {
		return nil, err
	}
	permissions := make([]Permission, len(rows))
	for i, row := range rows {
		permissions[i] = Permission{ID: row.ID, Name: row.Name, Description: row.Description}
	}
	return permissions, nil
}

func (r *PGRepository) EnsurePermission(ctx context.Context, name, description string) (Permission, error) {
	row, err := r.queries.CreatePermission(ctx, sqlc.CreatePermissionParams{Name: name, Description: description})
	if err != nil {
		return Permission{}, err
	}
	return Permission{ID: row.ID, Name: row.Name, Description: row.Description}, nil
}

func (r *PGRepository) ListRolePermissions(ctx context.Context, roleID int64) ([]Permission, error) {
	rows, err := r.queries.ListRolePermissions(ctx, roleID)
	if err != nil {
		return nil, err
	}
	permissions := make([]Permission, len(rows))
	for i, row := range rows {
		permissions[i] = Permission{ID: row.ID, Name: row.Name, Description: row.Description}
	}
	return permissions, nil
}

func (r *PGRepository) AttachPermissionToRole(ctx context.Context, roleID, permissionID int64) error {
	return r.queries.AttachPermissionToRole(ctx, sqlc.AttachPermissionToRoleParams{RoleID: roleID, PermissionID: permissionID})
}

func (r *PGRepository) DetachPermissionFromRole(ctx context.Context, roleID, permissionID int64) error {
	return r.queries.DetachPermissionFromRole(ctx, sqlc.DetachPermissionFromRoleParams{RoleID: roleID, PermissionID: permissionID})
}

func (r *PGRepository) AssignRoleToUser(ctx context.Context, userID, roleID int64) error {
	return r.queries.AssignRoleToUser(ctx, sqlc.AssignRoleToUserParams{UserID: userID, RoleID: roleID})
}

func (r *PGRepository) RemoveRoleFromUser(ctx context.Context, userID, roleID int64) error {
	return r.queries.RemoveRoleFromUser(ctx, sqlc.RemoveRoleFromUserParams{UserID: userID, RoleID: roleID})
}

func (r *PGRepository) EffectivePermissions(ctx context.Context, userID int64) ([]string, error) {
	return r.queries.UserEffectivePermissions(ctx, userID)
}

func (r *PGRepository) CreateScopedRoleAssignment(ctx context.Context, input ScopedRoleAssignmentInput) (ScopedRoleAssignment, error) {
	row, err := r.queries.RbacCreateScopedRoleAssignment(ctx, sqlc.RbacCreateScopedRoleAssignmentParams{
		CompanyID: input.CompanyID,
		UserID:    input.UserID,
		RoleID:    input.RoleID,
		BranchID:  nullableInt8(input.BranchID),
		ValidFrom: timestamptz(input.ValidFrom),
		ValidTo:   nullableTimestamptz(input.ValidTo),
	})
	if err != nil {
		return ScopedRoleAssignment{}, err
	}
	return mapScopedRoleAssignment(row), nil
}

func (r *PGRepository) ListScopedRoleAssignments(ctx context.Context, companyID, userID int64) ([]ScopedRoleAssignment, error) {
	rows, err := r.queries.RbacListScopedRoleAssignments(ctx, sqlc.RbacListScopedRoleAssignmentsParams{
		CompanyID: companyID,
		UserID:    userID,
	})
	if err != nil {
		return nil, err
	}
	assignments := make([]ScopedRoleAssignment, len(rows))
	for i, row := range rows {
		assignments[i] = mapScopedRoleAssignment(row)
	}
	return assignments, nil
}

func (r *PGRepository) DeleteScopedRoleAssignment(ctx context.Context, companyID, assignmentID int64) (bool, error) {
	rows, err := r.queries.RbacDeleteScopedRoleAssignment(ctx, sqlc.RbacDeleteScopedRoleAssignmentParams{
		CompanyID: companyID,
		ID:        assignmentID,
	})
	return rows > 0, err
}

func (r *PGRepository) EffectivePermissionsInScope(ctx context.Context, userID int64, scope AccessScope, at time.Time) ([]string, error) {
	return r.queries.RbacEffectivePermissionsInScope(ctx, sqlc.RbacEffectivePermissionsInScopeParams{
		UserID:    userID,
		CompanyID: scope.CompanyID,
		At:        timestamptz(at),
		BranchID:  nullableInt8(scope.BranchID),
	})
}

func (r *PGRepository) OpenAccessReview(ctx context.Context, input OpenAccessReviewInput) (AccessReview, error) {
	row, err := r.queries.RbacOpenAccessReview(ctx, sqlc.RbacOpenAccessReviewParams{
		CompanyID:      input.CompanyID,
		SubjectUserID:  input.SubjectUserID,
		ReviewKey:      input.ReviewKey,
		OpenedByUserID: input.OpenedByUserID,
	})
	if err != nil {
		return AccessReview{}, err
	}
	return mapAccessReview(row), nil
}

func (r *PGRepository) GetAccessReview(ctx context.Context, companyID, reviewID int64) (AccessReview, error) {
	row, err := r.queries.RbacGetAccessReview(ctx, sqlc.RbacGetAccessReviewParams{
		CompanyID: companyID,
		ID:        reviewID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return AccessReview{}, ErrNotFound
	}
	if err != nil {
		return AccessReview{}, err
	}
	return mapAccessReview(row), nil
}

func (r *PGRepository) ListOpenAccessReviews(ctx context.Context, companyID int64) ([]AccessReview, error) {
	rows, err := r.queries.RbacListOpenAccessReviews(ctx, companyID)
	if err != nil {
		return nil, err
	}
	reviews := make([]AccessReview, len(rows))
	for i, row := range rows {
		reviews[i] = mapAccessReview(row)
	}
	return reviews, nil
}

func (r *PGRepository) CompleteAccessReview(ctx context.Context, companyID, reviewID, decidedByUserID int64, decision AccessReviewDecision) (AccessReview, error) {
	row, err := r.queries.RbacCompleteAccessReview(ctx, sqlc.RbacCompleteAccessReviewParams{
		Decision:        pgtype.Text{String: string(decision), Valid: true},
		DecidedByUserID: pgtype.Int8{Int64: decidedByUserID, Valid: true},
		CompanyID:       companyID,
		ID:              reviewID,
	})
	if err == nil {
		return mapAccessReview(row), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return AccessReview{}, err
	}

	// A retry after a successful concurrent completion is idempotent only when
	// it repeats the same decision and reviewer. A different decision fails
	// closed instead of reopening or overwriting the review.
	existing, getErr := r.GetAccessReview(ctx, companyID, reviewID)
	if getErr != nil {
		return AccessReview{}, getErr
	}
	if existing.Status == AccessReviewCompleted {
		if existing.Decision == decision && existing.DecidedByUserID != nil && *existing.DecidedByUserID == decidedByUserID {
			return existing, nil
		}
		return AccessReview{}, ErrAccessReviewClosed
	}
	return AccessReview{}, ErrNotFound
}

func mapRole(row sqlc.Role) Role {
	return Role{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		CreatedAt:   safeTime(row.CreatedAt.Time),
		UpdatedAt:   safeTime(row.UpdatedAt.Time),
	}
}

func safeTime(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	return t
}

func mapScopedRoleAssignment(row sqlc.RbacUserRoleAssignment) ScopedRoleAssignment {
	return ScopedRoleAssignment{
		ID:        row.ID,
		CompanyID: row.CompanyID,
		UserID:    row.UserID,
		RoleID:    row.RoleID,
		BranchID:  nullableInt64(row.BranchID),
		ValidFrom: safeTime(row.ValidFrom.Time),
		ValidTo:   nullableTime(row.ValidTo),
		CreatedAt: safeTime(row.CreatedAt.Time),
	}
}

func mapAccessReview(row sqlc.RbacAccessReview) AccessReview {
	decision := AccessReviewDecision("")
	if row.Decision.Valid {
		decision = AccessReviewDecision(row.Decision.String)
	}
	return AccessReview{
		ID:              row.ID,
		CompanyID:       row.CompanyID,
		SubjectUserID:   row.SubjectUserID,
		ReviewKey:       row.ReviewKey,
		Status:          AccessReviewStatus(row.Status),
		Decision:        decision,
		OpenedByUserID:  row.OpenedByUserID,
		DecidedByUserID: nullableInt64(row.DecidedByUserID),
		DecidedAt:       nullableTime(row.DecidedAt),
		CreatedAt:       safeTime(row.CreatedAt.Time),
		UpdatedAt:       safeTime(row.UpdatedAt.Time),
	}
}

func nullableInt8(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func nullableInt64(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	v := value.Int64
	return &v
}

func timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func nullableTimestamptz(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return timestamptz(*value)
}

func nullableTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	v := value.Time
	return &v
}
