package checklists

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

// The handler tests run as user 3 in company 1.
const (
	hUserID    = int64(3)
	hCompanyID = int64(1)
)

type handlerPerms struct{ permissions []string }

func (p handlerPerms) EffectivePermissions(context.Context, int64) ([]string, error) {
	return p.permissions, nil
}

// handlerFakeService is an in-memory HandlerService that records writes.
type handlerFakeService struct {
	checklist      Checklist
	createdTpl     []TemplateInput
	updatedTpl     []int64
	addedItems     []ItemInput
	deletedItems   []int64
	started        []StartInput
	completedTasks []int64
	skippedTasks   []int64
	taskAdminFlags []bool
	offboarded     []int64
	onboarded      []int64
	cancelled      []int64
	assigned       [][2]int64
	returned       []int64
	listCalls      int
	templateCalls  int
}

func (f *handlerFakeService) writes() int {
	return len(f.createdTpl) + len(f.updatedTpl) + len(f.addedItems) + len(f.deletedItems) + len(f.started) +
		len(f.completedTasks) + len(f.skippedTasks) + len(f.offboarded) + len(f.onboarded) + len(f.cancelled) +
		len(f.assigned) + len(f.returned)
}

func (f *handlerFakeService) ListTemplates(context.Context, int64) ([]Template, error) {
	f.templateCalls++
	return []Template{{ID: 21, Code: "OFF-STD", Name: "Standard offboarding", Kind: KindOffboarding, IsActive: true,
		Items: []Item{{ID: 201, TemplateID: 21, Seq: 1, Title: "Collect laptop", AssigneeRole: RoleIT, DefaultAssigneeName: "IT Desk", ActionKey: ActionKeyReturnAssets}}}}, nil
}

func (f *handlerFakeService) CreateTemplate(_ context.Context, _ int64, in TemplateInput, _ int64) (Template, error) {
	f.createdTpl = append(f.createdTpl, in)
	return Template{ID: 22}, nil
}

func (f *handlerFakeService) UpdateTemplate(_ context.Context, _, id int64, _ TemplateUpdate, _ int64) (Template, error) {
	f.updatedTpl = append(f.updatedTpl, id)
	return Template{ID: id}, nil
}

func (f *handlerFakeService) AddItem(_ context.Context, _, _ int64, in ItemInput, _ int64) (Item, error) {
	f.addedItems = append(f.addedItems, in)
	return Item{ID: 202}, nil
}

func (f *handlerFakeService) DeleteItem(_ context.Context, _, id, _ int64) error {
	f.deletedItems = append(f.deletedItems, id)
	return nil
}

func (f *handlerFakeService) Start(_ context.Context, in StartInput) (Checklist, error) {
	f.started = append(f.started, in)
	return Checklist{ID: 55}, nil
}

func (f *handlerFakeService) CompleteTask(_ context.Context, _, id, _ int64, isAdmin bool, _ string) error {
	f.completedTasks = append(f.completedTasks, id)
	f.taskAdminFlags = append(f.taskAdminFlags, isAdmin)
	return nil
}

func (f *handlerFakeService) SkipTask(_ context.Context, _, id, _ int64, isAdmin bool, _ string) error {
	f.skippedTasks = append(f.skippedTasks, id)
	f.taskAdminFlags = append(f.taskAdminFlags, isAdmin)
	return nil
}

func (f *handlerFakeService) IsTaskAssignee(_ context.Context, companyID, taskID, userID int64) (bool, error) {
	if companyID != hCompanyID {
		return false, nil
	}
	for _, t := range f.checklist.Tasks {
		if t.ID == taskID && t.AssigneeUserID != nil && *t.AssigneeUserID == userID {
			return true, nil
		}
	}
	return false, nil
}

func (f *handlerFakeService) MyTasks(_ context.Context, _, userID int64) ([]Task, error) {
	if userID != hUserID {
		return nil, nil
	}
	due := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	uid := hUserID
	return []Task{{ID: 401, ChecklistID: 31, Seq: 1, Title: "Revoke VPN access", AssigneeRole: RoleIT, AssigneeUserID: &uid,
		DueDate: &due, Status: TaskOpen, EmployeeID: 11, EmployeeName: "Rina Wijaya", Kind: KindOffboarding}}, nil
}

