package approvals

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type failingFinalizer struct{ err error }

func (f failingFinalizer) FinalizeApproval(context.Context, Request, string, int64, string) error {
	return f.err
}

func TestDecideFlashWhenFinalizerFailsAfterCommit(t *testing.T) {
	const generic = "Approved, but applying the result failed: an unexpected error occurred. HR can retry from the request page."
	tests := []struct {
		name     string
		action   string
		finalErr error
		want     string
	}{
		{"hr-prefixed error is shown", "approve", errors.New("hr: insufficient leave balance"),
			"Approved, but applying the result failed: insufficient leave balance. HR can retry from the request page."},
		{"trailing period is not doubled", "approve", errors.New("hr: leave balance not configured."),
			"Approved, but applying the result failed: leave balance not configured. HR can retry from the request page."},
		{"reject says rejected", "reject", errors.New("hr: invalid leave request"),
			"Rejected, but applying the result failed: invalid leave request. HR can retry from the request page."},
		{"non-hr error is hidden", "approve", errors.New(`pq: duplicate key value violates unique constraint "hr_leave_requests_pkey"`), generic},
		{"wrapped hr error with detail is hidden", "approve", fmt.Errorf("hr: apply leave: %w", errors.New("connection reset by peer")), generic},
		{"multi-line hr error is hidden", "approve", errors.New("hr: bad\nSELECT secret"), generic},
		{"wrapped hr sentinel is shown", "approve", fmt.Errorf("%w: overtime request #5 stays pending", errors.New("hr: payroll run awaiting approval")),
			"Approved, but applying the result failed: payroll run awaiting approval. HR can retry from the request page."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newMemoryStore(Policy{ID: 1, Module: "LEAVE", Steps: []PolicyStep{{Order: 1, ApproverManager: true}}})
			service := NewService(store, nil)
			service.RegisterFinalizer("LEAVE", failingFinalizer{err: tc.finalErr})
			req, err := service.Submit(context.Background(), Submission{Module: "LEAVE", DocumentID: 9, RequesterID: 7, ManagerID: 50})
			require.NoError(t, err)

			h := &Handler{logger: slog.Default(), service: service}
			router := chi.NewRouter()
			router.Post("/{id}/approve", h.approve)
			router.Post("/{id}/reject", h.reject)

			manager := shared.NewSessionManager(nil, "session", "secret", time.Hour, false)
			hreq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/%d/%s", req.ID, tc.action), strings.NewReader(""))
			session, err := manager.Load(hreq.Context(), hreq)
			require.NoError(t, err)
			session.SetUser("50")
			hreq = hreq.WithContext(shared.ContextWithSession(hreq.Context(), session))

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, hreq)

			require.Equal(t, http.StatusSeeOther, rec.Code)
			require.Equal(t, "/approvals", rec.Header().Get("Location"))
			flash := session.PopFlash()
			require.NotNil(t, flash)
			require.Equal(t, "error", flash.Kind)
			require.Equal(t, tc.want, flash.Message)
			require.NotContains(t, flash.Message, "pq:")
			require.NotContains(t, flash.Message, "SELECT")
			require.NotContains(t, flash.Message, "connection reset")
		})
	}
}

func TestDecideFlashSuccessAndNonFinalizerErrorsUnchanged(t *testing.T) {
	manager := shared.NewSessionManager(nil, "session", "secret", time.Hour, false)
	run := func(finalizer Finalizer, user string) *shared.FlashMessage {
		store := newMemoryStore(Policy{ID: 1, Module: "LEAVE", Steps: []PolicyStep{{Order: 1, ApproverManager: true}}})
		service := NewService(store, nil)
		if finalizer != nil {
			service.RegisterFinalizer("LEAVE", finalizer)
		}
		req, err := service.Submit(context.Background(), Submission{Module: "LEAVE", DocumentID: 9, RequesterID: 7, ManagerID: 50})
		require.NoError(t, err)
		h := &Handler{logger: slog.Default(), service: service}
		router := chi.NewRouter()
		router.Post("/{id}/approve", h.approve)
		hreq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/%d/approve", req.ID), strings.NewReader(""))
		session, err := manager.Load(hreq.Context(), hreq)
		require.NoError(t, err)
		session.SetUser(user)
		hreq = hreq.WithContext(shared.ContextWithSession(hreq.Context(), session))
		router.ServeHTTP(httptest.NewRecorder(), hreq)
		return session.PopFlash()
	}

	flash := run(failingFinalizer{}, "50")
	require.NotNil(t, flash)
	require.Equal(t, "success", flash.Kind)
	require.Equal(t, "Approval decision recorded", flash.Message)

	// A decision that never committed (not the assignee) keeps the generic flash.
	flash = run(failingFinalizer{err: errors.New("hr: unreachable")}, "999")
	require.NotNil(t, flash)
	require.Equal(t, "error", flash.Kind)
	require.NotContains(t, flash.Message, "applying the result failed")
}

type decideErrStore struct {
	*memoryStore
	err error
}

func (s decideErrStore) Decide(context.Context, int64, int64, string, string) (DecisionResult, error) {
	return DecisionResult{}, s.err
}

func TestDecideFlashForUserCausedErrors(t *testing.T) {
	manager := shared.NewSessionManager(nil, "session", "secret", time.Hour, false)
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"double approve", ErrAlreadyDecided, "This request has already been decided."},
		{"late second approver", ErrNotAssigned, "This request is not assigned to you."},
		{"unknown request", pgx.ErrNoRows, "The requested item was not found."},
		{"bad input", ErrInvalid, "The provided input is invalid."},
		{"internal error stays generic", errors.New("pq: connection reset"), "An unexpected error occurred. Please try again."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{logger: slog.Default(), service: NewService(decideErrStore{newMemoryStore(), tc.err}, nil)}
			router := chi.NewRouter()
			router.Post("/{id}/approve", h.approve)
			hreq := httptest.NewRequest(http.MethodPost, "/5/approve", strings.NewReader(""))
			session, err := manager.Load(hreq.Context(), hreq)
			require.NoError(t, err)
			session.SetUser("50")
			hreq = hreq.WithContext(shared.ContextWithSession(hreq.Context(), session))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, hreq)
			require.Equal(t, http.StatusSeeOther, rec.Code)
			flash := session.PopFlash()
			require.NotNil(t, flash)
			require.Equal(t, "error", flash.Kind)
			require.Equal(t, tc.want, flash.Message)
			require.NotContains(t, flash.Message, "pq:")
		})
	}
}
