package rbac

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// PermissionsHandler manages permission listing.
type PermissionsHandler struct {
	logger    *slog.Logger
	service   *Service
	templates *view.Engine
	csrf      *shared.CSRFManager
	sessions  *shared.SessionManager
	rbac      Middleware
}

// NewPermissionsHandler builds PermissionsHandler instance.
func NewPermissionsHandler(logger *slog.Logger, service *Service, templates *view.Engine, csrf *shared.CSRFManager, sessions *shared.SessionManager, rbac Middleware) *PermissionsHandler {
	return &PermissionsHandler{logger: logger, service: service, templates: templates, csrf: csrf, sessions: sessions, rbac: rbac}
}

func (h *PermissionsHandler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermPermissionsView))
		r.Get("/", h.listPermissions)
	})

	// Scoped administration deliberately uses the tenant-aware middleware. The
	// active company comes from the authenticated session; handlers never accept
	// a company_id from a query string or request body.
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAnyInScope(shared.PermPermissionsAssign))
		r.Get("/scoped-assignments", h.listScopedAssignments)
		r.Post("/scoped-assignments", h.createScopedAssignment)
		r.Delete("/scoped-assignments/{id}", h.deleteScopedAssignment)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAnyInScope(shared.PermPermissionsReview))
		r.Get("/access-reviews", h.listAccessReviews)
		r.Post("/access-reviews", h.openAccessReview)
		r.Post("/access-reviews/{id}/decision", h.decideAccessReview)
	})
}

type formErrors map[string]string

func (h *PermissionsHandler) listPermissions(w http.ResponseWriter, r *http.Request) {
	perms, err := h.service.ListPermissions(r.Context())
	if err != nil {
		if h.logger != nil {
			h.logger.Error("list permissions failed", slog.Any("error", err))
		}
		h.render(w, r, "pages/permissions/list.html", map[string]any{"Errors": formErrors{"general": shared.UserSafeMessage(err)}}, http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/permissions/list.html", map[string]any{"Permissions": perms}, http.StatusOK)
}

// scopedAssignmentRequest is intentionally company-blind. A request can
// select a user, role, branch, and effective dates, but the company is always
// taken from the authenticated tenant identity.
type scopedAssignmentRequest struct {
	UserID    int64  `json:"user_id"`
	RoleID    int64  `json:"role_id"`
	BranchID  *int64 `json:"branch_id"`
	ValidFrom string `json:"valid_from"`
	ValidTo   string `json:"valid_to"`
}

type accessReviewRequest struct {
	SubjectUserID int64  `json:"subject_user_id"`
	ReviewKey     string `json:"review_key"`
}

type accessReviewDecisionRequest struct {
	Decision string `json:"decision"`
}

func (h *PermissionsHandler) listScopedAssignments(w http.ResponseWriter, r *http.Request) {
	identity, ok := scopedRequestIdentity(w, r)
	if !ok {
		return
	}

	userID := identity.UserID
	if raw := strings.TrimSpace(r.URL.Query().Get("user_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			writeJSONError(w, http.StatusBadRequest, "user_id must be a positive integer")
			return
		}
		userID = parsed
	}

	assignments, err := h.service.ListRoleAssignmentsInCompany(r.Context(), identity.CompanyID, userID)
	if err != nil {
		h.logScopedError("list scoped assignments failed", err)
		writeScopedError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, map[string]any{
		"company_id":  identity.CompanyID,
		"user_id":     userID,
		"assignments": assignments,
	})
}

func (h *PermissionsHandler) createScopedAssignment(w http.ResponseWriter, r *http.Request) {
	identity, ok := scopedRequestIdentity(w, r)
	if !ok {
		return
	}

	request, err := decodeScopedAssignmentRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid scoped assignment request")
		return
	}
	validFrom, err := parseOptionalRFC3339(request.ValidFrom)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "valid_from must be an RFC3339 timestamp")
		return
	}
	validTo, err := parseOptionalRFC3339Pointer(request.ValidTo)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "valid_to must be an RFC3339 timestamp")
		return
	}

	assignment, err := h.service.AssignRoleInScope(r.Context(), ScopedRoleAssignmentInput{
		CompanyID: identity.CompanyID,
		UserID:    request.UserID,
		RoleID:    request.RoleID,
		BranchID:  request.BranchID,
		ValidFrom: validFrom,
		ValidTo:   validTo,
	})
	if err != nil {
		h.logScopedError("create scoped assignment failed", err)
		writeScopedError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusCreated, map[string]any{"assignment": assignment})
}

func (h *PermissionsHandler) deleteScopedAssignment(w http.ResponseWriter, r *http.Request) {
	identity, ok := scopedRequestIdentity(w, r)
	if !ok {
		return
	}
	assignmentID, err := positivePathID(r, "id")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "assignment id must be a positive integer")
		return
	}

	if err := h.service.RemoveRoleAssignmentInCompany(r.Context(), identity.CompanyID, assignmentID); err != nil {
		h.logScopedError("delete scoped assignment failed", err)
		writeScopedError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *PermissionsHandler) listAccessReviews(w http.ResponseWriter, r *http.Request) {
	identity, ok := scopedRequestIdentity(w, r)
	if !ok {
		return
	}
	reviews, err := h.service.ListOpenAccessReviews(r.Context(), identity.CompanyID)
	if err != nil {
		h.logScopedError("list access reviews failed", err)
		writeScopedError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, map[string]any{
		"company_id": identity.CompanyID,
		"reviews":    reviews,
	})
}

