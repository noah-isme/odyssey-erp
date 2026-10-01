package employees

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type Handler struct {
	logger    *slog.Logger
	service   *Service
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
}

func NewHandler(l *slog.Logger, s *Service, t *view.Engine, c *shared.CSRFManager, r rbac.Middleware) *Handler {
	return &Handler{l, s, t, c, r}
}

func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermHREmployeeView, shared.PermHREmployeeAdmin))
		r.Get("/", h.list)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermHREmployeeAdmin))
		r.Post("/", h.create)
		r.Post("/{id}/update", h.update)
		r.Post("/departments", h.createDepartment)
		r.Post("/positions", h.createPosition)
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
	company, _ := strconv.ParseInt(s.Get("company_id"), 10, 64)

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
	company, _ := strconv.ParseInt(s.Get("company_id"), 10, 64)
	hire, _ := time.Parse("2006-01-02", r.FormValue("hire_date"))

	_, err := h.service.Create(r.Context(), CreateInput{
		CompanyID:      company,
		UserID:         opt(r.FormValue("user_id")),
		DepartmentID:   opt(r.FormValue("department_id")),
		PositionID:     opt(r.FormValue("position_id")),
		ManagerID:      opt(r.FormValue("manager_id")),
		EmployeeNumber: r.FormValue("employee_number"),
		Name:           r.FormValue("name"),
		Email:          r.FormValue("email"),
		HireDate:       hire,
	})

	if err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: hrFlashError(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: "Employee created successfully"})
	}
	http.Redirect(w, r, "/hr/employees?tab=employees", http.StatusSeeOther)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	s := shared.SessionFromContext(r.Context())
	company, _ := strconv.ParseInt(s.Get("company_id"), 10, 64)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	hire, _ := time.Parse("2006-01-02", r.FormValue("hire_date"))

	_, err := h.service.Update(r.Context(), company, id, UpdateInput{
		UserID:         opt(r.FormValue("user_id")),
		DepartmentID:   opt(r.FormValue("department_id")),
		PositionID:     opt(r.FormValue("position_id")),
		ManagerID:      opt(r.FormValue("manager_id")),
		EmployeeNumber: r.FormValue("employee_number"),
		Name:           r.FormValue("name"),
		Email:          r.FormValue("email"),
		HireDate:       hire,
		Status:         r.FormValue("status"),
	})

	if err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: hrFlashError(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: "Employee updated successfully"})
	}
	http.Redirect(w, r, "/hr/employees?tab=employees", http.StatusSeeOther)
}

func (h *Handler) createDepartment(w http.ResponseWriter, r *http.Request) {
	s := shared.SessionFromContext(r.Context())
	company, _ := strconv.ParseInt(s.Get("company_id"), 10, 64)

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
	company, _ := strconv.ParseInt(s.Get("company_id"), 10, 64)

	_, err := h.service.CreatePosition(r.Context(), company, r.FormValue("code"), r.FormValue("name"))
	if err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: hrFlashError(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: "Position created successfully"})
	}
	http.Redirect(w, r, "/hr/employees?tab=positions", http.StatusSeeOther)
}
