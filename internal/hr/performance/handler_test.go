package performance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// The handler tests run as user 3 in company 1. User 7 and user 9 are other
// people in the company.
const (
	hUserID    = int64(3)
	hCompanyID = int64(1)
	hOtherUser = int64(7)
	hThirdUser = int64(9)
)

type handlerPerms struct{ permissions []string }

func (p handlerPerms) EffectivePermissions(context.Context, int64) ([]string, error) {
	return p.permissions, nil
}

func ptrInt(v int) *int              { return &v }
func ptrID(v int64) *int64           { return &v }
func ptrTime(t time.Time) *time.Time { return &t }

var (
	hPeriodStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	hPeriodEnd   = time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	hSelfDue     = time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	hManagerDue  = time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	hSubmitted   = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
)

func baseParticipant(id int64, employeeUser, reviewerUser int64, name, stage string) Participant {
	return Participant{
		ID: id, CompanyID: hCompanyID, CycleID: 5, CycleName: "FY2026 Annual", CycleStatus: CycleActive,
		PeriodStart: hPeriodStart, PeriodEnd: hPeriodEnd, SelfReviewDue: hSelfDue, ManagerReviewDue: hManagerDue,
		EmployeeID: id + 100, EmployeeName: name, EmployeeUserID: ptrID(employeeUser),
		ReviewerEmployeeID: ptrID(200), ReviewerName: "Reviewer " + name, ReviewerUserID: ptrID(reviewerUser),
		Status: stage,
	}
}

// fixtureParticipants:
//   - 41: mine (user 3), reviewed by user 7, self submitted.
//   - 42: user 9's review, reviewed by me, manager submitted.
//   - 43: user 9's review, reviewed by user 7 (nothing to do with me).
//   - 44: mine, completed with a final rating.
//   - 45: user 9's review, reviewed by me, self submitted (manager form open).
func fixtureParticipants() map[int64]Participant {
	p41 := baseParticipant(41, hUserID, hOtherUser, "Ana Self", StageSelfSubmitted)
	p41.SelfRating, p41.SelfComments, p41.SelfSubmittedAt = ptrInt(4), "SELF-41-COMMENT", ptrTime(hSubmitted)

	p42 := baseParticipant(42, hThirdUser, hUserID, "Budi Team", StageManagerSubmitted)
	p42.SelfRating, p42.SelfComments, p42.SelfSubmittedAt = ptrInt(3), "SELF-42-COMMENT", ptrTime(hSubmitted)
	p42.ManagerRating, p42.ManagerComments, p42.ManagerSubmittedAt = ptrInt(2), "MGR-42-COMMENT", ptrTime(hSubmitted)

	p43 := baseParticipant(43, hThirdUser, hOtherUser, "Citra Other", StageManagerSubmitted)
	p43.SelfComments, p43.ManagerComments = "SELF-43-COMMENT", "MGR-43-COMMENT"

	p44 := baseParticipant(44, hUserID, hOtherUser, "Ana Done", StageCompleted)
	p44.SelfRating, p44.SelfComments, p44.SelfSubmittedAt = ptrInt(4), "SELF-44-COMMENT", ptrTime(hSubmitted)
	p44.ManagerRating, p44.ManagerComments, p44.ManagerSubmittedAt = ptrInt(3), "MGR-44-COMMENT", ptrTime(hSubmitted)
	p44.FinalRating, p44.CompletedAt = ptrInt(5), ptrTime(hSubmitted)

	p45 := baseParticipant(45, hThirdUser, hUserID, "Dewi Pending", StageSelfSubmitted)
	p45.SelfRating, p45.SelfComments, p45.SelfSubmittedAt = ptrInt(4), "SELF-45-COMMENT", ptrTime(hSubmitted)

	return map[int64]Participant{41: p41, 42: p42, 43: p43, 44: p44, 45: p45}
}

type managerCall struct {
	participantID int64
	isAdmin       bool
	in            ReviewInput
}

// handlerFakeService is an in-memory HandlerService that records writes.
type handlerFakeService struct {
	participants  map[int64]Participant
	createdCycles []CycleInput
	activated     []int64
	closed        []int64
	added         [][]int64
	goalsAdded    []GoalInput
	goalsUpdated  []int64
	goalsDeleted  []int64
	goalAdmin     []bool
	selfCalls     []ReviewInput
	managerCalls  []managerCall
	completed     [][2]int64
	cycleCalls    int
}

