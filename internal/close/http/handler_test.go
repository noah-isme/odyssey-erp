package closehttp

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"github.com/odyssey-erp/odyssey-erp/internal/close"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

func TestListPeriodsRendersStatusAndActions(t *testing.T) {
	svc := &stubCloseService{
		listPeriodsFn: func(ctx context.Context, companyID int64, limit, offset int) ([]close.Period, error) {
			if companyID != 1 {
				t.Fatalf("expected company id 1, got %d", companyID)
			}
			return []close.Period{
				{
					ID:        1,
					CompanyID: 1,
					Name:      "2024-01",
					StartDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					EndDate:   time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
					Status:    close.PeriodStatusOpen,
				},
				{
					ID:          2,
					CompanyID:   1,
					Name:        "2023-12",
					StartDate:   time.Date(2023, 12, 1, 0, 0, 0, 0, time.UTC),
					EndDate:     time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC),
					Status:      close.PeriodStatusHardClosed,
					LatestRunID: 99,
				},
			}, nil
		},
	}
	handler, sessions := newTestHandler(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/accounting/periods?company_id=1", nil)
	sess := loadSession(t, sessions, req)
	req = req.WithContext(shared.ContextWithSession(req.Context(), sess))

	rr := httptest.NewRecorder()
	handler.listPeriods(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Mulai Close") {
		t.Fatalf("expected Mulai Close action in response")
	}
	if !strings.Contains(body, "Lihat Close") {
		t.Fatalf("expected Lihat Close link for runs")
	}
	if !strings.Contains(body, "Hard Closed") {
		t.Fatalf("expected status badge in response")
	}
}

func TestStartCloseRunRedirectsOnSuccess(t *testing.T) {
	var captured close.StartCloseRunInput
	svc := &stubCloseService{
		startCloseRunFn: func(ctx context.Context, in close.StartCloseRunInput) (close.CloseRun, error) {
			captured = in
			return close.CloseRun{ID: 55}, nil
		},
	}
	handler, sessions := newTestHandler(t, svc)

	form := url.Values{}
	form.Set("company_id", "7")
	form.Set("notes", "closing remarks")
	req := httptest.NewRequest(http.MethodPost, "/accounting/periods/11/close-run", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	sess := loadSession(t, sessions, req)
	sess.SetUser("99")
	req = req.WithContext(shared.ContextWithSession(req.Context(), sess))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", "11")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))

	rr := httptest.NewRecorder()
	handler.startCloseRun(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rr.Code)
	}
	if got := rr.Header().Get("Location"); got != "/close-runs/55" {
		t.Fatalf("unexpected redirect location %s", got)
	}
	if captured.CompanyID != 7 || captured.PeriodID != 11 || captured.ActorID != 99 {
		t.Fatalf("unexpected captured input: %+v", captured)
	}
	flash := sess.PopFlash()
	if flash == nil || flash.Kind != "success" {
		t.Fatalf("expected success flash after redirect")
	}
}

