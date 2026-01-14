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
	// View routes
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermFinanceARView))
		r.Get("/", h.listInvoices)
		r.Get("/invoices", h.listInvoices)
		r.Get("/invoices/new", h.showCreateInvoiceForm)
		r.Get("/invoices/{id}", h.showInvoiceDetail)
		r.Get("/payments", h.listPayments)
		r.Get("/payments/new", h.showCreatePaymentForm)
		r.Get("/aging", h.showARAgingReport)
		r.Get("/customer-statement", h.showCustomerStatement)
	})

	// Create routes
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll(shared.PermFinanceAREdit))
		r.Post("/invoices", h.createARInvoice)
		r.Post("/invoices/from-delivery/{doID}", h.createInvoiceFromDelivery)
		r.Post("/payments", h.createARPayment)
	})

	// Workflow routes
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll(shared.PermFinanceAREdit))
		r.Post("/invoices/{id}/post", h.postInvoice)
		r.Post("/invoices/{id}/void", h.voidInvoice)
	})
}

type formErrors map[string]string

// listInvoices shows the AR invoice list.
func (h *Handler) listInvoices(w http.ResponseWriter, r *http.Request) {
	status := ARInvoiceStatus(r.URL.Query().Get("status"))
	customerIDStr := r.URL.Query().Get("customer_id")
	var customerID int64
	if customerIDStr != "" {
		customerID, _ = strconv.ParseInt(customerIDStr, 10, 64)
	}

	invoices, err := h.service.ListARInvoices(r.Context(), ListARInvoicesRequest{
		Status:     status,
		CustomerID: customerID,
		Limit:      100,
	})
	if err != nil {
		h.logger.Error("list AR invoices", slog.Any("error", err))
		h.render(w, r, "pages/ar/ar_invoice_form.html", map[string]any{
			"Errors": formErrors{"general": shared.UserSafeMessage(err)},
		}, http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/ar/ar_invoice_form.html", map[string]any{
		"Invoices":       invoices,
		"StatusFilter":   status,
		"CustomerFilter": customerID,
	}, http.StatusOK)
}

// showInvoiceDetail shows a single invoice with details.
func (h *Handler) showInvoiceDetail(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid invoice ID", http.StatusBadRequest)
		return
	}

	invoice, err := h.service.GetARInvoiceWithDetails(r.Context(), id)
	if err != nil {
		h.logger.Error("get AR invoice", slog.Any("error", err), slog.Int64("id", id))
		h.render(w, r, "pages/ar/ar_invoice_form.html", map[string]any{
			"Errors": formErrors{"general": shared.UserSafeMessage(err)},
		}, http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/ar/ar_invoice_form.html", map[string]any{
		"Invoice": invoice,
	}, http.StatusOK)
}

// showCreateInvoiceForm shows the create invoice form.
func (h *Handler) showCreateInvoiceForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/ar/ar_invoice_form.html", map[string]any{
		"Errors": formErrors{},
	}, http.StatusOK)
}