func (f *handlerFakeService) writes() int {
	return len(f.createdCycles) + len(f.activated) + len(f.closed) + len(f.added) + len(f.goalsAdded) + len(f.goalsUpdated) +
		len(f.goalsDeleted) + len(f.selfCalls) + len(f.managerCalls) + len(f.completed)
}

func (f *handlerFakeService) CreateCycle(_ context.Context, _ int64, in CycleInput) (Cycle, error) {
	f.createdCycles = append(f.createdCycles, in)
	return Cycle{ID: 6}, nil
}

func (f *handlerFakeService) ListCycles(context.Context, int64) ([]Cycle, error) {
	f.cycleCalls++
	return []Cycle{{ID: 5, Name: "FY2026 Annual", PeriodStart: hPeriodStart, PeriodEnd: hPeriodEnd, SelfReviewDue: hSelfDue,
		ManagerReviewDue: hManagerDue, Status: CycleActive, ParticipantCount: 5, CompletedCount: 1}}, nil
}

func (f *handlerFakeService) GetCycle(_ context.Context, companyID, id int64) (Cycle, error) {
	if companyID != hCompanyID || id != 5 {
		return Cycle{}, ErrCycleNotFound
	}
	return Cycle{ID: 5, CompanyID: hCompanyID, Name: "FY2026 Annual", PeriodStart: hPeriodStart, PeriodEnd: hPeriodEnd,
		SelfReviewDue: hSelfDue, ManagerReviewDue: hManagerDue, Status: CycleDraft, ParticipantCount: 2}, nil
}

func (f *handlerFakeService) ActivateCycle(_ context.Context, _, id, _ int64) error {
	f.activated = append(f.activated, id)
	return nil
}

func (f *handlerFakeService) CloseCycle(_ context.Context, _, id, _ int64) error {
	f.closed = append(f.closed, id)
	return nil
}

func (f *handlerFakeService) AddParticipants(_ context.Context, _, _ int64, ids []int64, _ int64) (int, error) {
	f.added = append(f.added, ids)
	return len(ids), nil
}

func (f *handlerFakeService) ListParticipants(context.Context, int64, int64) ([]Participant, error) {
	ps := fixtureParticipants()
	return []Participant{ps[42], ps[44]}, nil
}

func (f *handlerFakeService) GetParticipant(_ context.Context, companyID, id int64) (Participant, error) {
	p, ok := f.participants[id]
	if !ok || companyID != hCompanyID {
		return Participant{}, ErrParticipantNotFound
	}
	return p, nil
}

func (f *handlerFakeService) ListGoals(_ context.Context, _, participantID int64) ([]Goal, error) {
	return []Goal{{ID: 501, ParticipantID: participantID, Seq: 1, Title: "Ship payroll v2", KPITarget: "Go live by Q4", Weight: "60.00",
		SelfScore: ptrInt(4), ManagerScore: ptrInt(2)}}, nil
}

func (f *handlerFakeService) AddGoal(_ context.Context, _, _, _ int64, isAdmin bool, in GoalInput) (Goal, error) {
	f.goalsAdded = append(f.goalsAdded, in)
	f.goalAdmin = append(f.goalAdmin, isAdmin)
	return Goal{ID: 502}, nil
}

func (f *handlerFakeService) UpdateGoal(_ context.Context, _, goalID, _ int64, isAdmin bool, _ GoalInput) error {
	f.goalsUpdated = append(f.goalsUpdated, goalID)
	f.goalAdmin = append(f.goalAdmin, isAdmin)
	return nil
}

func (f *handlerFakeService) DeleteGoal(_ context.Context, _, goalID, _ int64, isAdmin bool) error {
	f.goalsDeleted = append(f.goalsDeleted, goalID)
	f.goalAdmin = append(f.goalAdmin, isAdmin)
	return nil
}

func (f *handlerFakeService) SubmitSelfReview(_ context.Context, _, _, _ int64, in ReviewInput) error {
	f.selfCalls = append(f.selfCalls, in)
	return nil
}

func (f *handlerFakeService) SubmitManagerReview(_ context.Context, _, pid, _ int64, isAdmin bool, in ReviewInput) error {
	f.managerCalls = append(f.managerCalls, managerCall{participantID: pid, isAdmin: isAdmin, in: in})
	return nil
}