func TestShowCloseRunDisplaysProgress(t *testing.T) {
	svc := &stubCloseService{
		getCloseRunFn: func(ctx context.Context, id int64) (close.CloseRun, error) {
			return close.CloseRun{
				ID:       50,
				PeriodID: 70,
				Status:   close.RunStatusInProgress,
				Checklist: []close.ChecklistItem{
					{ID: 1, Label: "Bank", Code: "BANK", Status: close.ChecklistStatusDone, Comment: "ok"},
					{ID: 2, Label: "AP", Code: "AP", Status: close.ChecklistStatusPending},
					{ID: 3, Label: "AR", Code: "AR", Status: close.ChecklistStatusSkipped},
				},
			}, nil
		},
		getPeriodFn: func(ctx context.Context, id int64) (close.Period, error) {
			return close.Period{
				ID:        id,
				CompanyID: 3,
				Name:      "2024-02",
				StartDate: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				EndDate:   time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
				Status:    close.PeriodStatusOpen,
			}, nil
		},
	}
	handler, sessions := newTestHandler(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/close-runs/50", nil)
	sess := loadSession(t, sessions, req)
	req = req.WithContext(shared.ContextWithSession(req.Context(), sess))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", "50")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))

	rr := httptest.NewRecorder()
	handler.showCloseRun(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "2 dari 3 checklist selesai") {
		t.Fatalf("expected checklist summary in page")
	}
	if !strings.Contains(body, "Checklist belum selesai (2 dari 3)") {
		t.Fatalf("expected hard close guard message when checklist pending")
	}
}

type stubCloseService struct {
	listPeriodsFn     func(context.Context, int64, int, int) ([]close.Period, error)
	createPeriodFn    func(context.Context, close.CreatePeriodInput) (close.Period, error)
	startCloseRunFn   func(context.Context, close.StartCloseRunInput) (close.CloseRun, error)
	getCloseRunFn     func(context.Context, int64) (close.CloseRun, error)
	getPeriodFn       func(context.Context, int64) (close.Period, error)
	updateChecklistFn func(context.Context, close.ChecklistUpdateInput) (close.ChecklistItem, error)
	softCloseFn       func(context.Context, int64, int64) (close.Period, error)
	hardCloseFn       func(context.Context, int64, int64) (close.Period, error)
}

func (s *stubCloseService) ListPeriods(ctx context.Context, companyID int64, limit, offset int) ([]close.Period, error) {
	if s.listPeriodsFn != nil {
		return s.listPeriodsFn(ctx, companyID, limit, offset)
	}
	return nil, nil
}

func (s *stubCloseService) CreatePeriod(ctx context.Context, in close.CreatePeriodInput) (close.Period, error) {
	if s.createPeriodFn != nil {
		return s.createPeriodFn(ctx, in)
	}
	return close.Period{}, nil
}

func (s *stubCloseService) StartCloseRun(ctx context.Context, in close.StartCloseRunInput) (close.CloseRun, error) {
	if s.startCloseRunFn != nil {
		return s.startCloseRunFn(ctx, in)
	}
	return close.CloseRun{}, nil
}

func (s *stubCloseService) GetCloseRun(ctx context.Context, id int64) (close.CloseRun, error) {
	if s.getCloseRunFn != nil {
		return s.getCloseRunFn(ctx, id)
	}
	return close.CloseRun{}, nil
}

func (s *stubCloseService) GetPeriod(ctx context.Context, id int64) (close.Period, error) {
	if s.getPeriodFn != nil {
		return s.getPeriodFn(ctx, id)
	}
	return close.Period{}, nil
}

func (s *stubCloseService) UpdateChecklist(ctx context.Context, in close.ChecklistUpdateInput) (close.ChecklistItem, error) {
	if s.updateChecklistFn != nil {
		return s.updateChecklistFn(ctx, in)
	}
	return close.ChecklistItem{}, nil
}

func (s *stubCloseService) SoftClose(ctx context.Context, runID, actorID int64) (close.Period, error) {
	if s.softCloseFn != nil {
		return s.softCloseFn(ctx, runID, actorID)
	}
	return close.Period{}, nil
}

func (s *stubCloseService) HardClose(ctx context.Context, runID, actorID int64) (close.Period, error) {
	if s.hardCloseFn != nil {
		return s.hardCloseFn(ctx, runID, actorID)
	}
	return close.Period{}, nil
}

func newTestHandler(t *testing.T, svc *stubCloseService) (*Handler, *shared.SessionManager) {
	t.Helper()
	mr := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	sessions := shared.NewSessionManager(redisClient, "test_session", "secret", time.Hour, false)
	csrf := shared.NewCSRFManager("csrfsecret")
	templates, err := view.NewEngine()
	if err != nil {
		t.Fatalf("templates: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewHandler(logger, svc, templates, csrf, rbac.Middleware{})
	return handler, sessions
}

func loadSession(t *testing.T, sessions *shared.SessionManager, req *http.Request) *shared.Session {
	t.Helper()
	sess, err := sessions.Load(context.Background(), req)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	return sess
}
