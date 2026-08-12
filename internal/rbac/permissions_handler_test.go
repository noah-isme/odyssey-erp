package rbac

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type permissionsHandlerRepository struct {
	Repository
	assignment       ScopedRoleAssignment
	assignmentInput  ScopedRoleAssignmentInput
	listCompanyID    int64
	listUserID       int64
	deleteCompanyID  int64
	deleteAssignment int64
	review           AccessReview
	reviewInput      OpenAccessReviewInput
	decision         AccessReviewDecision
	decisionCompany  int64
	decisionReviewID int64
	decisionActorID  int64
}

func (f *permissionsHandlerRepository) CreateScopedRoleAssignment(_ context.Context, input ScopedRoleAssignmentInput) (ScopedRoleAssignment, error) {
	f.assignmentInput = input
	assignment := f.assignment
	assignment.CompanyID = input.CompanyID
	assignment.UserID = input.UserID
	assignment.RoleID = input.RoleID
	assignment.BranchID = input.BranchID
	assignment.ValidFrom = input.ValidFrom
	assignment.ValidTo = input.ValidTo
	return assignment, nil
}

func (f *permissionsHandlerRepository) ListScopedRoleAssignments(_ context.Context, companyID, userID int64) ([]ScopedRoleAssignment, error) {
	f.listCompanyID = companyID
	f.listUserID = userID
	return []ScopedRoleAssignment{f.assignment}, nil
}

func (f *permissionsHandlerRepository) DeleteScopedRoleAssignment(_ context.Context, companyID, assignmentID int64) (bool, error) {
	f.deleteCompanyID = companyID
	f.deleteAssignment = assignmentID
	return true, nil
}

func (f *permissionsHandlerRepository) EffectivePermissionsInScope(context.Context, int64, AccessScope, time.Time) ([]string, error) {
	return []string{shared.PermPermissionsAssign, shared.PermPermissionsReview}, nil
}

func (f *permissionsHandlerRepository) OpenAccessReview(_ context.Context, input OpenAccessReviewInput) (AccessReview, error) {
	f.reviewInput = input
	review := f.review
	review.CompanyID = input.CompanyID
	review.SubjectUserID = input.SubjectUserID
	review.ReviewKey = input.ReviewKey
	review.OpenedByUserID = input.OpenedByUserID
	return review, nil
}

func (f *permissionsHandlerRepository) GetAccessReview(context.Context, int64, int64) (AccessReview, error) {
	return f.review, nil
}

func (f *permissionsHandlerRepository) ListOpenAccessReviews(context.Context, int64) ([]AccessReview, error) {
	return []AccessReview{f.review}, nil
}

func (f *permissionsHandlerRepository) CompleteAccessReview(_ context.Context, companyID, reviewID, actorID int64, decision AccessReviewDecision) (AccessReview, error) {
	f.decisionCompany = companyID
	f.decisionReviewID = reviewID
	f.decisionActorID = actorID
	f.decision = decision
	review := f.review
	review.CompanyID = companyID
	review.ID = reviewID
	review.Decision = decision
	review.Status = AccessReviewCompleted
	return review, nil
}

func permissionsHandlerRouter(repo *permissionsHandlerRepository) *chi.Mux {
	service := NewService(repo)
	reader := &stubScopedPermissionReader{scopedPermissions: []string{
		shared.PermPermissionsAssign,
		shared.PermPermissionsReview,
	}}
	handler := NewPermissionsHandler(nil, service, nil, nil, nil, Middleware{Service: reader})
	router := chi.NewRouter()
	handler.MountRoutes(router)
	return router
}

func permissionsHandlerRequest(t *testing.T, method, path, userID, companyID string, body io.Reader) *http.Request {
	t.Helper()
	request := requestWithSession(t, userID, companyID, "")
	request.Method = method
	request.URL.Path = path
	request.Body = io.NopCloser(body)
	return request
}