func (f *handlerFakeService) Get(_ context.Context, companyID, id int64) (Checklist, error) {
	if companyID != hCompanyID || id != f.checklist.ID {
		return Checklist{}, ErrChecklistNotFound
	}
	return f.checklist, nil
}

func (f *handlerFakeService) List(context.Context, int64, Filter) ([]Checklist, error) {
	f.listCalls++
	return []Checklist{{ID: 31, EmployeeName: "Rina Wijaya", EmployeeNumber: "E-011", TemplateName: "Standard offboarding",
		Kind: KindOffboarding, Status: StatusOpen, StartedOn: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC), OpenTasks: 2}}, nil
}

func (f *handlerFakeService) CompleteOffboarding(_ context.Context, _, id, _ int64) (OffboardingResult, error) {
	f.offboarded = append(f.offboarded, id)
	return OffboardingResult{UserDeactivated: true}, nil
}

func (f *handlerFakeService) CompleteOnboarding(_ context.Context, _, id, _ int64) error {
	f.onboarded = append(f.onboarded, id)
	return nil
}

func (f *handlerFakeService) CancelOffboarding(_ context.Context, _, id, _ int64, _ string) error {
	f.cancelled = append(f.cancelled, id)
	return nil
}

func (f *handlerFakeService) AssetsHeldBy(context.Context, int64, int64) ([]Asset, error) {
	return []Asset{{ID: 61, Number: "FA-0061", Name: "ThinkPad X1", Status: "ACTIVE"}}, nil
}

func (f *handlerFakeService) AssignAsset(_ context.Context, _, assetID, employeeID, _ int64) error {
	f.assigned = append(f.assigned, [2]int64{assetID, employeeID})
	return nil
}

func (f *handlerFakeService) ReturnAsset(_ context.Context, _, assetID, _ int64) error {
	f.returned = append(f.returned, assetID)
	return nil
}

type handlerFakePeople struct{ reports []employees.JobHistory }

func (p handlerFakePeople) DirectReportsAsOf(context.Context, int64, int64, time.Time) ([]employees.JobHistory, error) {
	return p.reports, nil
}

func (p handlerFakePeople) ListAvailableUsers(context.Context) ([]employees.UserOption, error) {
	return []employees.UserOption{{ID: 300, Name: "IT Desk", Email: "it@example.com"}}, nil
}

func offboardingChecklist() Checklist {
	term := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	due := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	me, other := hUserID, int64(300)
	return Checklist{ID: 31, CompanyID: hCompanyID, EmployeeID: 11, EmployeeName: "Rina Wijaya", EmployeeNumber: "E-011",
		TemplateName: "Standard offboarding", Kind: KindOffboarding, Status: StatusOpen,
		StartedOn: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC), TerminationDate: &term, TerminationReason: "Resignation", OpenTasks: 2,
		Tasks: []Task{
			{ID: 401, ChecklistID: 31, Seq: 1, Title: "Revoke VPN access", AssigneeRole: RoleIT, AssigneeUserID: &me, AssigneeName: "Me", DueDate: &due, Status: TaskOpen},
			{ID: 402, ChecklistID: 31, Seq: 2, Title: "Exit interview", AssigneeRole: RoleHR, AssigneeUserID: &other, AssigneeName: "HR Desk", Status: TaskOpen},
		}}
}

func checklistRequest(t *testing.T, method, path string, form url.Values) (*http.Request, *shared.Session) {
	t.Helper()
	return checklistRequestAs(t, "3", method, path, form)
}

func checklistRequestAs(t *testing.T, userID, method, path string, form url.Values) (*http.Request, *shared.Session) {
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
	session.SetUser(userID)
	session.Set("company_id", "1")
	return req.WithContext(shared.ContextWithSession(req.Context(), session)), session
}