func (f *handlerFakeService) Complete(_ context.Context, _, pid, _ int64, rating int) (int64, error) {
	f.completed = append(f.completed, [2]int64{pid, int64(rating)})
	return 900, nil
}

func (f *handlerFakeService) MyReviews(context.Context, int64, int64) ([]Participant, error) {
	ps := fixtureParticipants()
	return []Participant{ps[41], ps[44]}, nil
}

func (f *handlerFakeService) TeamReviews(context.Context, int64, int64) ([]Participant, error) {
	ps := fixtureParticipants()
	return []Participant{ps[42], ps[45]}, nil
}

type handlerFakePeople struct{}

func (handlerFakePeople) List(context.Context, int64, ...employees.EmployeeFilter) ([]employees.Employee, error) {
	return []employees.Employee{
		{ID: 142, EmployeeNumber: "E-142", Name: "Budi Team"},
		{ID: 160, EmployeeNumber: "E-160", Name: "Eko Newcomer", Profile: employees.Profile{NationalID: "NIK-SECRET"}},
	}, nil
}

func perfRequest(t *testing.T, method, path string, form url.Values) (*http.Request, *shared.Session) {
	t.Helper()
	manager := shared.NewSessionManager(nil, "session", "secret", time.Hour, false)
	body := strings.NewReader("")
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	session, err := manager.Load(req.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	session.SetUser("3")
	session.Set("company_id", "1")
	return req.WithContext(shared.ContextWithSession(req.Context(), session)), session
}

func newPerfRouter(t *testing.T, perms []string, svc *handlerFakeService, withTemplates bool) http.Handler {
	t.Helper()
	var engine *view.Engine
	if withTemplates {
		var err error
		engine, err = view.NewEngine()
		if err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler(nil, svc, handlerFakePeople{}, engine, shared.NewCSRFManager("secret"), rbac.Middleware{Service: handlerPerms{perms}})
	h.now = func() time.Time { return time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC) }
	router := chi.NewRouter()
	router.Route("/hr/performance", h.MountRoutes)
	return router
}

func newFakeService() *handlerFakeService {
	return &handlerFakeService{participants: fixtureParticipants()}
}

var (
	viewPerms  = []string{shared.PermHRPerformanceView}
	adminPerms = []string{shared.PermHRPerformanceAdmin}
)

func TestPerformanceRoutePermissions(t *testing.T) {
	cases := []struct {
		name         string
		perms        []string
		method, path string
		form         url.Values
	}{
		{"page requires performance view or admin", []string{shared.PermHREmployeeView}, http.MethodGet, "/hr/performance", nil},
		{"review requires performance view or admin", []string{shared.PermHREmployeeView}, http.MethodGet, "/hr/performance/reviews/41", nil},
		{"self submit requires performance view or admin", []string{shared.PermHREmployeeView}, http.MethodPost, "/hr/performance/reviews/41/self", url.Values{"rating": {"4"}}},
		{"create cycle requires admin", viewPerms, http.MethodPost, "/hr/performance/cycles", url.Values{"name": {"X"}}},
		{"cycle page requires admin", viewPerms, http.MethodGet, "/hr/performance/cycles/5", nil},
		{"activate requires admin", viewPerms, http.MethodPost, "/hr/performance/cycles/5/activate", url.Values{}},
		{"close requires admin", viewPerms, http.MethodPost, "/hr/performance/cycles/5/close", url.Values{}},
		{"participants require admin", viewPerms, http.MethodPost, "/hr/performance/cycles/5/participants", url.Values{"employee_ids": {"142"}}},
		{"complete requires admin even for the reviewer", viewPerms, http.MethodPost, "/hr/performance/reviews/42/complete", url.Values{"final_rating": {"3"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newFakeService()
			req, _ := perfRequest(t, tc.method, tc.path, tc.form)
			rec := httptest.NewRecorder()
			newPerfRouter(t, tc.perms, svc, false).ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status=%d want 403", rec.Code)
			}
			if svc.writes() != 0 {
				t.Fatal("forbidden request reached the performance service")
			}
		})
	}
}

func TestPerformanceReviewerAndParticipantChecks(t *testing.T) {
	cases := []struct {
		name   string
		perms  []string
		path   string
		form   url.Values
		status int
	}{
		{"non-reviewer manager cannot submit a manager review", viewPerms, "/hr/performance/reviews/43/manager", url.Values{"rating": {"2"}}, http.StatusForbidden},
		{"participant cannot submit their own manager review", viewPerms, "/hr/performance/reviews/41/manager", url.Values{"rating": {"5"}}, http.StatusForbidden},
		{"reviewer cannot submit the self review", viewPerms, "/hr/performance/reviews/45/self", url.Values{"rating": {"5"}}, http.StatusForbidden},
		{"admin cannot submit someone else's self review", adminPerms, "/hr/performance/reviews/45/self", url.Values{"rating": {"5"}}, http.StatusForbidden},
		{"reviewer cannot add goals", viewPerms, "/hr/performance/reviews/45/goals", url.Values{"title": {"x"}}, http.StatusForbidden},
		{"stranger cannot delete goals", viewPerms, "/hr/performance/reviews/43/goals/501/delete", url.Values{}, http.StatusForbidden},
		{"unknown participant is 404", viewPerms, "/hr/performance/reviews/999/manager", url.Values{"rating": {"2"}}, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newFakeService()
			req, _ := perfRequest(t, http.MethodPost, tc.path, tc.form)
			rec := httptest.NewRecorder()
			newPerfRouter(t, tc.perms, svc, false).ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status=%d want %d", rec.Code, tc.status)
			}
			if svc.writes() != 0 {
				t.Fatal("rejected request reached the performance service")
			}
		})
	}
}

