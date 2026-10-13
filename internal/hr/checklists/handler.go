package checklists

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// HandlerService is the subset of *Service used by the HTTP handler.
type HandlerService interface {
	ListTemplates(ctx context.Context, companyID int64) ([]Template, error)
	CreateTemplate(ctx context.Context, companyID int64, in TemplateInput, actorID int64) (Template, error)
	UpdateTemplate(ctx context.Context, companyID, templateID int64, in TemplateUpdate, actorID int64) (Template, error)
	AddItem(ctx context.Context, companyID, templateID int64, in ItemInput, actorID int64) (Item, error)
	DeleteItem(ctx context.Context, companyID, itemID, actorID int64) error
	Start(ctx context.Context, in StartInput) (Checklist, error)
	CompleteTask(ctx context.Context, companyID, taskID, actorID int64, isAdmin bool, notes string) error
	SkipTask(ctx context.Context, companyID, taskID, actorID int64, isAdmin bool, notes string) error
	MyTasks(ctx context.Context, companyID, userID int64) ([]Task, error)
	IsTaskAssignee(ctx context.Context, companyID, taskID, userID int64) (bool, error)
	Get(ctx context.Context, companyID, checklistID int64) (Checklist, error)
	List(ctx context.Context, companyID int64, f Filter) ([]Checklist, error)
	CompleteOffboarding(ctx context.Context, companyID, checklistID, actorID int64) (OffboardingResult, error)
	CompleteOnboarding(ctx context.Context, companyID, checklistID, actorID int64) error
	CancelOffboarding(ctx context.Context, companyID, checklistID, actorID int64, reason string) error
	AssetsHeldBy(ctx context.Context, companyID, employeeID int64) ([]Asset, error)
	AssignAsset(ctx context.Context, companyID, assetID, employeeID, actorID int64) error
	ReturnAsset(ctx context.Context, companyID, assetID, actorID int64) error
}

// People is the employee lookup surface used by the handler.
// *employees.Service satisfies it.
type People interface {
	DirectReportsAsOf(ctx context.Context, companyID, managerEmployeeID int64, asOf time.Time) ([]employees.JobHistory, error)
	ListAvailableUsers(ctx context.Context) ([]employees.UserOption, error)
}

var (
	_ HandlerService = (*Service)(nil)
	_ People         = (*employees.Service)(nil)
)

// Handler serves /hr/checklists.
type Handler struct {
	logger    *slog.Logger
	service   HandlerService
	people    People
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
	now       func() time.Time
}

// NewHandler builds a Handler. people may be nil (no direct-report warning
// and no default-assignee picker then).
func NewHandler(l *slog.Logger, s HandlerService, p People, t *view.Engine, c *shared.CSRFManager, r rbac.Middleware) *Handler {
	if l == nil {
		l = slog.Default()
	}
	return &Handler{logger: l, service: s, people: p, templates: t, csrf: c, rbac: r, now: time.Now}
}

// MountRoutes mounts /hr/checklists.
func (h *Handler) MountRoutes(r chi.Router) {
	// Viewing and closing tasks needs hr.employee.view or hr.checklist.admin, or
	// being the assignee: a line manager who was assigned a task (the MANAGER
	// role) does not hold those permissions by default but must be able to see
	// the checklist and mark their own task done. The service still limits
	// closing a task to its assignee (or an admin), company-scoped.
	r.With(h.viewOrAssignee(assigneeList)).Get("/", h.list)
	r.With(h.viewOrAssignee(assigneeChecklist)).Get("/{id}", h.detail)
	r.With(h.viewOrAssignee(assigneeTask)).Post("/tasks/{tid}/done", h.completeTask)
	r.With(h.viewOrAssignee(assigneeTask)).Post("/tasks/{tid}/skip", h.skipTask)
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermHRChecklistAdmin))
		r.Post("/templates", h.createTemplate)
		r.Post("/templates/{tid}/update", h.updateTemplate)
		r.Post("/templates/{tid}/items", h.addItem)
		r.Post("/items/{iid}/delete", h.deleteItem)
		r.Post("/start", h.start)
		r.Post("/{id}/complete", h.complete)
		r.Post("/{id}/cancel", h.cancelOffboarding)
		r.Post("/assets/assign", h.assignAsset)
		r.Post("/assets/{aid}/return", h.returnAsset)
	})
}