func newChecklistRouter(t *testing.T, perms []string, svc *handlerFakeService, people People, withTemplates bool) http.Handler {
	t.Helper()
	var engine *view.Engine
	if withTemplates {
		var err error
		engine, err = view.NewEngine()
		if err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler(nil, svc, people, engine, shared.NewCSRFManager("secret"), rbac.Middleware{Service: handlerPerms{perms}})
	h.now = func() time.Time { return time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC) }
	router := chi.NewRouter()
	router.Route("/hr/checklists", h.MountRoutes)
	return router
}

func TestChecklistRoutePermissions(t *testing.T) {
	view := []string{shared.PermHREmployeeView}
	cases := []struct {
		name         string
		perms        []string
		method, path string
		form         url.Values
	}{
		{"page requires employee view or checklist admin", []string{shared.PermHROvertimeRequest}, http.MethodGet, "/hr/checklists", nil},
		{"detail requires employee view or checklist admin", []string{shared.PermHROvertimeRequest}, http.MethodGet, "/hr/checklists/31", nil},
		{"task done requires employee view or checklist admin", []string{shared.PermHRLeaveRequest}, http.MethodPost, "/hr/checklists/tasks/401/done", url.Values{}},
		{"create template requires checklist admin", append(view, shared.PermHREmployeeAdmin), http.MethodPost, "/hr/checklists/templates", url.Values{"code": {"X"}, "name": {"X"}, "kind": {KindOnboarding}}},
		{"update template requires checklist admin", view, http.MethodPost, "/hr/checklists/templates/21/update", url.Values{"code": {"X"}}},
		{"add item requires checklist admin", view, http.MethodPost, "/hr/checklists/templates/21/items", url.Values{"title": {"X"}}},
		{"delete item requires checklist admin", view, http.MethodPost, "/hr/checklists/items/201/delete", url.Values{}},
		{"start requires checklist admin", append(view, shared.PermHREmployeeAdmin), http.MethodPost, "/hr/checklists/start", url.Values{"employee_id": {"11"}, "template_id": {"21"}}},
		{"complete requires checklist admin", view, http.MethodPost, "/hr/checklists/31/complete", url.Values{}},
		{"cancel requires checklist admin", view, http.MethodPost, "/hr/checklists/31/cancel", url.Values{"reason": {"x"}}},
		{"asset assign requires checklist admin", append(view, shared.PermHREmployeeAdmin), http.MethodPost, "/hr/checklists/assets/assign", url.Values{"employee_id": {"11"}, "asset_id": {"61"}}},
		{"asset return requires checklist admin", view, http.MethodPost, "/hr/checklists/assets/61/return", url.Values{"employee_id": {"11"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &handlerFakeService{checklist: offboardingChecklist()}
			// User 9 holds no task on checklist 31; user 3 (the assignee) is covered below.
			req, _ := checklistRequestAs(t, "9", tc.method, tc.path, tc.form)
			rec := httptest.NewRecorder()
			newChecklistRouter(t, tc.perms, svc, nil, false).ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status=%d want 403", rec.Code)
			}
			if svc.writes() != 0 {
				t.Fatal("forbidden request reached the checklist service")
			}
		})
	}
}

