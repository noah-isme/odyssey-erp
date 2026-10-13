package employees

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/hr/compensation"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// EmployeeService is the employee-service surface used by Handler.
// *Service satisfies it.
type EmployeeService interface {
	List(ctx context.Context, companyID int64, filter ...EmployeeFilter) ([]Employee, error)
	GetByID(ctx context.Context, companyID, id int64) (Employee, error)
	Create(ctx context.Context, in CreateInput) (Employee, error)
	Update(ctx context.Context, companyID, id int64, in UpdateInput) (Employee, error)
	UpdateWithJobChange(ctx context.Context, companyID, id int64, in UpdateInput, job *JobChangeInput) (Employee, error)
	ListDepartments(ctx context.Context, companyID int64) ([]Department, error)
	ListPositions(ctx context.Context, companyID int64) ([]Position, error)
	ListAvailableUsers(ctx context.Context) ([]UserOption, error)
	CreateDepartment(ctx context.Context, companyID int64, code, name string) (Department, error)
	CreatePosition(ctx context.Context, companyID int64, code, name string) (Position, error)
	ListContacts(ctx context.Context, companyID, employeeID int64) ([]EmergencyContact, error)
	UpsertContact(ctx context.Context, companyID, employeeID int64, in ContactInput) (EmergencyContact, error)
	DeleteContact(ctx context.Context, companyID, employeeID, contactID, actorID int64) error
	ListJobHistory(ctx context.Context, companyID, employeeID int64) ([]JobHistory, error)
	AssignmentAsOf(ctx context.Context, companyID, employeeID int64, asOf time.Time) (JobHistory, error)
	RecordJobChange(ctx context.Context, companyID, employeeID int64, in JobChangeInput) (JobHistory, error)
}

// CompensationService is the compensation surface used by Handler.
// *compensation.Service satisfies it.
type CompensationService interface {
	ListForEmployee(ctx context.Context, companyID, employeeID int64) ([]compensation.Assignment, error)
	Change(ctx context.Context, companyID, employeeID int64, in compensation.ChangeInput) (compensation.Assignment, error)
	ChangeBank(ctx context.Context, companyID, employeeID int64, bank compensation.BankDetails, effectiveFrom time.Time, reason string, actorID int64) (compensation.Assignment, error)
}

var (
	_ EmployeeService     = (*Service)(nil)
	_ CompensationService = (*compensation.Service)(nil)
)

type Handler struct {
	logger       *slog.Logger
	service      EmployeeService
	compensation CompensationService
	checklists   ChecklistSource
	templates    *view.Engine
	csrf         *shared.CSRFManager
	rbac         rbac.Middleware
}

// NewHandler builds the employee handler. comp may be nil, in which case
// compensation data is never shown and compensation changes are rejected.
func NewHandler(l *slog.Logger, s EmployeeService, comp CompensationService, t *view.Engine, c *shared.CSRFManager, r rbac.Middleware) *Handler {
	if l == nil {
		l = slog.Default()
	}
	return &Handler{logger: l, service: s, compensation: comp, templates: t, csrf: c, rbac: r}
}

// SetChecklists enables the detail page's Assets tab and the start
// onboarding/offboarding dialogs (shown only with hr.checklist.admin). The
// checklist writes themselves are served by the checklists handler.
func (h *Handler) SetChecklists(c ChecklistSource) { h.checklists = c }

func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermHREmployeeView, shared.PermHREmployeeAdmin))
		r.Get("/", h.list)
		r.Get("/{id}", h.detail)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermHREmployeeAdmin))
		r.Post("/", h.create)
		r.Post("/{id}/update", h.update)
		r.Post("/{id}/profile", h.updateProfile)
		r.Post("/{id}/contacts", h.upsertContact)
		r.Post("/{id}/contacts/{cid}/delete", h.deleteContact)
		r.Post("/{id}/job-history", h.recordJobChange)
		r.Post("/departments", h.createDepartment)
		r.Post("/positions", h.createPosition)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermHRCompensationAdmin))
		r.Post("/{id}/compensation", h.changeCompensation)
	})
}

