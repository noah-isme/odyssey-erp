package rbac

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type scopedRepositoryFake struct {
	Repository
	assignmentInput ScopedRoleAssignmentInput
	scope           AccessScope
	at              time.Time
	openInput       OpenAccessReviewInput
	decision        AccessReviewDecision
	completeResult  AccessReview
}

func (f *scopedRepositoryFake) CreateScopedRoleAssignment(_ context.Context, input ScopedRoleAssignmentInput) (ScopedRoleAssignment, error) {
	f.assignmentInput = input
	return ScopedRoleAssignment{CompanyID: input.CompanyID, UserID: input.UserID, RoleID: input.RoleID, ValidFrom: input.ValidFrom, ValidTo: input.ValidTo}, nil
}

func (f *scopedRepositoryFake) ListScopedRoleAssignments(context.Context, int64, int64) ([]ScopedRoleAssignment, error) {
	return nil, nil
}

func (f *scopedRepositoryFake) DeleteScopedRoleAssignment(context.Context, int64, int64) (bool, error) {
	return true, nil
}

func (f *scopedRepositoryFake) EffectivePermissionsInScope(_ context.Context, _ int64, scope AccessScope, at time.Time) ([]string, error) {
	f.scope = scope
	f.at = at
	return []string{"inventory.view"}, nil
}

func (f *scopedRepositoryFake) OpenAccessReview(_ context.Context, input OpenAccessReviewInput) (AccessReview, error) {
	f.openInput = input
	return AccessReview{CompanyID: input.CompanyID, SubjectUserID: input.SubjectUserID, ReviewKey: input.ReviewKey, Status: AccessReviewOpen}, nil
}

func (f *scopedRepositoryFake) GetAccessReview(context.Context, int64, int64) (AccessReview, error) {
	return f.completeResult, nil
}

func (f *scopedRepositoryFake) ListOpenAccessReviews(context.Context, int64) ([]AccessReview, error) {
	return nil, nil
}

func (f *scopedRepositoryFake) CompleteAccessReview(_ context.Context, _ int64, _ int64, _ int64, decision AccessReviewDecision) (AccessReview, error) {
	f.decision = decision
	return f.completeResult, nil
}

func TestAssignRoleInScopeNormalizesEffectiveDates(t *testing.T) {
	repo := &scopedRepositoryFake{}
	service := NewService(repo)
	validTo := time.Date(2026, 8, 20, 0, 0, 0, 0, time.FixedZone("UTC+7", 7*60*60))
	assignment, err := service.AssignRoleInScope(context.Background(), ScopedRoleAssignmentInput{
		CompanyID: 1,
		UserID:    2,
		RoleID:    3,
		ValidFrom: time.Date(2026, 8, 10, 0, 0, 0, 0, time.FixedZone("UTC+7", 7*60*60)),
		ValidTo:   &validTo,
	})
	if err != nil {
		t.Fatalf("AssignRoleInScope() error = %v", err)
	}
	if !assignment.ValidFrom.Equal(repo.assignmentInput.ValidFrom) || assignment.ValidTo == nil || repo.assignmentInput.ValidTo == nil || !assignment.ValidTo.Equal(*repo.assignmentInput.ValidTo) {
		t.Fatalf("assignment dates = %#v, input = %#v", assignment, repo.assignmentInput)
	}
	if repo.assignmentInput.ValidFrom.Location() != time.UTC || repo.assignmentInput.ValidTo.Location() != time.UTC {
		t.Fatalf("dates were not normalized to UTC: %#v", repo.assignmentInput)
	}
}

func TestEffectivePermissionsInScopeRequiresExplicitTimeAndScope(t *testing.T) {
	service := NewService(&scopedRepositoryFake{})
	if _, err := service.EffectivePermissionsInScope(context.Background(), 1, AccessScope{CompanyID: 1}, time.Time{}); !errors.Is(err, ErrInvalidEffectiveTime) {
		t.Fatalf("zero time error = %v, want %v", err, ErrInvalidEffectiveTime)
	}
	branch := int64(0)
	if _, err := service.EffectivePermissionsInScope(context.Background(), 1, AccessScope{CompanyID: 1, BranchID: &branch}, time.Now()); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("invalid branch error = %v, want %v", err, ErrInvalidScope)
	}
}

