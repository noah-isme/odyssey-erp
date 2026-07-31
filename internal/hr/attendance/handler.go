package attendance

import (
	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
	"log/slog"
	"net/http"
	"strconv"
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
	r.Use(h.rbac.RequireAny(shared.PermHRAttendanceImport))
	r.Get("/", h.page)
	r.Post("/import", h.importCSV)
}
func identity(r *http.Request) (int64, int64) {
	s := shared.SessionFromContext(r.Context())
	u, _ := strconv.ParseInt(s.User(), 10, 64)
	c, _ := strconv.ParseInt(s.Get("company_id"), 10, 64)
	return u, c
}
func (h *Handler) page(w http.ResponseWriter, r *http.Request) {
	_, company := identity(r)
	items, err := h.service.Recent(r.Context(), company)
	s := shared.SessionFromContext(r.Context())
	token, _ := h.csrf.EnsureToken(r.Context(), s)
	_ = h.templates.Render(w, "pages/hr/attendance.html", view.TemplateData{Title: "Attendance Import", CurrentPath: r.URL.Path, CSRFToken: token, Flash: s.PopFlash(), Data: map[string]any{"Imports": items, "Error": err}})
}
func (h *Handler) importCSV(w http.ResponseWriter, r *http.Request) {
	user, company := identity(r)
	err := r.ParseMultipartForm(5 << 20)
	var result ImportResult
	if err == nil {
		file, header, e := r.FormFile("file")
		err = e
		if e == nil {
			defer file.Close()
			result, err = h.service.Import(r.Context(), company, user, header.Filename, file)
		}
	}
	s := shared.SessionFromContext(r.Context())
	if err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: shared.UserSafeMessage(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: "Imported " + strconv.Itoa(result.Accepted) + " attendance rows"})
	}
	http.Redirect(w, r, "/hr/attendance", http.StatusSeeOther)
}