type assigneeScope int

const (
	assigneeList      assigneeScope = iota // has at least one open assigned task
	assigneeChecklist                      // has a task on the checklist in the path
	assigneeTask                           // is the assignee of the task in the path
)

// viewOrAssignee lets through holders of hr.employee.view or hr.checklist.admin
// and, failing that, users who are an assignee within the requested scope;
// everyone else gets 403. It never widens access beyond what the handler and
// service already enforce for non-admins (own tasks only).
func (h *Handler) viewOrAssignee(scope assigneeScope) func(http.Handler) http.Handler {
	viewers := h.rbac.RequireAny(shared.PermHREmployeeView, shared.PermHRChecklistAdmin)
	return func(next http.Handler) http.Handler {
		viaPermission := viewers(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ident, ok := shared.IdentityFromContext(r.Context())
			if !ok {
				shared.WriteHTTPError(w, http.StatusForbidden, "")
				return
			}
			if h.hasViewPermission(r, ident.UserID) {
				viaPermission.ServeHTTP(w, r)
				return
			}
			allowed := false
			var err error
			switch scope {
			case assigneeList:
				var tasks []Task
				tasks, err = h.service.MyTasks(r.Context(), ident.CompanyID, ident.UserID)
				allowed = len(tasks) > 0
			case assigneeChecklist:
				var c Checklist
				if c, err = h.service.Get(r.Context(), ident.CompanyID, pathID(r, "id")); err == nil {
					for _, t := range c.Tasks {
						if t.AssigneeUserID != nil && *t.AssigneeUserID == ident.UserID {
							allowed = true
							break
						}
					}
				} else if errors.Is(err, ErrNotFound) || errors.Is(err, ErrChecklistNotFound) {
					err = nil
				}
			case assigneeTask:
				allowed, err = h.service.IsTaskAssignee(r.Context(), ident.CompanyID, pathID(r, "tid"), ident.UserID)
			}
			if err != nil {
				h.logger.Error("checklist assignee check", slog.Any("error", err))
				shared.WriteHTTPError(w, http.StatusInternalServerError, "")
				return
			}
			if !allowed {
				shared.WriteHTTPError(w, http.StatusForbidden, "")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// hasViewPermission reports whether the user holds hr.employee.view or
// hr.checklist.admin (or an admin wildcard).
func (h *Handler) hasViewPermission(r *http.Request, userID int64) bool {
	if h.rbac.Service == nil {
		return false
	}
	perms, err := h.rbac.Service.EffectivePermissions(r.Context(), userID)
	if err != nil {
		h.logger.Error("hr checklist permissions", slog.Any("error", err))
		return false
	}
	for _, p := range perms {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case shared.PermHREmployeeView, shared.PermHRChecklistAdmin:
			return true
		}
	}
	return false
}

// isAdmin reports whether the user holds hr.checklist.admin.
func (h *Handler) isAdmin(r *http.Request, userID int64) bool {
	if h.rbac.Service == nil {
		return false
	}
	perms, err := h.rbac.Service.EffectivePermissions(r.Context(), userID)
	if err != nil {
		h.logger.Error("hr checklist permissions", slog.Any("error", err))
		return false
	}
	for _, p := range perms {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "admin", "*", shared.PermHRChecklistAdmin:
			return true
		}
	}
	return false
}

func (h *Handler) today() time.Time {
	if h.now == nil {
		return dateOnly(time.Now())
	}
	return dateOnly(h.now())
}

// flashError shows "hr: ..." domain errors to the user and hides the rest.
func flashError(err error) string {
	msg := err.Error()
	if strings.HasPrefix(msg, "hr: ") {
		if trimmed := strings.TrimSpace(strings.TrimPrefix(msg, "hr: ")); trimmed != "" {
			return strings.ToUpper(trimmed[:1]) + trimmed[1:]
		}
	}
	return shared.UserSafeMessage(err)
}

func (h *Handler) redirect(w http.ResponseWriter, r *http.Request, to string, err error, success string) {
	if s := shared.SessionFromContext(r.Context()); s != nil {
		if err != nil {
			s.AddFlash(shared.FlashMessage{Kind: "error", Message: flashError(err)})
		} else {
			s.AddFlash(shared.FlashMessage{Kind: "success", Message: success})
		}
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

func pathID(r *http.Request, key string) int64 {
	id, err := strconv.ParseInt(chi.URLParam(r, key), 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func formID(r *http.Request, key string) int64 {
	id, err := strconv.ParseInt(strings.TrimSpace(r.FormValue(key)), 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func formBool(r *http.Request, key string) bool {
	switch strings.ToLower(strings.TrimSpace(r.FormValue(key))) {
	case "on", "true", "1", "yes":
		return true
	}
	return false
}

func kindLabel(kind string) string {
	if kind == KindOffboarding {
		return "Offboarding"
	}
	return "Onboarding"
}

func checklistURL(id int64) string { return "/hr/checklists/" + strconv.FormatInt(id, 10) }

func employeeAssetsURL(id int64) string {
	return "/hr/employees/" + strconv.FormatInt(id, 10) + "?tab=assets"
}

const (
	myTasksURL   = "/hr/checklists?tab=my-tasks"
	templatesURL = "/hr/checklists?tab=templates"
)

func (h *Handler) render(w http.ResponseWriter, r *http.Request, page, title string, data map[string]any) {
	var token string
	var flash *shared.FlashMessage
	if s := shared.SessionFromContext(r.Context()); s != nil {
		token, _ = h.csrf.EnsureToken(r.Context(), s)
		flash = s.PopFlash()
	}
	if err := h.templates.Render(w, page, view.TemplateData{
		Title:       title,
		CurrentPath: r.URL.Path,
		CSRFToken:   token,
		Flash:       flash,
		Data:        data,
	}); err != nil {
		h.logger.Error("render checklist page", slog.String("page", page), slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
	}
}

// TaskRow is a task with its overdue flag for the templates.
type TaskRow struct {
	Task
	IsOverdue bool
	CanAct    bool
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	isAdmin := h.isAdmin(r, ident.UserID)
	today := h.today()

	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if (tab != "all" && tab != "templates") || !isAdmin {
		tab = "my-tasks"
	}

	tasks, err := h.service.MyTasks(r.Context(), ident.CompanyID, ident.UserID)
	if err != nil {
		h.logger.Error("my checklist tasks", slog.Any("error", err))
	}
	rows := make([]TaskRow, 0, len(tasks))
	for _, t := range tasks {
		rows = append(rows, TaskRow{Task: t, IsOverdue: t.Overdue(today), CanAct: true})
	}

	statusFilter := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status")))
	kindFilter := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("kind")))
	if statusFilter != "" && statusFilter != StatusOpen && statusFilter != StatusCompleted && statusFilter != StatusCancelled {
		statusFilter = ""
	}
	if kindFilter != "" && !validKind(kindFilter) {
		kindFilter = ""
	}
	data := map[string]any{
		"Tasks":        rows,
		"IsAdmin":      isAdmin,
		"Tab":          tab,
		"StatusFilter": statusFilter,
		"KindFilter":   kindFilter,
		"Roles":        []string{RoleHR, RoleIT, RoleManager},
		"ActionKeys":   []string{ActionKeyDeactivateUser, ActionKeyReturnAssets, ActionKeyCloseLeave, ActionKeyCloseBenefits},
		"Kinds":        []string{KindOnboarding, KindOffboarding},
	}
	if isAdmin {
		lists, lerr := h.service.List(r.Context(), ident.CompanyID, Filter{Status: statusFilter, Kind: kindFilter})
		if lerr != nil {
			h.logger.Error("list checklists", slog.Any("error", lerr))
			err = errors.Join(err, lerr)
		}
		templates, terr := h.service.ListTemplates(r.Context(), ident.CompanyID)
		if terr != nil {
			h.logger.Error("list checklist templates", slog.Any("error", terr))
			err = errors.Join(err, terr)
		}
		var users []employees.UserOption
		if h.people != nil {
			users, _ = h.people.ListAvailableUsers(r.Context())
		}
		data["Checklists"] = lists
		data["Templates"] = templates
		data["Users"] = users
	}
	data["Error"] = err
	h.render(w, r, "pages/hr/checklists.html", "Checklists", data)
}

// DirectReport is one employee still reporting to the leaver.
type DirectReport struct {
	EmployeeID int64
	Name       string
}

// directReports returns the distinct direct reports as of today and as of
// the termination date (the same checks CompleteOffboarding refuses on).
func (h *Handler) directReports(ctx context.Context, companyID int64, c Checklist, today time.Time) []DirectReport {
	if h.people == nil || c.Kind != KindOffboarding {
		return nil
	}
	days := []time.Time{today}
	if c.TerminationDate != nil && !dateOnly(*c.TerminationDate).Equal(today) {
		days = append(days, dateOnly(*c.TerminationDate))
	}
	seen := map[int64]bool{}
	var out []DirectReport
	for _, day := range days {
		reports, err := h.people.DirectReportsAsOf(ctx, companyID, c.EmployeeID, day)
		if err != nil {
			h.logger.Error("checklist direct reports", slog.Int64("checklist_id", c.ID), slog.Any("error", err))
			continue
		}
		for _, jh := range reports {
			if !seen[jh.EmployeeID] {
				seen[jh.EmployeeID] = true
				out = append(out, DirectReport{EmployeeID: jh.EmployeeID, Name: jh.EmployeeName})
			}
		}
	}
	return out
}

func (h *Handler) detail(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	id := pathID(r, "id")
	if id == 0 {
		shared.WriteHTTPError(w, http.StatusNotFound, "checklist not found")
		return
	}
	c, err := h.service.Get(r.Context(), ident.CompanyID, id)
	if errors.Is(err, ErrNotFound) {
		shared.WriteHTTPError(w, http.StatusNotFound, "checklist not found")
		return
	}
	if err != nil {
		h.logger.Error("get checklist", slog.Int64("checklist_id", id), slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
		return
	}
	isAdmin := h.isAdmin(r, ident.UserID)
	isAssignee := false
	for _, t := range c.Tasks {
		if t.AssigneeUserID != nil && *t.AssigneeUserID == ident.UserID {
			isAssignee = true
			break
		}
	}
	// Non-admins only see checklists they have a task on; others are
	// indistinguishable from missing ones.
	if !isAdmin && !isAssignee {
		shared.WriteHTTPError(w, http.StatusNotFound, "checklist not found")
		return
	}

	today := h.today()
	open := c.Status == StatusOpen
	rows := make([]TaskRow, 0, len(c.Tasks))
	for _, t := range c.Tasks {
		mine := t.AssigneeUserID != nil && *t.AssigneeUserID == ident.UserID
		rows = append(rows, TaskRow{Task: t, IsOverdue: open && t.Overdue(today), CanAct: open && t.Status == TaskOpen && (isAdmin || mine)})
	}
	assets, aerr := h.service.AssetsHeldBy(r.Context(), ident.CompanyID, c.EmployeeID)
	if aerr != nil {
		h.logger.Error("checklist assets", slog.Int64("checklist_id", id), slog.Any("error", aerr))
	}
	reports := h.directReports(r.Context(), ident.CompanyID, c, today)
	futureTermination := c.TerminationDate != nil && today.Before(dateOnly(*c.TerminationDate))

	h.render(w, r, "pages/hr/checklist_detail.html", fmt.Sprintf("%s checklist - %s", kindLabel(c.Kind), c.EmployeeName), map[string]any{
		"Checklist":         c,
		"Tasks":             rows,
		"Assets":            assets,
		"DirectReports":     reports,
		"IsAdmin":           isAdmin,
		"IsOffboarding":     c.Kind == KindOffboarding,
		"IsOpen":            open,
		"CanComplete":       isAdmin && open,
		"CanCancel":         isAdmin && open && c.Kind == KindOffboarding,
		"FutureTermination": futureTermination,
		"Error":             aerr,
	})
}

func (h *Handler) taskReturn(r *http.Request) string {
	if strings.TrimSpace(r.FormValue("return_to")) == "my-tasks" {
		return myTasksURL
	}
	if cid := formID(r, "checklist_id"); cid > 0 {
		return checklistURL(cid)
	}
	return myTasksURL
}

func (h *Handler) completeTask(w http.ResponseWriter, r *http.Request) {
	h.closeTask(w, r, false)
}

func (h *Handler) skipTask(w http.ResponseWriter, r *http.Request) {
	h.closeTask(w, r, true)
}

func (h *Handler) closeTask(w http.ResponseWriter, r *http.Request, skip bool) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	taskID := pathID(r, "tid")
	isAdmin := h.isAdmin(r, ident.UserID)
	notes := r.FormValue("notes")
	var err error
	success := "Task marked done"
	if skip {
		success = "Task skipped"
		err = h.service.SkipTask(r.Context(), ident.CompanyID, taskID, ident.UserID, isAdmin, notes)
	} else {
		err = h.service.CompleteTask(r.Context(), ident.CompanyID, taskID, ident.UserID, isAdmin, notes)
	}
	h.redirect(w, r, h.taskReturn(r), err, success)
}

func (h *Handler) createTemplate(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	_, err := h.service.CreateTemplate(r.Context(), ident.CompanyID, TemplateInput{
		Code:     r.FormValue("code"),
		Name:     r.FormValue("name"),
		Kind:     strings.ToUpper(strings.TrimSpace(r.FormValue("kind"))),
		IsActive: formBool(r, "is_active"),
	}, ident.UserID)
	h.redirect(w, r, templatesURL, err, "Checklist template created")
}

func (h *Handler) updateTemplate(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	_, err := h.service.UpdateTemplate(r.Context(), ident.CompanyID, pathID(r, "tid"), TemplateUpdate{
		Code:     r.FormValue("code"),
		Name:     r.FormValue("name"),
		IsActive: formBool(r, "is_active"),
	}, ident.UserID)
	h.redirect(w, r, templatesURL, err, "Checklist template updated")
}

func (h *Handler) addItem(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	var err error
	seq, offset := 0, 0
	if v := strings.TrimSpace(r.FormValue("seq")); v != "" {
		if seq, err = strconv.Atoi(v); err != nil || seq < 0 {
			err = ErrInvalidItem
		}
	}
	if v := strings.TrimSpace(r.FormValue("due_offset_days")); err == nil && v != "" {
		if offset, err = strconv.Atoi(v); err != nil {
			err = ErrInvalidItem
		}
	}
	if err == nil {
		var assignee *int64
		if id := formID(r, "default_assignee_user_id"); id > 0 {
			assignee = &id
		}
		_, err = h.service.AddItem(r.Context(), ident.CompanyID, pathID(r, "tid"), ItemInput{
			Seq:                   seq,
			Title:                 r.FormValue("title"),
			Description:           r.FormValue("description"),
			AssigneeRole:          strings.ToUpper(strings.TrimSpace(r.FormValue("assignee_role"))),
			DefaultAssigneeUserID: assignee,
			DueOffsetDays:         offset,
			ActionKey:             strings.ToUpper(strings.TrimSpace(r.FormValue("action_key"))),
		}, ident.UserID)
	}
	h.redirect(w, r, templatesURL, err, "Checklist item added")
}

func (h *Handler) deleteItem(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	err := h.service.DeleteItem(r.Context(), ident.CompanyID, pathID(r, "iid"), ident.UserID)
	h.redirect(w, r, templatesURL, err, "Checklist item deleted")
}

func (h *Handler) start(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	employeeID := formID(r, "employee_id")
	if employeeID == 0 {
		h.redirect(w, r, "/hr/employees?tab=employees", ErrEmployeeNotFound, "")
		return
	}
	in := StartInput{
		CompanyID:         ident.CompanyID,
		EmployeeID:        employeeID,
		TemplateID:        formID(r, "template_id"),
		ActorID:           ident.UserID,
		TerminationReason: strings.TrimSpace(r.FormValue("termination_reason")),
	}
	var err error
	if v := strings.TrimSpace(r.FormValue("termination_date")); v != "" {
		d, perr := time.Parse("2006-01-02", v)
		if perr != nil {
			err = ErrInvalidStart
		} else {
			in.TerminationDate = &d
		}
	}
	if err == nil {
		var c Checklist
		c, err = h.service.Start(r.Context(), in)
		if err == nil {
			h.redirect(w, r, checklistURL(c.ID), nil, "Checklist started; assignees have been notified")
			return
		}
	}
	h.redirect(w, r, employeeAssetsURL(employeeID), err, "")
}

func (h *Handler) complete(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	id := pathID(r, "id")
	c, err := h.service.Get(r.Context(), ident.CompanyID, id)
	if errors.Is(err, ErrNotFound) {
		shared.WriteHTTPError(w, http.StatusNotFound, "checklist not found")
		return
	}
	success := "Onboarding checklist completed"
	if err == nil {
		if c.Kind == KindOffboarding {
			var res OffboardingResult
			res, err = h.service.CompleteOffboarding(r.Context(), ident.CompanyID, id, ident.UserID)
			success = "Employee offboarded"
			if err == nil && res.UserDeactivated {
				success += "; the linked login has been deactivated"
			}
		} else {
			err = h.service.CompleteOnboarding(r.Context(), ident.CompanyID, id, ident.UserID)
		}
	}
	h.redirect(w, r, checklistURL(id), err, success)
}

func (h *Handler) cancelOffboarding(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	id := pathID(r, "id")
	err := h.service.CancelOffboarding(r.Context(), ident.CompanyID, id, ident.UserID, r.FormValue("reason"))
	h.redirect(w, r, checklistURL(id), err, "Offboarding cancelled; the termination date was cleared")
}

func (h *Handler) assignAsset(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	employeeID := formID(r, "employee_id")
	if employeeID == 0 {
		h.redirect(w, r, "/hr/employees?tab=employees", ErrEmployeeNotFound, "")
		return
	}
	err := h.service.AssignAsset(r.Context(), ident.CompanyID, formID(r, "asset_id"), employeeID, ident.UserID)
	h.redirect(w, r, employeeAssetsURL(employeeID), err, "Asset assigned")
}

func (h *Handler) returnAsset(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	err := h.service.ReturnAsset(r.Context(), ident.CompanyID, pathID(r, "aid"), ident.UserID)
	to := "/hr/employees?tab=employees"
	if cid := formID(r, "checklist_id"); cid > 0 {
		to = checklistURL(cid)
	} else if eid := formID(r, "employee_id"); eid > 0 {
		to = employeeAssetsURL(eid)
	}
	h.redirect(w, r, to, err, "Asset returned")
}