// A line manager is assigned MANAGER-role tasks but holds neither
// hr.employee.view nor hr.checklist.admin by default.
func TestAssigneeWithoutHRPermissionsCanViewAndCloseOwnTask(t *testing.T) {
	noPerms := []string{shared.PermHROvertimeRequest}
	t.Run("assignee sees the page and the checklist", func(t *testing.T) {
		for _, path := range []string{"/hr/checklists", "/hr/checklists/31"} {
			svc := &handlerFakeService{checklist: offboardingChecklist()}
			req, _ := checklistRequest(t, http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			newChecklistRouter(t, noPerms, svc, nil, true).ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s status=%d want 200", path, rec.Code)
			}
			if path == "/hr/checklists/31" && (!strings.Contains(rec.Body.String(), "Revoke VPN access") || !strings.Contains(rec.Body.String(), "Exit interview")) {
				t.Fatal("assignee must see the whole checklist, read-only for other tasks")
			}
			if svc.templateCalls != 0 || svc.listCalls != 0 {
				t.Fatal("admin data must not load for an assignee without permissions")
			}
		}
	})
	t.Run("assignee can mark own task done but not someone else's", func(t *testing.T) {
		svc := &handlerFakeService{checklist: offboardingChecklist()}
		req, _ := checklistRequest(t, http.MethodPost, "/hr/checklists/tasks/401/done", url.Values{"checklist_id": {"31"}})
		rec := httptest.NewRecorder()
		newChecklistRouter(t, noPerms, svc, nil, false).ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther || len(svc.completedTasks) != 1 || svc.completedTasks[0] != 401 || svc.taskAdminFlags[0] {
			t.Fatalf("status=%d completed=%v adminFlags=%v", rec.Code, svc.completedTasks, svc.taskAdminFlags)
		}

		svc = &handlerFakeService{checklist: offboardingChecklist()}
		req, _ = checklistRequest(t, http.MethodPost, "/hr/checklists/tasks/402/done", url.Values{})
		rec = httptest.NewRecorder()
		newChecklistRouter(t, noPerms, svc, nil, false).ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden || svc.writes() != 0 {
			t.Fatalf("someone else's task: status=%d writes=%d, want 403 and no write", rec.Code, svc.writes())
		}
	})
	t.Run("non-assignee without permissions is forbidden everywhere", func(t *testing.T) {
		for _, tc := range []struct{ method, path string }{
			{http.MethodGet, "/hr/checklists"}, {http.MethodGet, "/hr/checklists/31"}, {http.MethodGet, "/hr/checklists/999"},
			{http.MethodPost, "/hr/checklists/tasks/401/done"}, {http.MethodPost, "/hr/checklists/tasks/401/skip"},
		} {
			svc := &handlerFakeService{checklist: offboardingChecklist()}
			req, _ := checklistRequestAs(t, "9", tc.method, tc.path, url.Values{})
			rec := httptest.NewRecorder()
			newChecklistRouter(t, noPerms, svc, nil, false).ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden || svc.writes() != 0 {
				t.Fatalf("%s %s status=%d writes=%d, want 403 and no write", tc.method, tc.path, rec.Code, svc.writes())
			}
		}
	})
	t.Run("admin routes stay admin-only for an assignee", func(t *testing.T) {
		svc := &handlerFakeService{checklist: offboardingChecklist()}
		req, _ := checklistRequest(t, http.MethodPost, "/hr/checklists/31/complete", url.Values{})
		rec := httptest.NewRecorder()
		newChecklistRouter(t, noPerms, svc, nil, false).ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden || svc.writes() != 0 {
			t.Fatalf("status=%d writes=%d, want 403", rec.Code, svc.writes())
		}
	})
}