func TestEffectivePermissionsInScopePassesTenantAndEvaluationTime(t *testing.T) {
	repo := &scopedRepositoryFake{}
	service := NewService(repo)
	branch := int64(7)
	at := time.Date(2026, 8, 12, 10, 0, 0, 0, time.FixedZone("UTC+7", 7*60*60))
	permissions, err := service.EffectivePermissionsInScope(context.Background(), 4, AccessScope{CompanyID: 9, BranchID: &branch}, at)
	if err != nil {
		t.Fatalf("EffectivePermissionsInScope() error = %v", err)
	}
	if !reflect.DeepEqual(permissions, []string{"inventory.view"}) {
		t.Fatalf("permissions = %#v", permissions)
	}
	if repo.scope.CompanyID != 9 || repo.scope.BranchID == nil || *repo.scope.BranchID != 7 || !repo.at.Equal(at.UTC()) {
		t.Fatalf("scope/evaluation = %#v at %v", repo.scope, repo.at)
	}
}

func TestOpenAccessReviewTrimsIdempotencyKey(t *testing.T) {
	repo := &scopedRepositoryFake{}
	service := NewService(repo)
	review, err := service.OpenAccessReview(context.Background(), OpenAccessReviewInput{
		CompanyID:      8,
		SubjectUserID:  12,
		ReviewKey:      "  2026-Q3  ",
		OpenedByUserID: 3,
	})
	if err != nil {
		t.Fatalf("OpenAccessReview() error = %v", err)
	}
	if repo.openInput.ReviewKey != "2026-Q3" || review.ReviewKey != "2026-Q3" {
		t.Fatalf("review key = %q, input = %q", review.ReviewKey, repo.openInput.ReviewKey)
	}
}

func TestDecideAccessReviewValidatesAndNormalizesDecision(t *testing.T) {
	repo := &scopedRepositoryFake{completeResult: AccessReview{Status: AccessReviewCompleted}}
	service := NewService(repo)
	if _, err := service.DecideAccessReview(context.Background(), 8, 10, 3, " retain "); !errors.Is(err, ErrInvalidAccessReviewDecision) {
		t.Fatalf("invalid decision error = %v, want %v", err, ErrInvalidAccessReviewDecision)
	}
	if _, err := service.DecideAccessReview(context.Background(), 8, 10, 3, " revoke "); err != nil {
		t.Fatalf("DecideAccessReview() error = %v", err)
	}
	if repo.decision != AccessReviewRevoke {
		t.Fatalf("decision = %q, want %q", repo.decision, AccessReviewRevoke)
	}
}

func TestScopedAPIsFailClosedWithoutScopedRepository(t *testing.T) {
	legacy := &legacyRepositoryFake{}
	service := NewService(legacy)
	_, err := service.EffectivePermissionsInScope(context.Background(), 1, AccessScope{CompanyID: 1}, time.Now())
	if !errors.Is(err, ErrScopedRepositoryUnavailable) {
		t.Fatalf("error = %v, want %v", err, ErrScopedRepositoryUnavailable)
	}
}

type legacyRepositoryFake struct{}

func (legacyRepositoryFake) ListRoles(context.Context) ([]Role, error)    { return nil, nil }
func (legacyRepositoryFake) GetRole(context.Context, int64) (Role, error) { return Role{}, nil }
func (legacyRepositoryFake) CreateRole(context.Context, string, string) (Role, error) {
	return Role{}, nil
}
func (legacyRepositoryFake) UpdateRole(context.Context, int64, string, string) (Role, error) {
	return Role{}, nil
}
func (legacyRepositoryFake) DeleteRole(context.Context, int64) (bool, error)       { return false, nil }
func (legacyRepositoryFake) ListPermissions(context.Context) ([]Permission, error) { return nil, nil }
func (legacyRepositoryFake) EnsurePermission(context.Context, string, string) (Permission, error) {
	return Permission{}, nil
}
func (legacyRepositoryFake) ListRolePermissions(context.Context, int64) ([]Permission, error) {
	return nil, nil
}
func (legacyRepositoryFake) AttachPermissionToRole(context.Context, int64, int64) error   { return nil }
func (legacyRepositoryFake) DetachPermissionFromRole(context.Context, int64, int64) error { return nil }
func (legacyRepositoryFake) AssignRoleToUser(context.Context, int64, int64) error         { return nil }
func (legacyRepositoryFake) RemoveRoleFromUser(context.Context, int64, int64) error       { return nil }
func (legacyRepositoryFake) EffectivePermissions(context.Context, int64) ([]string, error) {
	return nil, nil
}
