package employees

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// fakeChecklistSource records whether the checklist data was loaded.
type fakeChecklistSource struct {
	calls      int
	openOffbrd bool
}

func (f *fakeChecklistSource) AssetsHeldBy(context.Context, int64, int64) ([]ChecklistAsset, error) {
	f.calls++
	return []ChecklistAsset{{ID: 61, Number: "FA-0061", Name: "ThinkPad X1", Status: "ACTIVE"}}, nil
}

func (f *fakeChecklistSource) AssignableAssets(context.Context, int64) ([]ChecklistAsset, error) {
	f.calls++
	return []ChecklistAsset{{ID: 62, Number: "FA-0062", Name: "Monitor", Status: "ACTIVE"}}, nil
}

func (f *fakeChecklistSource) ActiveTemplates(context.Context, int64) ([]ChecklistTemplateOption, error) {
	f.calls++
	return []ChecklistTemplateOption{
		{ID: 20, Code: "ON-STD", Name: "Standard onboarding", Kind: "ONBOARDING"},
		{ID: 21, Code: "OFF-STD", Name: "Standard offboarding", Kind: "OFFBOARDING"},
	}, nil
}

func (f *fakeChecklistSource) EmployeeChecklists(context.Context, int64, int64) ([]EmployeeChecklist, error) {
	f.calls++
	if !f.openOffbrd {
		return nil, nil
	}
	return []EmployeeChecklist{{ID: 31, Kind: "OFFBOARDING", Status: "OPEN", TemplateName: "Standard offboarding",
		StartedOn: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), OpenTasks: 2}}, nil
}

func newChecklistDetailRouter(t *testing.T, perms []string, src ChecklistSource) http.Handler {
	t.Helper()
	engine, err := view.NewEngine()
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(nil, newFakeEmployees(), &fakeCompensation{}, engine, shared.NewCSRFManager("secret"), rbac.Middleware{Service: permissionReader{perms}})
	if src != nil {
		h.SetChecklists(src)
	}
	router := chi.NewRouter()
	router.Route("/hr/employees", h.MountRoutes)
	return router
}

func TestEmployeeDetailAssetsTabRequiresChecklistAdmin(t *testing.T) {
	markers := []string{
		`data-tab="assets"`, "FA-0061", "FA-0062",
		`action="/hr/checklists/assets/assign"`, `action="/hr/checklists/assets/61/return"`,
		`id="start-onboarding-dialog"`, `id="start-offboarding-dialog"`, `action="/hr/checklists/start"`,
		`name="termination_date"`, `name="termination_reason"`, "for every company in this Odyssey instance",
	}
	cases := []struct {
		name      string
		perms     []string
		src       *fakeChecklistSource
		want      bool
		wantCalls bool
	}{
		{"employee admin without checklist admin", []string{shared.PermHREmployeeAdmin}, &fakeChecklistSource{}, false, false},
		{"checklist admin", []string{shared.PermHREmployeeView, shared.PermHRChecklistAdmin}, &fakeChecklistSource{}, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newChecklistDetailRouter(t, tc.perms, tc.src).ServeHTTP(rec, employeeRequest(t, http.MethodGet, "/hr/employees/8?tab=assets", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			for _, m := range markers {
				if got := strings.Contains(body, m); got != tc.want {
					t.Fatalf("%q rendered=%v want %v", m, got, tc.want)
				}
			}
			if (tc.src.calls > 0) != tc.wantCalls {
				t.Fatalf("checklist source calls=%d", tc.src.calls)
			}
			if tc.want && !strings.Contains(body, `data-tab="assets" class="btn tab active"`) {
				t.Fatal("assets tab not active")
			}
			if !tc.want && !strings.Contains(body, `data-tab="profile" class="btn tab active"`) {
				t.Fatal("assets tab request did not fall back to profile")
			}
			parts := strings.Split(body, `<form method="post"`)
			for _, part := range parts[1:] {
				end := strings.Index(part, "</form>")
				if end < 0 || !strings.Contains(part[:end], `name="csrf_token"`) {
					t.Fatalf("POST form without csrf_token: %.120s", part)
				}
			}
		})
	}
}

func TestEmployeeDetailHidesStartOffboardingWhenOpen(t *testing.T) {
	src := &fakeChecklistSource{openOffbrd: true}
	rec := httptest.NewRecorder()
	newChecklistDetailRouter(t, []string{shared.PermHRChecklistAdmin, shared.PermHREmployeeView}, src).ServeHTTP(rec, employeeRequest(t, http.MethodGet, "/hr/employees/8?tab=assets", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, `id="start-offboarding-dialog"`) {
		t.Fatal("start offboarding offered while one is open")
	}
	if !strings.Contains(body, `id="start-onboarding-dialog"`) || !strings.Contains(body, `href="/hr/checklists/31"`) {
		t.Fatal("onboarding dialog or checklist link missing")
	}
}

func TestEmployeeDetailWithoutChecklistSource(t *testing.T) {
	rec := httptest.NewRecorder()
	newChecklistDetailRouter(t, []string{shared.PermHRChecklistAdmin, shared.PermHREmployeeView}, nil).ServeHTTP(rec, employeeRequest(t, http.MethodGet, "/hr/employees/8?tab=assets", nil))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `data-tab="assets"`) {
		t.Fatalf("status=%d assets tab rendered without a checklist source", rec.Code)
	}
}
