package leave

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
	r.Use(h.rbac.RequireAny("hr.leave.request", "hr.leave.admin"))
	r.Get("/", h.list)
	r.Post("/", h.submit)
}
func uid(r *http.Request) int64 {
	s := shared.SessionFromContext(r.Context())
	if s == nil {
		return 0
	}
	id, _ := strconv.ParseInt(s.User(), 10, 64)
	return id
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	s := shared.SessionFromContext(r.Context())
	company, _ := strconv.ParseInt(s.Get("company_id"), 10, 64)
	items, err := h.service.ListOwn(r.Context(), uid(r))
	types, _ := h.service.Types(r.Context(), company)
	token, _ := h.csrf.EnsureToken(r.Context(), s)
	_ = h.templates.Render(w, "pages/hr/leave.html", view.TemplateData{Title: "Leave", CurrentPath: r.URL.Path, CSRFToken: token, Flash: s.PopFlash(), Data: map[string]any{"Requests": items, "Types": types, "Error": err}})
}
func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	start, _ := time.Parse("2006-01-02", r.FormValue("start_date"))
	end, _ := time.Parse("2006-01-02", r.FormValue("end_date"))
	typeID, _ := strconv.ParseInt(r.FormValue("leave_type_id"), 10, 64)
	_, err := h.service.Submit(r.Context(), CreateInput{UserID: uid(r), LeaveTypeID: typeID, StartDate: start, EndDate: end, Reason: r.FormValue("reason")})
	s := shared.SessionFromContext(r.Context())
	if err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: shared.UserSafeMessage(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: "Leave request submitted"})
	}
	http.Redirect(w, r, "/hr/leave", http.StatusSeeOther)
}