func TestPerformanceReviewPageVisibility(t *testing.T) {
	cases := []struct {
		name   string
		perms  []string
		path   string
		mutate func(map[int64]Participant)
		status int
	}{
		{"participant cannot load another participant", viewPerms, "/hr/performance/reviews/43", nil, http.StatusNotFound},
		{"participant loads own review", viewPerms, "/hr/performance/reviews/41", nil, http.StatusOK},
		{"reviewer loads team review", viewPerms, "/hr/performance/reviews/42", nil, http.StatusOK},
		{"admin loads any review", adminPerms, "/hr/performance/reviews/43", nil, http.StatusOK},
		{"cross-tenant or missing id is 404", adminPerms, "/hr/performance/reviews/999", nil, http.StatusNotFound},
		{"draft cycle hidden from participant", viewPerms, "/hr/performance/reviews/41", func(m map[int64]Participant) {
			p := m[41]
			p.CycleStatus = CycleDraft
			m[41] = p
		}, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newFakeService()
			if tc.mutate != nil {
				tc.mutate(svc.participants)
			}
			req, _ := perfRequest(t, http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			newPerfRouter(t, tc.perms, svc, true).ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status=%d want %d", rec.Code, tc.status)
			}
			if tc.status == http.StatusNotFound && strings.Contains(rec.Body.String(), "COMMENT") {
				t.Fatal("404 response leaks review content")
			}
		})
	}
}