func opt(v string) *int64 {
	id, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if id <= 0 {
		return nil
	}
	return &id
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	s := shared.SessionFromContext(r.Context())
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	company := ident.CompanyID

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	deptID := opt(r.URL.Query().Get("department_id"))
	activeTab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if activeTab == "" {
		activeTab = "employees"
	}

	filter := EmployeeFilter{Search: q, DepartmentID: deptID, Status: status}
	items, err := h.service.List(r.Context(), company, filter)
	allEmployees, _ := h.service.List(r.Context(), company)
	depts, _ := h.service.ListDepartments(r.Context(), company)
	positions, _ := h.service.ListPositions(r.Context(), company)
	users, _ := h.service.ListAvailableUsers(r.Context())

	// The directory never shows sensitive fields; mask them for every caller.
	for i := range items {
		MaskSensitive(&items[i])
	}
	for i := range allEmployees {
		MaskSensitive(&allEmployees[i])
	}

	activeCount := 0
	for _, emp := range allEmployees {
		if strings.EqualFold(emp.Status, "ACTIVE") {
			activeCount++
		}
	}

	var selectedDept int64
	if deptID != nil {
		selectedDept = *deptID
	}

	perms := h.permissions(r, ident.UserID)

	token, _ := h.csrf.EnsureToken(r.Context(), s)
	_ = h.templates.Render(w, "pages/hr/employees.html", view.TemplateData{
		Title:       "Employee Directory",
		CurrentPath: r.URL.Path,
		CSRFToken:   token,
		Flash:       s.PopFlash(),
		Data: map[string]any{
			"Employees":   items,
			"AllStaff":    allEmployees,
			"Departments": depts,
			"Positions":   positions,
			"Users":       users,
			// Initial compensation fields on the create dialog are shown only
			// to hr.compensation.admin holders; create() enforces the same.
			"CanAdminCompensation": perms.compensationAdmin && h.compensation != nil,
			"EmploymentTypes":      employmentTypes,
			"PTKPCodes":            ptkpCodes,
			"Filter": map[string]any{
				"Search":       q,
				"DepartmentID": selectedDept,
				"Status":       status,
				"Tab":          activeTab,
			},
			"Stats": map[string]int{
				"Total":       len(allEmployees),
				"Active":      activeCount,
				"Departments": len(depts),
				"Positions":   len(positions),
			},
			"Error": err,
		},
	})
}

func hrFlashError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.HasPrefix(msg, "hr: ") {
		trimmed := strings.TrimSpace(strings.TrimPrefix(msg, "hr: "))
		if len(trimmed) > 0 {
			return strings.ToUpper(trimmed[:1]) + trimmed[1:]
		}
	}
	return shared.UserSafeMessage(err)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	s := shared.SessionFromContext(r.Context())
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	company := ident.CompanyID
	hire, _ := time.Parse("2006-01-02", r.FormValue("hire_date"))

	in := CreateInput{
		CompanyID:      company,
		UserID:         opt(r.FormValue("user_id")),
		DepartmentID:   opt(r.FormValue("department_id")),
		PositionID:     opt(r.FormValue("position_id")),
		ManagerID:      opt(r.FormValue("manager_id")),
		EmployeeNumber: r.FormValue("employee_number"),
		Name:           r.FormValue("name"),
		Email:          r.FormValue("email"),
		HireDate:       hire,
		EmploymentType: r.FormValue("employment_type"),
		Grade:          r.FormValue("grade"),
		Profile: Profile{
			ContractStartDate: optDate(r.FormValue("contract_start_date")),
			ContractEndDate:   optDate(r.FormValue("contract_end_date")),
			ProbationEndDate:  optDate(r.FormValue("probation_end_date")),
		},
		ActorID: ident.UserID,
	}
	var err error
	if strings.TrimSpace(r.FormValue("base_salary")) != "" {
		if !h.permissions(r, ident.UserID).compensationAdmin {
			err = errCompensationForbidden
		} else {
			comp := compensationForm(r, ident.UserID)
			comp.EffectiveFrom = hire
			in.Compensation = &comp
		}
	}
	if err == nil {
		_, err = h.service.Create(r.Context(), in)
	}

	if err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: hrFlashError(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: "Employee created successfully"})
	}
	http.Redirect(w, r, "/hr/employees?tab=employees", http.StatusSeeOther)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	s := shared.SessionFromContext(r.Context())
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	company := ident.CompanyID
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	hire, _ := time.Parse("2006-01-02", r.FormValue("hire_date"))
	if err := h.applyUpdate(r, company, ident.UserID, id, hire); err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: hrFlashError(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: "Employee updated successfully"})
	}
	http.Redirect(w, r, "/hr/employees?tab=employees", http.StatusSeeOther)
}

