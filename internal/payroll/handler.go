package payroll

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type Handler struct {
	logger    *slog.Logger
	service   *Service
	processor *PayslipProcessor
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
}

func NewHandler(logger *slog.Logger, service *Service, processor *PayslipProcessor, templates *view.Engine, csrf *shared.CSRFManager, middleware rbac.Middleware) *Handler {
	return &Handler{logger: logger, service: service, processor: processor, templates: templates, csrf: csrf, rbac: middleware}
}

func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) { r.Use(h.rbac.RequireAny("payroll.view", "payroll.process")); r.Get("/", h.list) })
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("payroll.process"))
		r.Post("/", h.create)
		r.Post("/{id}/calculate", h.calculate)
		r.Post("/{id}/submit", h.submit)
	})
	r.Group(func(r chi.Router) { r.Use(h.rbac.RequireAny("payroll.post")); r.Get("/{id}/bank.csv", h.bankCSV) })
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("payroll.payslip.own", "payroll.payslip.manager", "payroll.view"))
		r.Get("/payslips/{id}.pdf", h.payslip)
	})
}

func payrollUserID(r *http.Request) int64 {
	s := shared.SessionFromContext(r.Context())
	if s == nil {
		return 0
	}
	id, _ := strconv.ParseInt(s.User(), 10, 64)
	return id
}
func payrollCompanyID(r *http.Request) int64 {
	s := shared.SessionFromContext(r.Context())
	if s == nil {
		return 0
	}
	id, _ := strconv.ParseInt(s.Get("company_id"), 10, 64)
	return id
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.Runs(r.Context(), payrollCompanyID(r))
	s := shared.SessionFromContext(r.Context())
	token, _ := h.csrf.EnsureToken(r.Context(), s)
	_ = h.templates.Render(w, "pages/payroll/runs.html", view.TemplateData{Title: "Payroll", CurrentPath: r.URL.Path, CSRFToken: token, Flash: s.PopFlash(), Data: map[string]any{"Runs": items, "Error": err}})
}
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	period, _ := strconv.ParseInt(r.FormValue("period_id"), 10, 64)
	_, err := h.service.CreateDraft(r.Context(), payrollCompanyID(r), period, payrollUserID(r))
	h.redirect(w, r, err, "Payroll draft created")
}
func (h *Handler) calculate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_, err := h.service.Calculate(r.Context(), id)
	h.redirect(w, r, err, "Payroll calculated")
}
func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_, err := h.service.Submit(r.Context(), id, payrollUserID(r))
	h.redirect(w, r, err, "Payroll submitted for approval")
}
func (h *Handler) bankCSV(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	data, err := h.service.BankCSV(r.Context(), id)
	if err != nil {
		shared.WriteHTTPError(w, http.StatusConflict, shared.UserSafeMessage(err))
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=payroll-bank.csv")
	_, _ = w.Write(data)
}
func (h *Handler) payslip(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	staff := false
	if h.rbac.Service != nil {
		perms, _ := h.rbac.Service.EffectivePermissions(r.Context(), payrollUserID(r))
		for _, p := range perms {
			if p == "payroll.view" || p == "payroll.process" || p == "payroll.post" {
				staff = true
				break
			}
		}
	}
	line, err := h.service.Payslip(r.Context(), id, payrollUserID(r), staff)
	if err != nil {
		shared.WriteHTTPError(w, http.StatusForbidden, "")
		return
	}
	pdf, err := h.processor.Render(r.Context(), PayslipRecord{ID: id, Line: line, PeriodCode: line.PeriodCode})
	if err != nil {
		shared.WriteHTTPError(w, http.StatusBadGateway, "")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=payslip.pdf")
	_, _ = w.Write(pdf)
}
func (h *Handler) redirect(w http.ResponseWriter, r *http.Request, err error, success string) {
	s := shared.SessionFromContext(r.Context())
	if err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: shared.UserSafeMessage(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: success})
	}
	http.Redirect(w, r, "/payroll", http.StatusSeeOther)
}
