package schedule

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type permissionReader struct{ permissions []string }

func (p permissionReader) EffectivePermissions(context.Context, int64) ([]string, error) {
	return p.permissions, nil
}

type assignCall struct {
	companyID, employeeID, shiftID, actorID int64
	validFrom                               time.Time
}

// fakeSchedule is an in-memory HandlerService that records writes.
type fakeSchedule struct {
	holidays       []Holiday
	createdHoliday []string
	deleted        []int64
	createdShifts  []ShiftInput
	assigned       []assignCall
}

func (f *fakeSchedule) writes() int {
	return len(f.createdHoliday) + len(f.deleted) + len(f.createdShifts) + len(f.assigned)
}

func (f *fakeSchedule) ListHolidays(context.Context, int64, int) ([]Holiday, error) {
	return f.holidays, nil
}

func (f *fakeSchedule) CreateHoliday(_ context.Context, _ int64, date time.Time, name string, _ int64) (Holiday, error) {
	f.createdHoliday = append(f.createdHoliday, date.Format("2006-01-02")+" "+name)
	return Holiday{ID: 1}, nil
}

func (f *fakeSchedule) DeleteHoliday(_ context.Context, _, id, _ int64) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *fakeSchedule) ListShifts(context.Context, int64) ([]Shift, error) {
	return []Shift{
		{ID: 4, Code: "NIGHT", Name: "Night shift", StartTime: "22:00", EndTime: "06:00", BreakMinutes: 30,
			WorkDays: []time.Weekday{time.Monday, time.Tuesday, time.Saturday}, IsActive: true},
		{ID: 5, Code: "OLD", Name: "Retired shift", StartTime: "07:00", EndTime: "15:00",
			WorkDays: []time.Weekday{time.Monday}, IsActive: false},
	}, nil
}

func (f *fakeSchedule) CreateShift(_ context.Context, _ int64, in ShiftInput, _ int64) (Shift, error) {
	f.createdShifts = append(f.createdShifts, in)
	return Shift{ID: 6}, nil
}

func (f *fakeSchedule) AssignShift(_ context.Context, companyID, employeeID, shiftID int64, validFrom time.Time, actorID int64) (ShiftAssignment, error) {
	f.assigned = append(f.assigned, assignCall{companyID, employeeID, shiftID, actorID, validFrom})
	return ShiftAssignment{ID: 7}, nil
}

func (f *fakeSchedule) ListEmployeeShifts(context.Context, int64, time.Time) ([]EmployeeShift, error) {
	return []EmployeeShift{
		{EmployeeID: 42, EmployeeNumber: "E-042", Name: "Grace Hopper", Department: "Ops",
			Shift: &ShiftAssignment{ID: 9, EmployeeID: 42, ShiftID: 4, ValidFrom: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
				Shift: Shift{ID: 4, Code: "NIGHT", Name: "Night shift", StartTime: "22:00", EndTime: "06:00"}}},
		{EmployeeID: 43, EmployeeNumber: "E-043", Name: "Alan Turing"},
	}, nil
}

func scheduleRequest(t *testing.T, method, path string, form url.Values) *http.Request {
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
	return req.WithContext(shared.ContextWithSession(req.Context(), session))
}

func newScheduleRouter(t *testing.T, perms []string, svc *fakeSchedule, withTemplates bool) http.Handler {
	t.Helper()
	var engine *view.Engine
	if withTemplates {
		var err error
		engine, err = view.NewEngine()
		if err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler(nil, svc, engine, shared.NewCSRFManager("secret"), rbac.Middleware{Service: permissionReader{perms}})
	router := chi.NewRouter()
	router.Route("/hr/holidays", h.MountHolidayRoutes)
	router.Route("/hr/shifts", h.MountShiftRoutes)
	return router
}

func TestScheduleRoutesRequireScheduleAdmin(t *testing.T) {
	nonAdmin := []string{shared.PermHREmployeeAdmin, shared.PermHROvertimeAdmin, shared.PermHRLeaveAdmin}
	cases := []struct {
		method, path string
		form         url.Values
	}{
		{http.MethodGet, "/hr/holidays", nil},
		{http.MethodPost, "/hr/holidays", url.Values{"holiday_date": {"2026-08-17"}, "name": {"Independence Day"}}},
		{http.MethodPost, "/hr/holidays/1/delete", url.Values{}},
		{http.MethodGet, "/hr/shifts", nil},
		{http.MethodPost, "/hr/shifts", url.Values{"code": {"DAY"}, "name": {"Day"}}},
		{http.MethodPost, "/hr/shifts/assign", url.Values{"employee_id": {"42"}, "shift_id": {"4"}, "valid_from": {"2026-11-01"}}},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			svc := &fakeSchedule{}
			rec := httptest.NewRecorder()
			newScheduleRouter(t, nonAdmin, svc, false).ServeHTTP(rec, scheduleRequest(t, tc.method, tc.path, tc.form))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status=%d want 403", rec.Code)
			}
			if svc.writes() != 0 {
				t.Fatal("forbidden request reached the schedule service")
			}
		})
	}
}