func TestScopedAssignmentEndpointUsesActiveCompany(t *testing.T) {
	repo := &permissionsHandlerRepository{}
	router := permissionsHandlerRouter(repo)
	form := url.Values{
		"user_id":    {"12"},
		"role_id":    {"3"},
		"company_id": {"999"}, // must be ignored; company comes from the session.
	}
	request := permissionsHandlerRequest(t, http.MethodPost, "/scoped-assignments", "42", "7", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	if repo.assignmentInput.CompanyID != 7 || repo.assignmentInput.UserID != 12 || repo.assignmentInput.RoleID != 3 {
		t.Fatalf("assignment input = %#v, want company 7/user 12/role 3", repo.assignmentInput)
	}
}

func TestScopedAssignmentListAndDeleteRemainCompanyBound(t *testing.T) {
	repo := &permissionsHandlerRepository{}
	router := permissionsHandlerRouter(repo)

	listRequest := permissionsHandlerRequest(t, http.MethodGet, "/scoped-assignments", "42", "7", strings.NewReader(""))
	listRequest.URL.RawQuery = "user_id=12"
	listRecorder := httptest.NewRecorder()
	router.ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRecorder.Code, http.StatusOK)
	}
	if repo.listCompanyID != 7 || repo.listUserID != 12 {
		t.Fatalf("list scope = company %d/user %d, want company 7/user 12", repo.listCompanyID, repo.listUserID)
	}

	deleteRequest := permissionsHandlerRequest(t, http.MethodDelete, "/scoped-assignments/55", "42", "9", strings.NewReader(""))
	deleteRecorder := httptest.NewRecorder()
	router.ServeHTTP(deleteRecorder, deleteRequest)
	if deleteRecorder.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", deleteRecorder.Code, http.StatusNoContent)
	}
	if repo.deleteCompanyID != 9 || repo.deleteAssignment != 55 {
		t.Fatalf("delete scope = company %d/assignment %d, want company 9/assignment 55", repo.deleteCompanyID, repo.deleteAssignment)
	}
}

func TestAccessReviewEndpointsUseAuthenticatedActorAndCompany(t *testing.T) {
	repo := &permissionsHandlerRepository{}
	router := permissionsHandlerRouter(repo)

	openRequest := permissionsHandlerRequest(t, http.MethodPost, "/access-reviews", "42", "7", strings.NewReader(`{"subject_user_id":12,"review_key":"2026-Q3"}`))
	openRequest.Header.Set("Content-Type", "application/json")
	openRecorder := httptest.NewRecorder()
	router.ServeHTTP(openRecorder, openRequest)
	if openRecorder.Code != http.StatusCreated {
		t.Fatalf("open status = %d, want %d (body %s)", openRecorder.Code, http.StatusCreated, openRecorder.Body.String())
	}
	if repo.reviewInput.CompanyID != 7 || repo.reviewInput.SubjectUserID != 12 || repo.reviewInput.OpenedByUserID != 42 {
		t.Fatalf("review input = %#v, want company 7/subject 12/actor 42", repo.reviewInput)
	}

	decisionRequest := permissionsHandlerRequest(t, http.MethodPost, "/access-reviews/18/decision", "42", "7", strings.NewReader("decision=approve"))
	decisionRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	decisionRecorder := httptest.NewRecorder()
	router.ServeHTTP(decisionRecorder, decisionRequest)
	if decisionRecorder.Code != http.StatusOK {
		t.Fatalf("decision status = %d, want %d (body %s)", decisionRecorder.Code, http.StatusOK, decisionRecorder.Body.String())
	}
	if repo.decisionCompany != 7 || repo.decisionReviewID != 18 || repo.decisionActorID != 42 || repo.decision != AccessReviewApprove {
		t.Fatalf("decision = company %d/review %d/actor %d/decision %q", repo.decisionCompany, repo.decisionReviewID, repo.decisionActorID, repo.decision)
	}
}

func TestScopedAdminEndpointsRejectMissingTenantIdentity(t *testing.T) {
	repo := &permissionsHandlerRepository{}
	router := permissionsHandlerRouter(repo)
	request := httptest.NewRequest(http.MethodGet, "/access-reviews", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}