// createARInvoice handles invoice creation.
func (h *Handler) createARInvoice(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	customerID, _ := strconv.ParseInt(r.PostFormValue("customer_id"), 10, 64)
	soID, _ := strconv.ParseInt(r.PostFormValue("so_id"), 10, 64)
	total, _ := strconv.ParseFloat(r.PostFormValue("total"), 64)
	dueDate, _ := time.Parse("2006-01-02", r.PostFormValue("due_date"))

	sess := shared.SessionFromContext(r.Context())
	userID := getUserID(sess)

	invoice, err := h.service.CreateARInvoice(r.Context(), CreateARInvoiceInput{
		CustomerID: customerID,
		SOID:       soID,
		Currency:   r.PostFormValue("currency"),
		Total:      total,
		DueDate:    dueDate,
		CreatedBy:  userID,
	})
	if err != nil {
		h.logger.Error("create AR invoice", slog.Any("error", err))
		h.render(w, r, "pages/ar/ar_invoice_form.html", map[string]any{
			"Errors": formErrors{"general": shared.UserSafeMessage(err)},
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/finance/ar/invoices/"+strconv.FormatInt(invoice.ID, 10), "success", "AR Invoice created")
}

// createInvoiceFromDelivery creates invoice from delivery order.
func (h *Handler) createInvoiceFromDelivery(w http.ResponseWriter, r *http.Request) {
	doIDStr := chi.URLParam(r, "doID")
	doID, err := strconv.ParseInt(doIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid delivery order ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	dueDate, _ := time.Parse("2006-01-02", r.PostFormValue("due_date"))
	if dueDate.IsZero() {
		dueDate = time.Now().AddDate(0, 0, 30) // Default 30 days
	}

	sess := shared.SessionFromContext(r.Context())
	userID := getUserID(sess)

	invoice, err := h.service.CreateARInvoiceFromDelivery(r.Context(), CreateARInvoiceFromDeliveryInput{
		DeliveryOrderID: doID,
		DueDate:         dueDate,
		CreatedBy:       userID,
	})
	if err != nil {
		h.logger.Error("create invoice from delivery", slog.Any("error", err), slog.Int64("do_id", doID))
		h.redirectWithFlash(w, r, "/delivery/orders/"+doIDStr, "error", shared.UserSafeMessage(err))
		return
	}

	h.redirectWithFlash(w, r, "/finance/ar/invoices/"+strconv.FormatInt(invoice.ID, 10), "success", "Invoice created from delivery order")
}

// postInvoice posts a draft invoice.
func (h *Handler) postInvoice(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid invoice ID", http.StatusBadRequest)
		return
	}

	sess := shared.SessionFromContext(r.Context())
	userID := getUserID(sess)

	if err := h.service.PostARInvoice(r.Context(), PostARInvoiceInput{
		InvoiceID: id,
		PostedBy:  userID,
	}); err != nil {
		h.logger.Error("post AR invoice", slog.Any("error", err), slog.Int64("id", id))
		h.redirectWithFlash(w, r, "/finance/ar/invoices/"+idStr, "error", shared.UserSafeMessage(err))
		return
	}

	h.redirectWithFlash(w, r, "/finance/ar/invoices/"+idStr, "success", "Invoice posted successfully")
}

// voidInvoice voids an invoice.
func (h *Handler) voidInvoice(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid invoice ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	sess := shared.SessionFromContext(r.Context())
	userID := getUserID(sess)
	reason := r.PostFormValue("reason")

	if err := h.service.VoidARInvoice(r.Context(), VoidARInvoiceInput{
		InvoiceID:  id,
		VoidedBy:   userID,
		VoidReason: reason,
	}); err != nil {
		h.logger.Error("void AR invoice", slog.Any("error", err), slog.Int64("id", id))
		h.redirectWithFlash(w, r, "/finance/ar/invoices/"+idStr, "error", shared.UserSafeMessage(err))
		return
	}

	h.redirectWithFlash(w, r, "/finance/ar/invoices/"+idStr, "success", "Invoice voided")
}

// listPayments shows payment list.
func (h *Handler) listPayments(w http.ResponseWriter, r *http.Request) {
	payments, err := h.service.GetARPayments(r.Context())
	if err != nil {
		h.logger.Error("list AR payments", slog.Any("error", err))
		h.render(w, r, "pages/ar/ar_payment_form.html", map[string]any{
			"Errors": formErrors{"general": shared.UserSafeMessage(err)},
		}, http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/ar/ar_payment_form.html", map[string]any{
		"Payments": payments,
	}, http.StatusOK)
}

// showCreatePaymentForm shows the create payment form.
func (h *Handler) showCreatePaymentForm(w http.ResponseWriter, r *http.Request) {
	// Get outstanding invoices for allocation
	invoices, _ := h.service.ListARInvoices(r.Context(), ListARInvoicesRequest{
		Status: ARStatusPosted,
		Limit:  100,
	})

	h.render(w, r, "pages/ar/ar_payment_form.html", map[string]any{
		"Errors":   formErrors{},
		"Invoices": invoices,
	}, http.StatusOK)
}

// createARPayment handles payment creation.
func (h *Handler) createARPayment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	invoiceID, _ := strconv.ParseInt(r.PostFormValue("ar_invoice_id"), 10, 64)
	amount, _ := strconv.ParseFloat(r.PostFormValue("amount"), 64)
	paidAt, _ := time.Parse("2006-01-02", r.PostFormValue("paid_at"))
	if paidAt.IsZero() {
		paidAt = time.Now()
	}

	sess := shared.SessionFromContext(r.Context())
	userID := getUserID(sess)

	_, err := h.service.RegisterARPayment(r.Context(), CreateARPaymentInput{
		Amount:    amount,
		PaidAt:    paidAt,
		Method:    r.PostFormValue("method"),
		Note:      r.PostFormValue("note"),
		CreatedBy: userID,
		Allocations: []PaymentAllocationInput{
			{ARInvoiceID: invoiceID, Amount: amount},
		},
	})
	if err != nil {
		h.logger.Error("create AR payment", slog.Any("error", err))
		h.render(w, r, "pages/ar/ar_payment_form.html", map[string]any{
			"Errors": formErrors{"general": shared.UserSafeMessage(err)},
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/finance/ar/payments", "success", "Payment recorded")
}

// showARAgingReport shows aging report.
func (h *Handler) showARAgingReport(w http.ResponseWriter, r *http.Request) {
	aging, err := h.service.CalculateARAging(r.Context(), time.Now())
	if err != nil {
		h.logger.Error("calculate AR aging", slog.Any("error", err))
		h.render(w, r, "pages/ar/ar_aging_report.html", map[string]any{
			"Errors": formErrors{"general": shared.UserSafeMessage(err)},
		}, http.StatusInternalServerError)
		return
	}

	total := aging.Current + aging.Bucket30 + aging.Bucket60 + aging.Bucket90 + aging.Bucket120

	h.render(w, r, "pages/ar/ar_aging_report.html", map[string]any{
		"Aging": aging,
		"Total": total,
	}, http.StatusOK)
}

// showCustomerStatement shows customer statement.
func (h *Handler) showCustomerStatement(w http.ResponseWriter, r *http.Request) {
	invoices, err := h.service.ListARInvoices(r.Context(), ListARInvoicesRequest{Limit: 1000})
	if err != nil {
		h.logger.Error("get customer statement", slog.Any("error", err))
		h.render(w, r, "pages/ar/customer_statement.html", map[string]any{
			"Errors": formErrors{"general": shared.UserSafeMessage(err)},
		}, http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/ar/customer_statement.html", map[string]any{
		"Invoices": invoices,
	}, http.StatusOK)
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, template string, data map[string]any, status int) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	viewData := view.TemplateData{
		Title:       "Accounts Receivable",
		CSRFToken:   csrfToken,
		Flash:       flash,
		CurrentPath: r.URL.Path,
		Data:        data,
	}
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

func getUserID(sess *shared.Session) int64 {
	if sess == nil {
		return 0
	}
	// User() returns string, parse to int64
	userStr := sess.User()
	if userStr == "" {
		return 0
	}
	id, _ := strconv.ParseInt(userStr, 10, 64)
	return id
}