func (h *PermissionsHandler) openAccessReview(w http.ResponseWriter, r *http.Request) {
	identity, ok := scopedRequestIdentity(w, r)
	if !ok {
		return
	}
	request, err := decodeAccessReviewRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid access review request")
		return
	}
	review, err := h.service.OpenAccessReview(r.Context(), OpenAccessReviewInput{
		CompanyID:      identity.CompanyID,
		SubjectUserID:  request.SubjectUserID,
		ReviewKey:      request.ReviewKey,
		OpenedByUserID: identity.UserID,
	})
	if err != nil {
		h.logScopedError("open access review failed", err)
		writeScopedError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusCreated, map[string]any{"review": review})
}

func (h *PermissionsHandler) decideAccessReview(w http.ResponseWriter, r *http.Request) {
	identity, ok := scopedRequestIdentity(w, r)
	if !ok {
		return
	}
	reviewID, err := positivePathID(r, "id")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "review id must be a positive integer")
		return
	}
	request, err := decodeAccessReviewDecisionRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid access review decision request")
		return
	}
	review, err := h.service.DecideAccessReview(r.Context(), identity.CompanyID, reviewID, identity.UserID, AccessReviewDecision(request.Decision))
	if err != nil {
		h.logScopedError("decide access review failed", err)
		writeScopedError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, map[string]any{"review": review})
}

func scopedRequestIdentity(w http.ResponseWriter, r *http.Request) (shared.RequestIdentity, bool) {
	identity, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusForbidden, "tenant identity required")
		return shared.RequestIdentity{}, false
	}
	return identity, true
}

func decodeScopedAssignmentRequest(r *http.Request) (scopedAssignmentRequest, error) {
	if isJSONRequest(r) {
		var request scopedAssignmentRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			return scopedAssignmentRequest{}, err
		}
		return request, nil
	}
	if err := r.ParseForm(); err != nil {
		return scopedAssignmentRequest{}, err
	}
	userID, err := parseRequiredFormID(r, "user_id")
	if err != nil {
		return scopedAssignmentRequest{}, err
	}
	roleID, err := parseRequiredFormID(r, "role_id")
	if err != nil {
		return scopedAssignmentRequest{}, err
	}
	branchID, err := parseOptionalFormID(r, "branch_id")
	if err != nil {
		return scopedAssignmentRequest{}, err
	}
	return scopedAssignmentRequest{
		UserID:    userID,
		RoleID:    roleID,
		BranchID:  branchID,
		ValidFrom: strings.TrimSpace(r.PostFormValue("valid_from")),
		ValidTo:   strings.TrimSpace(r.PostFormValue("valid_to")),
	}, nil
}

func decodeAccessReviewRequest(r *http.Request) (accessReviewRequest, error) {
	if isJSONRequest(r) {
		var request accessReviewRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			return accessReviewRequest{}, err
		}
		return request, nil
	}
	if err := r.ParseForm(); err != nil {
		return accessReviewRequest{}, err
	}
	subjectUserID, err := parseRequiredFormID(r, "subject_user_id")
	if err != nil {
		return accessReviewRequest{}, err
	}
	return accessReviewRequest{SubjectUserID: subjectUserID, ReviewKey: strings.TrimSpace(r.PostFormValue("review_key"))}, nil
}

func decodeAccessReviewDecisionRequest(r *http.Request) (accessReviewDecisionRequest, error) {
	if isJSONRequest(r) {
		var request accessReviewDecisionRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			return accessReviewDecisionRequest{}, err
		}
		return request, nil
	}
	if err := r.ParseForm(); err != nil {
		return accessReviewDecisionRequest{}, err
	}
	return accessReviewDecisionRequest{Decision: strings.TrimSpace(r.PostFormValue("decision"))}, nil
}

func isJSONRequest(r *http.Request) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))), "application/json")
}

func parseOptionalRFC3339(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func parseOptionalRFC3339Pointer(value string) (*time.Time, error) {
	parsed, err := parseOptionalRFC3339(value)
	if err != nil || parsed.IsZero() {
		return nil, err
	}
	return &parsed, nil
}

func parseRequiredFormID(r *http.Request, field string) (int64, error) {
	value := strings.TrimSpace(r.PostFormValue(field))
	if value == "" {
		return 0, fmt.Errorf("%s is required", field)
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", field)
	}
	return parsed, nil
}

func parseOptionalFormID(r *http.Request, field string) (*int64, error) {
	value := strings.TrimSpace(r.PostFormValue(field))
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return nil, fmt.Errorf("%s must be a positive integer", field)
	}
	return &parsed, nil
}

func positivePathID(r *http.Request, name string) (int64, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(r, name)), 10, 64)
	if err != nil || parsed <= 0 {
		return 0, errors.New("invalid path id")
	}
	return parsed, nil
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	shared.JSONError(w, status, message)
}

func (h *PermissionsHandler) logScopedError(message string, err error) {
	if h.logger != nil {
		h.logger.Error(message, slog.Any("error", err))
	}
}

func writeScopedError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ErrAccessReviewClosed):
		status = http.StatusConflict
	case errors.Is(err, ErrScopedRepositoryUnavailable):
		status = http.StatusServiceUnavailable
	case errors.Is(err, ErrInvalidScope), errors.Is(err, ErrInvalidEffectiveTime), errors.Is(err, ErrInvalidAccessReviewDecision), strings.HasPrefix(err.Error(), "rbac:"):
		status = http.StatusBadRequest
	}
	shared.JSONError(w, status, shared.UserSafeMessage(err))
}

func (h *PermissionsHandler) render(w http.ResponseWriter, r *http.Request, template string, data map[string]any, status int) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	viewData := view.TemplateData{Title: "Permissions", CSRFToken: csrfToken, Flash: flash, CurrentPath: r.URL.Path, Data: data}
	if err := h.templates.RenderStatus(w, template, viewData, status); err != nil {
		h.logger.Error("render template", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}