func TestPerformanceWriteHappyPaths(t *testing.T) {
	cases := []struct {
		name         string
		perms        []string
		path         string
		form         url.Values
		wantLocation string
		check        func(t *testing.T, svc *handlerFakeService)
	}{
		{"participant submits self review", viewPerms, "/hr/performance/reviews/41/self",
			url.Values{"rating": {"4"}, "comments": {"Proud of Q3"}, "goal_score_501": {"5"}, "goal_score_502": {""}},
			"/hr/performance/reviews/41", func(t *testing.T, svc *handlerFakeService) {
				if len(svc.selfCalls) != 1 {
					t.Fatalf("self=%v", svc.selfCalls)
				}
				in := svc.selfCalls[0]
				if in.Rating != 4 || in.Comments != "Proud of Q3" || len(in.GoalScores) != 1 || in.GoalScores[501] != 5 {
					t.Fatalf("self input=%+v", in)
				}
			}},
		{"reviewer submits manager review", viewPerms, "/hr/performance/reviews/45/manager",
			url.Values{"rating": {"3"}, "comments": {"Solid"}}, "/hr/performance/reviews/45", func(t *testing.T, svc *handlerFakeService) {
				if len(svc.managerCalls) != 1 || svc.managerCalls[0].participantID != 45 || svc.managerCalls[0].isAdmin || svc.managerCalls[0].in.Rating != 3 {
					t.Fatalf("manager=%+v", svc.managerCalls)
				}
			}},
		{"admin submits manager review for a participant without a reviewer", adminPerms, "/hr/performance/reviews/43/manager",
			url.Values{"rating": {"2"}}, "/hr/performance/reviews/43", func(t *testing.T, svc *handlerFakeService) {
				if len(svc.managerCalls) != 1 || !svc.managerCalls[0].isAdmin {
					t.Fatalf("manager=%+v", svc.managerCalls)
				}
			}},
		{"participant adds goal", viewPerms, "/hr/performance/reviews/41/goals",
			url.Values{"title": {"Mentor two juniors"}, "kpi_target": {"2 people"}, "weight": {"40"}}, "/hr/performance/reviews/41",
			func(t *testing.T, svc *handlerFakeService) {
				want := GoalInput{Title: "Mentor two juniors", KPITarget: "2 people", Weight: "40"}
				if len(svc.goalsAdded) != 1 || svc.goalsAdded[0] != want || svc.goalAdmin[0] {
					t.Fatalf("goals=%+v admin=%v", svc.goalsAdded, svc.goalAdmin)
				}
			}},
		{"participant updates goal", viewPerms, "/hr/performance/reviews/41/goals/501/update", url.Values{"title": {"Ship payroll v3"}},
			"/hr/performance/reviews/41", func(t *testing.T, svc *handlerFakeService) {
				if len(svc.goalsUpdated) != 1 || svc.goalsUpdated[0] != 501 {
					t.Fatalf("updated=%v", svc.goalsUpdated)
				}
			}},
		{"admin deletes goal", adminPerms, "/hr/performance/reviews/43/goals/501/delete", url.Values{},
			"/hr/performance/reviews/43", func(t *testing.T, svc *handlerFakeService) {
				if len(svc.goalsDeleted) != 1 || !svc.goalAdmin[0] {
					t.Fatalf("deleted=%v admin=%v", svc.goalsDeleted, svc.goalAdmin)
				}
			}},
		{"admin completes review", adminPerms, "/hr/performance/reviews/42/complete", url.Values{"final_rating": {"3"}},
			"/hr/performance/reviews/42", func(t *testing.T, svc *handlerFakeService) {
				if len(svc.completed) != 1 || svc.completed[0] != [2]int64{42, 3} {
					t.Fatalf("completed=%v", svc.completed)
				}
			}},
		{"admin creates cycle", adminPerms, "/hr/performance/cycles",
			url.Values{"name": {"FY2027"}, "period_start": {"2027-01-01"}, "period_end": {"2027-12-31"}, "self_review_due": {"2027-11-15"}, "manager_review_due": {"2027-11-30"}},
			"/hr/performance/cycles/6", func(t *testing.T, svc *handlerFakeService) {
				if len(svc.createdCycles) != 1 {
					t.Fatalf("created=%v", svc.createdCycles)
				}
				in := svc.createdCycles[0]
				if in.Name != "FY2027" || in.ActorID != hUserID || !in.PeriodEnd.Equal(time.Date(2027, 12, 31, 0, 0, 0, 0, time.UTC)) ||
					!in.ManagerReviewDue.Equal(time.Date(2027, 11, 30, 0, 0, 0, 0, time.UTC)) {
					t.Fatalf("cycle input=%+v", in)
				}
			}},
		{"admin adds participants", adminPerms, "/hr/performance/cycles/5/participants", url.Values{"employee_ids": {"142", "160"}},
			"/hr/performance/cycles/5", func(t *testing.T, svc *handlerFakeService) {
				if len(svc.added) != 1 || len(svc.added[0]) != 2 || svc.added[0][0] != 142 || svc.added[0][1] != 160 {
					t.Fatalf("added=%v", svc.added)
				}
			}},
		{"admin activates cycle", adminPerms, "/hr/performance/cycles/5/activate", url.Values{}, "/hr/performance/cycles/5",
			func(t *testing.T, svc *handlerFakeService) {
				if len(svc.activated) != 1 || svc.activated[0] != 5 {
					t.Fatalf("activated=%v", svc.activated)
				}
			}},
		{"admin closes cycle", adminPerms, "/hr/performance/cycles/5/close", url.Values{}, "/hr/performance/cycles/5",
			func(t *testing.T, svc *handlerFakeService) {
				if len(svc.closed) != 1 || svc.closed[0] != 5 {
					t.Fatalf("closed=%v", svc.closed)
				}
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newFakeService()
			req, session := perfRequest(t, http.MethodPost, tc.path, tc.form)
			rec := httptest.NewRecorder()
			newPerfRouter(t, tc.perms, svc, false).ServeHTTP(rec, req)
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != tc.wantLocation {
				t.Fatalf("status=%d location=%q want %q", rec.Code, rec.Header().Get("Location"), tc.wantLocation)
			}
			if f := session.PopFlash(); f == nil || f.Kind != "success" {
				t.Fatalf("flash=%+v", f)
			}
			tc.check(t, svc)
		})
	}
}