func TestCreateHolidayHappyPath(t *testing.T) {
	svc := &fakeSchedule{}
	rec := httptest.NewRecorder()
	form := url.Values{"holiday_date": {"2026-08-17"}, "name": {"Independence Day"}}
	newScheduleRouter(t, []string{shared.PermHRScheduleAdmin}, svc, false).ServeHTTP(rec, scheduleRequest(t, http.MethodPost, "/hr/holidays", form))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/hr/holidays?year=2026" {
		t.Fatalf("status=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
	if len(svc.createdHoliday) != 1 || svc.createdHoliday[0] != "2026-08-17 Independence Day" {
		t.Fatalf("created=%v", svc.createdHoliday)
	}
}

func TestCreateShiftParsesWorkDays(t *testing.T) {
	svc := &fakeSchedule{}
	rec := httptest.NewRecorder()
	form := url.Values{"code": {"NIGHT"}, "name": {"Night"}, "start_time": {"22:00"}, "end_time": {"06:00"},
		"break_minutes": {"30"}, "work_days": {"1", "6", "7", "9"}, "is_active": {"1"}}
	newScheduleRouter(t, []string{shared.PermHRScheduleAdmin}, svc, false).ServeHTTP(rec, scheduleRequest(t, http.MethodPost, "/hr/shifts", form))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", rec.Code)
	}
	if len(svc.createdShifts) != 1 {
		t.Fatalf("created=%d", len(svc.createdShifts))
	}
	got := svc.createdShifts[0]
	want := []time.Weekday{time.Monday, time.Saturday, time.Sunday}
	if len(got.WorkDays) != len(want) {
		t.Fatalf("work days=%v want %v", got.WorkDays, want)
	}
	for i := range want {
		if got.WorkDays[i] != want[i] {
			t.Fatalf("work days=%v want %v", got.WorkDays, want)
		}
	}
	if got.BreakMinutes != 30 || !got.IsActive || got.StartTime != "22:00" {
		t.Fatalf("input=%+v", got)
	}
}

func TestAssignShiftHappyPath(t *testing.T) {
	svc := &fakeSchedule{}
	rec := httptest.NewRecorder()
	form := url.Values{"employee_id": {"42"}, "shift_id": {"4"}, "valid_from": {"2026-11-01"}}
	newScheduleRouter(t, []string{shared.PermHRScheduleAdmin}, svc, false).ServeHTTP(rec, scheduleRequest(t, http.MethodPost, "/hr/shifts/assign", form))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/hr/shifts?tab=assignments" {
		t.Fatalf("status=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
	want := assignCall{companyID: 1, employeeID: 42, shiftID: 4, actorID: 3, validFrom: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}
	if len(svc.assigned) != 1 || svc.assigned[0] != want {
		t.Fatalf("assigned=%+v want %+v", svc.assigned, want)
	}
}

func TestHolidaysPageRenders(t *testing.T) {
	svc := &fakeSchedule{holidays: []Holiday{{ID: 11, Date: time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC), Name: "Independence Day"}}}
	rec := httptest.NewRecorder()
	newScheduleRouter(t, []string{shared.PermHRScheduleAdmin}, svc, true).ServeHTTP(rec, scheduleRequest(t, http.MethodGet, "/hr/holidays?year=2026", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Independence Day", "17 Aug 2026", `action="/hr/holidays/11/delete"`, `id="add-holiday-dialog"`, `name="csrf_token"`, "Holidays in 2026"} {
		if !strings.Contains(body, want) {
			t.Fatalf("holidays page missing %q", want)
		}
	}
	assertEveryFormHasCSRF(t, body)
}

func TestShiftsPageRenders(t *testing.T) {
	svc := &fakeSchedule{}
	rec := httptest.NewRecorder()
	newScheduleRouter(t, []string{shared.PermHRScheduleAdmin}, svc, true).ServeHTTP(rec, scheduleRequest(t, http.MethodGet, "/hr/shifts", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Night shift", "Mon, Tue, Sat", "(overnight)", "Grace Hopper", "Default (Mon–Fri)",
		`id="assign-shift-dialog"`, `action="/hr/shifts/assign"`, `id="create-shift-dialog"`, `<option value="4">`} {
		if !strings.Contains(body, want) {
			t.Fatalf("shifts page missing %q", want)
		}
	}
	if strings.Contains(body, `<option value="5">`) {
		t.Fatal("inactive shift offered in the assign dialog")
	}
	assertEveryFormHasCSRF(t, body)
}

// assertEveryFormHasCSRF checks every POST form carries a csrf_token field.
func assertEveryFormHasCSRF(t *testing.T, body string) {
	t.Helper()
	parts := strings.Split(body, `<form method="post"`)
	for _, part := range parts[1:] {
		end := strings.Index(part, "</form>")
		if end < 0 || !strings.Contains(part[:end], `name="csrf_token"`) {
			t.Fatalf("POST form without csrf_token: %.120s", part)
		}
	}
}
