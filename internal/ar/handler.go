package ar

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// Handler manages AR endpoints.
type Handler struct {
	logger    *slog.Logger
	service   *Service
	templates *view.Engine
	csrf      *shared.CSRFManager
	sessions  *shared.SessionManager
	rbac      rbac.Middleware
}

// NewHandler builds Handler instance.
func NewHandler(logger *slog.Logger, service *Service, templates *view.Engine, csrf *shared.CSRFManager, sessions *shared.SessionManager, rbac rbac.Middleware) *Handler {
	return &Handler{logger: logger, service: service, templates: templates, csrf: csrf, sessions: sessions, rbac: rbac}
}

// MountRoutes registers AR routes.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("ar.view"))
		r.Get("/invoices", h.showARInvoiceForm)
		r.Get("/payments", h.showARPaymentForm)
		r.Get("/aging", h.showARAgingReport)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("ar.edit"))
		r.Post("/invoices", h.createARInvoice)
		r.Post("/payments", h.createARPayment)
	})
}

type formErrors map[string]string

func (h *Handler) showARInvoiceForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/ar/ar_invoice_form.html", map[string]any{"Errors": formErrors{}}, http.StatusOK)
}

func (h *Handler) showARPaymentForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/ar/ar_payment_form.html", map[string]any{"Errors": formErrors{}}, http.StatusOK)
}

func (h *Handler) showARAgingReport(w http.ResponseWriter, r *http.Request) {
	aging, err := h.service.CalculateARAging(r.Context(), time.Now())
	if err != nil {
		h.render(w, r, "pages/ar/ar_aging_report.html", map[string]any{"Errors": formErrors{"general": err.Error()}}, http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/ar/ar_aging_report.html", map[string]any{"Aging": aging}, http.StatusOK)
}

func (h *Handler) createARInvoice(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	customerID, _ := strconv.ParseInt(r.PostFormValue("customer_id"), 10, 64)
	soID, _ := strconv.ParseInt(r.PostFormValue("so_id"), 10, 64)
	total, _ := strconv.ParseFloat(r.PostFormValue("total"), 64)
	dueDate, _ := time.Parse("2006-01-02", r.PostFormValue("due_date"))
	_, err := h.service.CreateARInvoiceFromSO(r.Context(), ARInvoiceInput{
		CustomerID: customerID,
		SOID:       soID,
		Number:     r.PostFormValue("number"),
		Currency:   r.PostFormValue("currency"),
		Total:      total,
		DueDate:    dueDate,
	})
	if err != nil {
		h.render(w, r, "pages/ar/ar_invoice_form.html", map[string]any{"Errors": formErrors{"general": err.Error()}}, http.StatusBadRequest)
		return
	}
	h.redirectWithFlash(w, r, "/finance/ar/invoices", "success", "AR Invoice created")
}

func (h *Handler) createARPayment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	invoiceID, _ := strconv.ParseInt(r.PostFormValue("ar_invoice_id"), 10, 64)
	amount, _ := strconv.ParseFloat(r.PostFormValue("amount"), 64)
	paidAt, _ := time.Parse("2006-01-02", r.PostFormValue("paid_at"))
	_, err := h.service.RegisterARPayment(r.Context(), ARPaymentInput{
		ARInvoiceID: invoiceID,
		Number:      r.PostFormValue("number"),
		Amount:      amount,
		PaidAt:      paidAt,
		Method:      r.PostFormValue("method"),
		Note:        r.PostFormValue("note"),
	})
	if err != nil {
		h.render(w, r, "pages/ar/ar_payment_form.html", map[string]any{"Errors": formErrors{"general": err.Error()}}, http.StatusBadRequest)
		return
	}
	h.redirectWithFlash(w, r, "/finance/ar/payments", "success", "AR Payment recorded")
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, template string, data map[string]any, status int) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	viewData := view.TemplateData{Title: "AR", CSRFToken: csrfToken, Flash: flash, CurrentPath: r.URL.Path, Data: data}
	w.WriteHeader(status)
	if err := h.templates.Render(w, template, viewData); err != nil {
		h.logger.Error("render template", slog.Any("error", err))
	}
}

func (h *Handler) redirectWithFlash(w http.ResponseWriter, r *http.Request, location, kind, message string) {
	if sess := shared.SessionFromContext(r.Context()); sess != nil {
		sess.AddFlash(shared.FlashMessage{Kind: kind, Message: message})
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}