func TestPerformanceInvalidInputs(t *testing.T) {
	cases := []struct {
		name, path   string
		form         url.Values
		wantLocation string
		wantMessage  string
	}{
		{"bad rating", "/hr/performance/reviews/41/self", url.Values{"rating": {"x"}}, "/hr/performance/reviews/41", "Rating must be between 1 and 5"},
		{"bad goal score key", "/hr/performance/reviews/41/self", url.Values{"rating": {"4"}, "goal_score_abc": {"3"}}, "/hr/performance/reviews/41", "Not found: performance goal"},
		{"bad cycle date", "/hr/performance/cycles", url.Values{"name": {"FY"}, "period_start": {"01/01/2027"}}, "/hr/performance?tab=cycles", "Invalid performance cycle"},
		{"bad employee id", "/hr/performance/cycles/5/participants", url.Values{"employee_ids": {"abc"}}, "/hr/performance/cycles/5", "Not found: employee"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newFakeService()
			req, session := perfRequest(t, http.MethodPost, tc.path, tc.form)
			rec := httptest.NewRecorder()
			// The participant (user 3) is also an admin here so every route is reachable.
			newPerfRouter(t, []string{shared.PermHRPerformanceView, shared.PermHRPerformanceAdmin}, svc, false).ServeHTTP(rec, req)
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != tc.wantLocation {
				t.Fatalf("status=%d location=%q", rec.Code, rec.Header().Get("Location"))
			}
			if f := session.PopFlash(); f == nil || f.Kind != "error" || f.Message != tc.wantMessage {
				t.Fatalf("flash=%+v", f)
			}
			if svc.writes() != 0 {
				t.Fatal("invalid input reached the performance service")
			}
		})
	}
}

func requireCSRFOnPostForms(t *testing.T, body string) {
	t.Helper()
	parts := strings.Split(body, `<form method="post"`)
	for _, part := range parts[1:] {
		end := strings.Index(part, "</form>")
		if end < 0 || !strings.Contains(part[:end], `name="csrf_token"`) {
			t.Fatalf("POST form without csrf_token: %.120s", part)
		}
	}
}

func renderPerf(t *testing.T, perms []string, path string) string {
	t.Helper()
	svc := newFakeService()
	req, _ := perfRequest(t, http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	newPerfRouter(t, perms, svc, true).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	requireCSRFOnPostForms(t, body)
	return body
}

func requireContains(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Fatalf("page missing %q", w)
		}
	}
}

func requireAbsent(t *testing.T, body string, absents ...string) {
	t.Helper()
	for _, a := range absents {
		if strings.Contains(body, a) {
			t.Fatalf("page renders %q", a)
		}
	}
}

func TestPerformancePageRenders(t *testing.T) {
	t.Run("viewer sees my and team tabs only", func(t *testing.T) {
		svc := newFakeService()
		req, _ := perfRequest(t, http.MethodGet, "/hr/performance?tab=cycles", nil)
		rec := httptest.NewRecorder()
		newPerfRouter(t, viewPerms, svc, true).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d", rec.Code)
		}
		body := rec.Body.String()
		requireContains(t, body, `data-tab="my" class="btn tab active"`, `data-tab="team"`, "Ana Self", "Budi Team", "Dewi Pending",
			`href="/hr/performance/reviews/41"`, `href="/hr/performance/reviews/45"`, "5 / 5")
		requireAbsent(t, body, `data-tab="cycles"`, `id="create-cycle-dialog"`, "COMMENT")
		if svc.cycleCalls != 0 {
			t.Fatal("cycles loaded for a viewer")
		}
		requireCSRFOnPostForms(t, body)
	})
	t.Run("my reviews hide final rating until completed", func(t *testing.T) {
		body := renderPerf(t, viewPerms, "/hr/performance")
		// 44 is completed with final rating 5; team row 42 has none.
		if strings.Count(body, " / 5") != 1 {
			t.Fatalf("expected exactly one final rating, got %d", strings.Count(body, " / 5"))
		}
	})
	t.Run("admin sees cycles tab and dialog", func(t *testing.T) {
		body := renderPerf(t, adminPerms, "/hr/performance?tab=cycles")
		requireContains(t, body, `data-tab="cycles" class="btn tab active"`, "FY2026 Annual", `href="/hr/performance/cycles/5"`,
			`id="create-cycle-dialog"`, `action="/hr/performance/cycles"`)
	})
}

