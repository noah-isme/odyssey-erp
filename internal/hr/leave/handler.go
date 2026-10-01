package leave

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
		r.Use(h.rbac.RequireAny(shared.PermHRLeaveRequest, shared.PermHRLeaveAdmin))
		r.Get("/", h.list)
		r.Post("/", h.submit)
		r.Post("/{id}/cancel", h.cancel)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermHRLeaveAdmin))
		r.Post("/balances", h.seedBalance)
		r.Post("/types", h.createType)
	})
}

func uid(r *http.Request) int64 {
	s := shared.SessionFromContext(r.Context())
	if s == nil {
		return 0
	}
	id, _ := strconv.ParseInt(s.User(), 10, 64)
	return id
}

func (h *Handler) hasLeaveAdmin(r *http.Request, userID int64) bool {
	if h.rbac.Service == nil {
		return false
	}
	perms, err := h.rbac.Service.EffectivePermissions(r.Context(), userID)
	if err != nil {
		return false
	}
	for _, p := range perms {
		if p == shared.PermHRLeaveAdmin || p == "admin" || p == "*" {
			return true
		}
	}
	return false
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	s := shared.SessionFromContext(r.Context())
	company, _ := strconv.ParseInt(s.Get("company_id"), 10, 64)
	currentUserID := uid(r)
	year := time.Now().Year()

	activeTab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if activeTab == "" {
		activeTab = "my-requests"
	}

	ownBalances, _ := h.service.GetOwnBalances(r.Context(), currentUserID, year)
	items, err := h.service.ListOwn(r.Context(), currentUserID)
	types, _ := h.service.Types(r.Context(), company)

	isAdmin := h.hasLeaveAdmin(r, currentUserID)
	var allRequests []Request
	var allBalances []EmployeeBalanceSummary
	var activeEmployees []EmployeeOption

	if isAdmin {
		statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
		searchFilter := strings.TrimSpace(r.URL.Query().Get("q"))
		allRequests, _ = h.service.ListAll(r.Context(), company, LeaveFilter{
			Status: statusFilter,
			Search: searchFilter,
		})
		allBalances, _ = h.service.ListAllBalances(r.Context(), company, year)
		activeEmployees, _ = h.service.ListActiveEmployees(r.Context(), company)
	}

	token, _ := h.csrf.EnsureToken(r.Context(), s)
	_ = h.templates.Render(w, "pages/hr/leave.html", view.TemplateData{
		Title:       "Leave Management",
		CurrentPath: r.URL.Path,
		CSRFToken:   token,
		Flash:       s.PopFlash(),
		Data: map[string]any{
			"Requests":        items,
			"OwnBalances":     ownBalances,
			"AllRequests":     allRequests,
			"AllBalances":     allBalances,
			"ActiveEmployees": activeEmployees,
			"Types":           types,
			"IsAdmin":         isAdmin,
			"Year":            year,
			"Tab":             activeTab,
			"StatusFilter":    r.URL.Query().Get("status"),
			"Search":          r.URL.Query().Get("q"),
			"Error":           err,
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

func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	start, _ := time.Parse("2006-01-02", r.FormValue("start_date"))
	end, _ := time.Parse("2006-01-02", r.FormValue("end_date"))
	typeID, _ := strconv.ParseInt(r.FormValue("leave_type_id"), 10, 64)

	_, err := h.service.Submit(r.Context(), CreateInput{
		UserID:      uid(r),
		LeaveTypeID: typeID,
		StartDate:   start,
		EndDate:     end,
		Reason:      r.FormValue("reason"),
	})

	s := shared.SessionFromContext(r.Context())
	if err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: hrFlashError(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: "Leave request submitted successfully"})
	}
	http.Redirect(w, r, "/hr/leave?tab=my-requests", http.StatusSeeOther)
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	requestID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	err := h.service.Cancel(r.Context(), requestID, uid(r))

	s := shared.SessionFromContext(r.Context())
	if err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: hrFlashError(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: "Leave request cancelled successfully"})
	}
	http.Redirect(w, r, "/hr/leave?tab=my-requests", http.StatusSeeOther)
}

func (h *Handler) seedBalance(w http.ResponseWriter, r *http.Request) {
	s := shared.SessionFromContext(r.Context())
	company, _ := strconv.ParseInt(s.Get("company_id"), 10, 64)
	employeeID, _ := strconv.ParseInt(r.FormValue("employee_id"), 10, 64)
	typeID, _ := strconv.ParseInt(r.FormValue("leave_type_id"), 10, 64)
	year, _ := strconv.Atoi(r.FormValue("year"))
	if year == 0 {
		year = time.Now().Year()
	}
	days, _ := strconv.ParseFloat(r.FormValue("days"), 64)

	err := h.service.SeedBalance(r.Context(), company, employeeID, typeID, year, days)
	if err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: hrFlashError(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: "Leave balance allocated successfully"})
	}
	http.Redirect(w, r, "/hr/leave?tab=balances", http.StatusSeeOther)
}

func (h *Handler) createType(w http.ResponseWriter, r *http.Request) {
	s := shared.SessionFromContext(r.Context())
	company, _ := strconv.ParseInt(s.Get("company_id"), 10, 64)
	code := r.FormValue("code")
	name := r.FormValue("name")
	defaultDays, _ := strconv.ParseFloat(r.FormValue("default_days"), 64)

	_, err := h.service.CreateType(r.Context(), company, code, name, defaultDays)
	if err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: hrFlashError(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: "Leave type created successfully"})
	}
	http.Redirect(w, r, "/hr/leave?tab=balances", http.StatusSeeOther)
}
