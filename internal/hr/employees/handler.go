package employees

import (
	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
	"log/slog"
	"net/http"
	"strconv"
	"time"
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
	r.Group(func(r chi.Router) { r.Use(h.rbac.RequireAny(shared.PermHREmployeeAdmin)); r.Post("/", h.create) })
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	company, _ := strconv.ParseInt(shared.SessionFromContext(r.Context()).Get("company_id"), 10, 64)
	items, err := h.service.List(r.Context(), company)
	s := shared.SessionFromContext(r.Context())
	token, _ := h.csrf.EnsureToken(r.Context(), s)
	_ = h.templates.Render(w, "pages/hr/employees.html", view.TemplateData{Title: "Employees", CurrentPath: r.URL.Path, CSRFToken: token, Data: map[string]any{"Employees": items, "Error": err}})
}
func opt(v string) *int64 {
	id, _ := strconv.ParseInt(v, 10, 64)
	if id <= 0 {
		return nil
	}
	return &id
}
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	company, _ := strconv.ParseInt(shared.SessionFromContext(r.Context()).Get("company_id"), 10, 64)
	hire, _ := time.Parse("2006-01-02", r.FormValue("hire_date"))
	_, err := h.service.Create(r.Context(), CreateInput{CompanyID: company, UserID: opt(r.FormValue("user_id")), ManagerID: opt(r.FormValue("manager_id")), EmployeeNumber: r.FormValue("employee_number"), Name: r.FormValue("name"), Email: r.FormValue("email"), HireDate: hire})
	s := shared.SessionFromContext(r.Context())
	if err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: shared.UserSafeMessage(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: "Employee created"})
	}
	http.Redirect(w, r, "/hr/employees", http.StatusSeeOther)
}