func TestPerformanceCyclePageRenders(t *testing.T) {
	body := renderPerf(t, adminPerms, "/hr/performance/cycles/5")
	requireContains(t, body, "FY2026 Annual", "Budi Team", "Ana Done", `href="/hr/performance/reviews/42"`,
		`action="/hr/performance/cycles/5/activate"`, `action="/hr/performance/cycles/5/participants"`,
		`<option value="160">Eko Newcomer (E-160)</option>`, "5 / 5")
	// Already-enrolled employees are not offered again; no sensitive fields
	// and no review comments are rendered on the cycle page.
	requireAbsent(t, body, `<option value="142">`, "NIK-SECRET", "COMMENT", `/close"`)
}

func TestPerformanceReviewPageRenders(t *testing.T) {
	t.Run("participant sees own self data only", func(t *testing.T) {
		body := renderPerf(t, viewPerms, "/hr/performance/reviews/41")
		requireContains(t, body, "Ana Self", "Ship payroll v2", "data-self-review", "SELF-41-COMMENT", "Self Score",
			`action="/hr/performance/reviews/41/goals"`, `action="/hr/performance/reviews/41/goals/501/update"`, `action="/hr/performance/reviews/41/goals/501/delete"`)
		requireAbsent(t, body, "data-manager-review", "Manager Score", "data-final-rating", "data-complete-review",
			`/self"`, `/manager"`, `/complete"`)
	})
	t.Run("participant sees final rating once completed but never manager comments", func(t *testing.T) {
		body := renderPerf(t, viewPerms, "/hr/performance/reviews/44")
		requireContains(t, body, "data-final-rating", "5 / 5", "SELF-44-COMMENT")
		requireAbsent(t, body, "MGR-44-COMMENT", "data-manager-review", "Manager Score", `/goals"`)
	})
	t.Run("reviewer sees self and manager data", func(t *testing.T) {
		body := renderPerf(t, viewPerms, "/hr/performance/reviews/42")
		requireContains(t, body, "SELF-42-COMMENT", "MGR-42-COMMENT", "Manager Score", "Self Score")
		requireAbsent(t, body, `/manager"`, `/complete"`, `/goals"`, `/self"`)
	})
	t.Run("reviewer gets the manager form", func(t *testing.T) {
		body := renderPerf(t, viewPerms, "/hr/performance/reviews/45")
		requireContains(t, body, `action="/hr/performance/reviews/45/manager"`, `name="goal_score_501"`, `name="rating"`, "SELF-45-COMMENT")
		requireAbsent(t, body, `/self"`, `/complete"`)
	})
	t.Run("participant gets the self form while enrolled", func(t *testing.T) {
		svc := newFakeService()
		p := svc.participants[41]
		p.Status, p.SelfRating, p.SelfComments, p.SelfSubmittedAt = StageEnrolled, nil, "", nil
		svc.participants[41] = p
		req, _ := perfRequest(t, http.MethodGet, "/hr/performance/reviews/41", nil)
		rec := httptest.NewRecorder()
		newPerfRouter(t, viewPerms, svc, true).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d", rec.Code)
		}
		body := rec.Body.String()
		requireContains(t, body, `action="/hr/performance/reviews/41/self"`, `name="goal_score_501"`)
		requireAbsent(t, body, `/manager"`)
		requireCSRFOnPostForms(t, body)
	})
	t.Run("admin gets the complete form", func(t *testing.T) {
		body := renderPerf(t, adminPerms, "/hr/performance/reviews/42")
		requireContains(t, body, "data-complete-review", `action="/hr/performance/reviews/42/complete"`, `<option value="2" selected>2</option>`,
			"MGR-42-COMMENT", `href="/hr/performance/cycles/5"`)
	})
}