func TestChecklistWriteHappyPaths(t *testing.T) {
	admin := []string{shared.PermHRChecklistAdmin}
	cases := []struct {
		name         string
		perms        []string
		path         string
		form         url.Values
		wantLocation string
		check        func(t *testing.T, svc *handlerFakeService)
	}{
		{"start offboarding", admin, "/hr/checklists/start",
			url.Values{"employee_id": {"11"}, "template_id": {"21"}, "termination_date": {"2026-10-31"}, "termination_reason": {" Resignation "}},
			"/hr/checklists/55", func(t *testing.T, svc *handlerFakeService) {
				if len(svc.started) != 1 {
					t.Fatalf("started=%v", svc.started)
				}
				in := svc.started[0]
				if in.CompanyID != hCompanyID || in.EmployeeID != 11 || in.TemplateID != 21 || in.ActorID != hUserID ||
					in.TerminationDate == nil || !in.TerminationDate.Equal(time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)) || in.TerminationReason != "Resignation" {
					t.Fatalf("start input=%+v", in)
				}
			}},
		{"start onboarding without termination date", admin, "/hr/checklists/start",
			url.Values{"employee_id": {"11"}, "template_id": {"20"}}, "/hr/checklists/55", func(t *testing.T, svc *handlerFakeService) {
				if len(svc.started) != 1 || svc.started[0].TerminationDate != nil {
					t.Fatalf("started=%+v", svc.started)
				}
			}},
		{"assignee marks own task done", []string{shared.PermHREmployeeView}, "/hr/checklists/tasks/401/done",
			url.Values{"notes": {"revoked"}, "return_to": {"my-tasks"}}, "/hr/checklists?tab=my-tasks", func(t *testing.T, svc *handlerFakeService) {
				if len(svc.completedTasks) != 1 || svc.completedTasks[0] != 401 || svc.taskAdminFlags[0] {
					t.Fatalf("completed=%v adminFlags=%v", svc.completedTasks, svc.taskAdminFlags)
				}
			}},
		{"admin skips task from detail", admin, "/hr/checklists/tasks/402/skip",
			url.Values{"checklist_id": {"31"}}, "/hr/checklists/31", func(t *testing.T, svc *handlerFakeService) {
				if len(svc.skippedTasks) != 1 || svc.skippedTasks[0] != 402 || !svc.taskAdminFlags[0] {
					t.Fatalf("skipped=%v adminFlags=%v", svc.skippedTasks, svc.taskAdminFlags)
				}
			}},
		{"complete dispatches offboarding by kind", admin, "/hr/checklists/31/complete", url.Values{}, "/hr/checklists/31",
			func(t *testing.T, svc *handlerFakeService) {
				if len(svc.offboarded) != 1 || len(svc.onboarded) != 0 {
					t.Fatalf("offboarded=%v onboarded=%v", svc.offboarded, svc.onboarded)
				}
			}},
		{"cancel offboarding", admin, "/hr/checklists/31/cancel", url.Values{"reason": {"Retained"}}, "/hr/checklists/31",
			func(t *testing.T, svc *handlerFakeService) {
				if len(svc.cancelled) != 1 || svc.cancelled[0] != 31 {
					t.Fatalf("cancelled=%v", svc.cancelled)
				}
			}},
		{"create template", admin, "/hr/checklists/templates", url.Values{"code": {"ON-STD"}, "name": {"Standard onboarding"}, "kind": {"onboarding"}, "is_active": {"on"}},
			"/hr/checklists?tab=templates", func(t *testing.T, svc *handlerFakeService) {
				want := TemplateInput{Code: "ON-STD", Name: "Standard onboarding", Kind: KindOnboarding, IsActive: true}
				if len(svc.createdTpl) != 1 || svc.createdTpl[0] != want {
					t.Fatalf("created=%+v", svc.createdTpl)
				}
			}},
		{"add item", admin, "/hr/checklists/templates/21/items",
			url.Values{"title": {"Collect badge"}, "assignee_role": {"hr"}, "default_assignee_user_id": {"300"}, "due_offset_days": {"-2"}, "action_key": {"return_assets"}},
			"/hr/checklists?tab=templates", func(t *testing.T, svc *handlerFakeService) {
				if len(svc.addedItems) != 1 {
					t.Fatalf("added=%v", svc.addedItems)
				}
				in := svc.addedItems[0]
				if in.Title != "Collect badge" || in.AssigneeRole != RoleHR || in.DefaultAssigneeUserID == nil || *in.DefaultAssigneeUserID != 300 ||
					in.DueOffsetDays != -2 || in.ActionKey != ActionKeyReturnAssets || in.Seq != 0 {
					t.Fatalf("item=%+v", in)
				}
			}},
		{"assign asset returns to employee assets tab", admin, "/hr/checklists/assets/assign", url.Values{"employee_id": {"11"}, "asset_id": {"61"}},
			"/hr/employees/11?tab=assets", func(t *testing.T, svc *handlerFakeService) {
				if len(svc.assigned) != 1 || svc.assigned[0] != [2]int64{61, 11} {
					t.Fatalf("assigned=%v", svc.assigned)
				}
			}},
		{"return asset from checklist", admin, "/hr/checklists/assets/61/return", url.Values{"checklist_id": {"31"}},
			"/hr/checklists/31", func(t *testing.T, svc *handlerFakeService) {
				if len(svc.returned) != 1 || svc.returned[0] != 61 {
					t.Fatalf("returned=%v", svc.returned)
				}
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &handlerFakeService{checklist: offboardingChecklist()}
			req, session := checklistRequest(t, http.MethodPost, tc.path, tc.form)
			rec := httptest.NewRecorder()
			newChecklistRouter(t, tc.perms, svc, nil, false).ServeHTTP(rec, req)
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

func TestCompleteOnboardingDispatch(t *testing.T) {
	c := offboardingChecklist()
	c.Kind, c.TerminationDate = KindOnboarding, nil
	svc := &handlerFakeService{checklist: c}
	req, _ := checklistRequest(t, http.MethodPost, "/hr/checklists/31/complete", url.Values{})
	rec := httptest.NewRecorder()
	newChecklistRouter(t, []string{shared.PermHRChecklistAdmin}, svc, nil, false).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || len(svc.onboarded) != 1 || len(svc.offboarded) != 0 {
		t.Fatalf("status=%d onboarded=%v offboarded=%v", rec.Code, svc.onboarded, svc.offboarded)
	}
}

func TestStartRejectsBadTerminationDate(t *testing.T) {
	svc := &handlerFakeService{}
	req, session := checklistRequest(t, http.MethodPost, "/hr/checklists/start", url.Values{"employee_id": {"11"}, "template_id": {"21"}, "termination_date": {"31/10/2026"}})
	rec := httptest.NewRecorder()
	newChecklistRouter(t, []string{shared.PermHRChecklistAdmin}, svc, nil, false).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/hr/employees/11?tab=assets" {
		t.Fatalf("status=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
	if len(svc.started) != 0 {
		t.Fatal("invalid date reached Start")
	}
	if f := session.PopFlash(); f == nil || f.Kind != "error" || f.Message != "Invalid checklist start" {
		t.Fatalf("flash=%+v", f)
	}
}

func TestChecklistDetailVisibility(t *testing.T) {
	cases := []struct {
		name   string
		perms  []string
		tasks  func(c *Checklist)
		status int
	}{
		{"admin sees any checklist", []string{shared.PermHRChecklistAdmin}, func(c *Checklist) { c.Tasks = nil }, http.StatusOK},
		{"assignee sees the checklist", []string{shared.PermHREmployeeView}, func(*Checklist) {}, http.StatusOK},
		{"non-assignee viewer gets 404", []string{shared.PermHREmployeeView}, func(c *Checklist) { c.Tasks = c.Tasks[1:] }, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := offboardingChecklist()
			tc.tasks(&c)
			svc := &handlerFakeService{checklist: c}
			req, _ := checklistRequest(t, http.MethodGet, "/hr/checklists/31", nil)
			rec := httptest.NewRecorder()
			newChecklistRouter(t, tc.perms, svc, nil, true).ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status=%d want %d", rec.Code, tc.status)
			}
		})
	}
	t.Run("cross-tenant id is 404", func(t *testing.T) {
		svc := &handlerFakeService{checklist: offboardingChecklist()}
		req, _ := checklistRequest(t, http.MethodGet, "/hr/checklists/999", nil)
		rec := httptest.NewRecorder()
		newChecklistRouter(t, []string{shared.PermHRChecklistAdmin}, svc, nil, true).ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status=%d want 404", rec.Code)
		}
	})
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

func TestChecklistsPageRenders(t *testing.T) {
	cases := []struct {
		name      string
		perms     []string
		tab       string
		wantAdmin bool
	}{
		{"viewer sees only my tasks", []string{shared.PermHREmployeeView}, "", false},
		{"viewer cannot open templates tab", []string{shared.PermHREmployeeView}, "?tab=templates", false},
		{"admin sees all tabs", []string{shared.PermHRChecklistAdmin}, "?tab=templates", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &handlerFakeService{}
			req, _ := checklistRequest(t, http.MethodGet, "/hr/checklists"+tc.tab, nil)
			rec := httptest.NewRecorder()
			newChecklistRouter(t, tc.perms, svc, handlerFakePeople{}, true).ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			for _, want := range []string{"Revoke VPN access", "Rina Wijaya", `action="/hr/checklists/tasks/401/done"`, `action="/hr/checklists/tasks/401/skip"`, "OVERDUE"} {
				if !strings.Contains(body, want) {
					t.Fatalf("my tasks missing %q", want)
				}
			}
			adminMarkers := []string{`data-tab="all"`, `data-tab="templates"`, `id="create-template-dialog"`, `action="/hr/checklists/templates/21/items"`,
				`action="/hr/checklists/items/201/delete"`, "Collect laptop", "it@example.com", `href="/hr/checklists/31" class="btn`}
			for _, m := range adminMarkers {
				if got := strings.Contains(body, m); got != tc.wantAdmin {
					t.Fatalf("%q rendered=%v want %v", m, got, tc.wantAdmin)
				}
			}
			if !tc.wantAdmin && (svc.listCalls != 0 || svc.templateCalls != 0) {
				t.Fatal("admin data loaded for a viewer")
			}
			if tc.wantAdmin && !strings.Contains(body, `data-tab="templates" class="btn tab active"`) {
				t.Fatal("templates tab not active")
			}
			requireCSRFOnPostForms(t, body)
		})
	}
}

func TestChecklistDetailPageRenders(t *testing.T) {
	reports := handlerFakePeople{reports: []employees.JobHistory{{EmployeeID: 12, EmployeeName: "Budi Santoso"}}}
	t.Run("admin offboarding", func(t *testing.T) {
		svc := &handlerFakeService{checklist: offboardingChecklist()}
		req, _ := checklistRequest(t, http.MethodGet, "/hr/checklists/31", nil)
		rec := httptest.NewRecorder()
		newChecklistRouter(t, []string{shared.PermHRChecklistAdmin}, svc, reports, true).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		for _, want := range []string{
			"Revoke VPN access", "Exit interview", "OVERDUE",
			`action="/hr/checklists/tasks/401/done"`, `action="/hr/checklists/tasks/402/done"`,
			"FA-0061", `action="/hr/checklists/assets/61/return"`,
			"data-direct-report-warning", "Budi Santoso",
			`action="/hr/checklists/31/complete"`, "Complete Offboarding",
			"data-global-deactivation-warning", "whole Odyssey instance",
			`id="cancel-offboarding-dialog"`, `action="/hr/checklists/31/cancel"`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("detail missing %q", want)
			}
		}
		requireCSRFOnPostForms(t, body)
	})
	t.Run("assignee sees own task actions only", func(t *testing.T) {
		svc := &handlerFakeService{checklist: offboardingChecklist()}
		req, _ := checklistRequest(t, http.MethodGet, "/hr/checklists/31", nil)
		rec := httptest.NewRecorder()
		newChecklistRouter(t, []string{shared.PermHREmployeeView}, svc, reports, true).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `action="/hr/checklists/tasks/401/done"`) {
			t.Fatal("own task action missing")
		}
		for _, absent := range []string{`action="/hr/checklists/tasks/402/done"`, `action="/hr/checklists/31/complete"`, `action="/hr/checklists/31/cancel"`, `action="/hr/checklists/assets/61/return"`} {
			if strings.Contains(body, absent) {
				t.Fatalf("non-admin sees %q", absent)
			}
		}
		requireCSRFOnPostForms(t, body)
	})
	t.Run("completed onboarding has no actions", func(t *testing.T) {
		c := offboardingChecklist()
		c.Kind, c.Status, c.TerminationDate = KindOnboarding, StatusCompleted, nil
		svc := &handlerFakeService{checklist: c}
		req, _ := checklistRequest(t, http.MethodGet, "/hr/checklists/31", nil)
		rec := httptest.NewRecorder()
		newChecklistRouter(t, []string{shared.PermHRChecklistAdmin}, svc, reports, true).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d", rec.Code)
		}
		body := rec.Body.String()
		for _, absent := range []string{"/done\"", "/complete\"", "data-direct-report-warning", "data-global-deactivation-warning", "OVERDUE"} {
			if strings.Contains(body, absent) {
				t.Fatalf("completed checklist renders %q", absent)
			}
		}
	})
}