// applyUpdate saves identity fields through Update (profile fields not on the
// form keep their stored values; sensitive fields are never written here) and,
// when department/position/manager/status differ from the current values,
// records a job change effective today (or the optional effective_date).
// The full detail-page flow arrives in P0.7.
func (h *Handler) applyUpdate(r *http.Request, company, actor, id int64, hire time.Time) error {
	current, err := h.service.GetByID(r.Context(), company, id)
	if err != nil {
		return err
	}
	in := UpdateInput{
		UserID:         opt(r.FormValue("user_id")),
		EmployeeNumber: r.FormValue("employee_number"),
		Name:           r.FormValue("name"),
		Email:          r.FormValue("email"),
		HireDate:       hire,
		Profile:        current.Profile,
		ActorID:        actor,
	}
	dept, pos, mgr := opt(r.FormValue("department_id")), opt(r.FormValue("position_id")), opt(r.FormValue("manager_id"))
	status := strings.ToUpper(strings.TrimSpace(r.FormValue("status")))
	if status == "" {
		status = current.Status
	}
	if sameID(dept, current.DepartmentID) && sameID(pos, current.PositionID) && sameID(mgr, current.ManagerID) && status == current.Status {
		_, err := h.service.Update(r.Context(), company, id, in)
		return err
	}
	// Default effective date: today (business timezone), but never before the
	// latest job record (a new or pre-boarded hire's record starts at
	// hire_date, which can be today or later); a record starting on the same
	// day is corrected in place.
	today := businessToday()
	validFrom := today
	history, err := h.service.ListJobHistory(r.Context(), company, id)
	if err != nil {
		return err
	}
	explicit, explicitErr := time.Parse("2006-01-02", r.FormValue("effective_date"))
	if len(history) > 0 && history[0].ValidFrom.After(today) {
		if len(history) > 1 && (explicitErr != nil || dateOnly(explicit).Equal(dateOnly(history[0].ValidFrom))) {
			// The form is prefilled from the hr_employees mirror, which does not
			// include the scheduled change. Correcting the scheduled record in
			// place (no date, or an explicit date equal to its start) would
			// silently drop it, and recording today would be rejected, so send
			// the user to the Job tab.
			return fmt.Errorf("hr: a job change is scheduled for %s; edit it from the employee's Job tab", dateOnly(history[0].ValidFrom).Format("2006-01-02"))
		}
		validFrom = dateOnly(history[0].ValidFrom)
	}
	if explicitErr == nil {
		validFrom = explicit
	}
	grade := ""
	if cur, err := h.service.AssignmentAsOf(r.Context(), company, id, validFrom); err == nil {
		grade = cur.Grade
	}
	reason := strings.TrimSpace(r.FormValue("change_reason"))
	if reason == "" {
		reason = "Updated from employee directory"
	}
	// One transaction: a rejected job change leaves no partial profile edit.
	_, err = h.service.UpdateWithJobChange(r.Context(), company, id, in, &JobChangeInput{
		DepartmentID:   dept,
		PositionID:     pos,
		ManagerID:      mgr,
		Status:         status,
		EmploymentType: current.EmploymentType,
		Grade:          grade,
		ValidFrom:      validFrom,
		Reason:         reason,
		ActorID:        actor,
	})
	return err
}

func (h *Handler) createDepartment(w http.ResponseWriter, r *http.Request) {
	s := shared.SessionFromContext(r.Context())
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	company := ident.CompanyID

	_, err := h.service.CreateDepartment(r.Context(), company, r.FormValue("code"), r.FormValue("name"))
	if err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: hrFlashError(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: "Department created successfully"})
	}
	http.Redirect(w, r, "/hr/employees?tab=departments", http.StatusSeeOther)
}

func (h *Handler) createPosition(w http.ResponseWriter, r *http.Request) {
	s := shared.SessionFromContext(r.Context())
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	company := ident.CompanyID

	_, err := h.service.CreatePosition(r.Context(), company, r.FormValue("code"), r.FormValue("name"))
	if err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: hrFlashError(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: "Position created successfully"})
	}
	http.Redirect(w, r, "/hr/employees?tab=positions", http.StatusSeeOther)
}
