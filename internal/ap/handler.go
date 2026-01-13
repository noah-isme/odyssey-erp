package ap

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// Handler manages AP endpoints.
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

// MountRoutes registers AP routes.
func (h *Handler) MountRoutes(r chi.Router) {
	// View routes
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("finance.ap.view"))

		r.Get("/", h.listInvoices)
		r.Get("/invoices", h.listInvoices)
		r.Get("/invoices/new", h.showCreateInvoiceForm)
		r.Get("/invoices/{id}", h.showInvoiceDetail)
		r.Get("/payments", h.listPayments)
		r.Get("/payments/new", h.showCreatePaymentForm)
		r.Get("/payments/{id}", h.showPaymentDetail)
		r.Get("/aging", h.showAPAgingReport)
	})

	// Create/Action routes
	r.Group(func(r chi.Router) {
		r.With(h.rbac.RequireAny("finance.ap.create")).Post("/invoices", h.createAPInvoice)
		r.With(h.rbac.RequireAny("finance.ap.create")).Post("/invoices/from-grn/{grnID}", h.createInvoiceFromGRN)
		r.With(h.rbac.RequireAny("finance.ap.create")).Post("/invoices/from-po/{poID}", h.createInvoiceFromPO)
		r.With(h.rbac.RequireAny("finance.ap.post")).Post("/invoices/{id}/post", h.postInvoice)
		r.With(h.rbac.RequireAny("finance.ap.void")).Post("/invoices/{id}/void", h.voidInvoice)
		r.With(h.rbac.RequireAny("finance.ap.payment")).Post("/payments", h.createAPPayment)
	})
}

type formErrors map[string]string

// listInvoices shows the AP invoice list.
func (h *Handler) listInvoices(w http.ResponseWriter, r *http.Request) {
	status := APInvoiceStatus(r.URL.Query().Get("status"))
	supplierIDStr := r.URL.Query().Get("supplier_id")
	var supplierID int64
	if supplierIDStr != "" {
		supplierID, _ = strconv.ParseInt(supplierIDStr, 10, 64)
	}

	invoices, err := h.service.ListAPInvoices(r.Context(), ListAPInvoicesRequest{
		Status:     status,
		SupplierID: supplierID,
		Limit:      100,
	})
	if err != nil {
		h.logger.Error("list AP invoices", slog.Any("error", err))
		h.render(w, r, "pages/ap/ap_invoice_list.html", map[string]any{
			"Errors": formErrors{"general": shared.UserSafeMessage(err)},
		}, http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/ap/ap_invoice_list.html", map[string]any{
		"Invoices":       invoices,
		"StatusFilter":   status,
		"SupplierFilter": supplierID,
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

	invoice, err := h.service.GetAPInvoiceWithDetails(r.Context(), id)
	if err != nil {
		h.logger.Error("get AP invoice", slog.Any("error", err), slog.Int64("id", id))
		h.render(w, r, "pages/ap/ap_invoice_form.html", map[string]any{
			"Errors": formErrors{"general": shared.UserSafeMessage(err)},
		}, http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/ap/ap_invoice_detail.html", map[string]any{
		"Invoice": invoice,
	}, http.StatusOK)
}

// showCreateInvoiceForm shows the create invoice form.
func (h *Handler) showCreateInvoiceForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/ap/ap_invoice_form.html", map[string]any{
		"Errors": formErrors{},
	}, http.StatusOK)
}

// createAPInvoice handles manual invoice creation.
func (h *Handler) createAPInvoice(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	sourceType := r.PostFormValue("source_type")
	sourceIDStr := r.PostFormValue("source_id")
	if sourceType == "" {
		if r.PostFormValue("grn_id") != "" {
			sourceType = "grn"
			sourceIDStr = r.PostFormValue("grn_id")
		} else if r.PostFormValue("po_id") != "" {
			sourceType = "po"
			sourceIDStr = r.PostFormValue("po_id")
		}
	}
	if sourceIDStr == "" {
		h.render(w, r, "pages/ap/ap_invoice_form.html", map[string]any{
			"Errors": formErrors{"general": "Source ID is required"},
		}, http.StatusBadRequest)
		return
	}
	sourceID, err := strconv.ParseInt(sourceIDStr, 10, 64)
	if err != nil {
		h.render(w, r, "pages/ap/ap_invoice_form.html", map[string]any{
			"Errors": formErrors{"general": "Invalid source ID"},
		}, http.StatusBadRequest)
		return
	}

	dueDate, _ := time.Parse("2006-01-02", r.PostFormValue("due_date"))
	if dueDate.IsZero() {
		dueDate = time.Now().AddDate(0, 0, 30)
	}

	sess := shared.SessionFromContext(r.Context())
	userID := getUserID(sess)
	number := r.PostFormValue("number")

	var invoice APInvoice
	switch sourceType {
	case "grn":
		invoice, err = h.service.CreateAPInvoiceFromGRN(r.Context(), CreateAPInvoiceFromGRNInput{
			GRNID:     sourceID,
			DueDate:   dueDate,
			CreatedBy: userID,
			Number:    number,
		})
	case "po":
		invoice, err = h.service.CreateAPInvoiceFromPO(r.Context(), CreateAPInvoiceFromPOInput{
			POID:      sourceID,
			DueDate:   dueDate,
			CreatedBy: userID,
			Number:    number,
		})
	default:
		err = fmt.Errorf("unsupported source type")
	}

	if err != nil {
		h.logger.Error("create AP invoice", slog.Any("error", err))
		h.render(w, r, "pages/ap/ap_invoice_form.html", map[string]any{
			"Errors": formErrors{"general": shared.UserSafeMessage(err)},
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/finance/ap/invoices/"+strconv.FormatInt(invoice.ID, 10), "success", "AP Invoice created")
}

// createInvoiceFromGRN creates invoice from goods receipt.
func (h *Handler) createInvoiceFromGRN(w http.ResponseWriter, r *http.Request) {
	grnIDStr := chi.URLParam(r, "grnID")
	grnID, err := strconv.ParseInt(grnIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid GRN ID", http.StatusBadRequest)
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
	number := r.PostFormValue("number")

	sess := shared.SessionFromContext(r.Context())
	userID := getUserID(sess)

	invoice, err := h.service.CreateAPInvoiceFromGRN(r.Context(), CreateAPInvoiceFromGRNInput{
		GRNID:     grnID,
		DueDate:   dueDate,
		CreatedBy: userID,
		Number:    number,
	})
	if err != nil {
		h.logger.Error("create invoice from GRN", slog.Any("error", err), slog.Int64("grn_id", grnID))
		h.redirectWithFlash(w, r, "/procurement/grns", "error", shared.UserSafeMessage(err))
		return
	}

	h.redirectWithFlash(w, r, "/finance/ap/invoices/"+strconv.FormatInt(invoice.ID, 10), "success", "Invoice created from GRN")
}

// createInvoiceFromPO creates invoice from purchase order.
func (h *Handler) createInvoiceFromPO(w http.ResponseWriter, r *http.Request) {
	poIDStr := chi.URLParam(r, "poID")
	poID, err := strconv.ParseInt(poIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid PO ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	dueDate, _ := time.Parse("2006-01-02", r.PostFormValue("due_date"))
	if dueDate.IsZero() {
		dueDate = time.Now().AddDate(0, 0, 30)
	}
	number := r.PostFormValue("number")

	sess := shared.SessionFromContext(r.Context())
	userID := getUserID(sess)

	invoice, err := h.service.CreateAPInvoiceFromPO(r.Context(), CreateAPInvoiceFromPOInput{
		POID:      poID,
		DueDate:   dueDate,
		CreatedBy: userID,
		Number:    number,
	})
	if err != nil {
		h.logger.Error("create invoice from PO", slog.Any("error", err), slog.Int64("po_id", poID))
		h.redirectWithFlash(w, r, "/procurement/pos", "error", shared.UserSafeMessage(err))
		return
	}

	h.redirectWithFlash(w, r, "/finance/ap/invoices/"+strconv.FormatInt(invoice.ID, 10), "success", "Invoice created from PO")
}

func (h *Handler) postInvoice(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid invoice ID", http.StatusBadRequest)
		return
	}

	sess := shared.SessionFromContext(r.Context())
	userID := getUserID(sess)

	if err := h.service.PostAPInvoice(r.Context(), PostAPInvoiceInput{
		InvoiceID: id,
		PostedBy:  userID,
	}); err != nil {
		h.logger.Error("post AP invoice", slog.Any("error", err), slog.Int64("id", id))
		h.redirectWithFlash(w, r, "/finance/ap/invoices/"+idStr, "error", shared.UserSafeMessage(err))
		return
	}

	h.redirectWithFlash(w, r, "/finance/ap/invoices/"+idStr, "success", "Invoice posted successfully")
}

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

	if err := h.service.VoidAPInvoice(r.Context(), VoidAPInvoiceInput{
		InvoiceID:  id,
		VoidedBy:   userID,
		VoidReason: reason,
	}); err != nil {
		h.logger.Error("void AP invoice", slog.Any("error", err), slog.Int64("id", id))
		h.redirectWithFlash(w, r, "/finance/ap/invoices/"+idStr, "error", shared.UserSafeMessage(err))
		return
	}

	h.redirectWithFlash(w, r, "/finance/ap/invoices/"+idStr, "success", "Invoice voided")
}

func (h *Handler) listPayments(w http.ResponseWriter, r *http.Request) {
	payments, err := h.service.ListAPPayments(r.Context())
	if err != nil {
		h.logger.Error("list AP payments", slog.Any("error", err))
		h.render(w, r, "pages/ap/ap_payment_list.html", map[string]any{
			"Errors": formErrors{"general": shared.UserSafeMessage(err)},
		}, http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/ap/ap_payment_list.html", map[string]any{
		"Payments": payments,
	}, http.StatusOK)
}

func (h *Handler) showCreatePaymentForm(w http.ResponseWriter, r *http.Request) {
	invoices, _ := h.service.ListAPInvoices(r.Context(), ListAPInvoicesRequest{
		Status: APStatusPosted,
		Limit:  100,
	})
	selectedID, _ := strconv.ParseInt(r.URL.Query().Get("ap_invoice_id"), 10, 64)

	h.render(w, r, "pages/ap/ap_payment_form.html", map[string]any{
		"Errors":            formErrors{},
		"Invoices":          invoices,
		"SelectedInvoiceID": selectedID,
	}, http.StatusOK)
}

func (h *Handler) showPaymentDetail(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid payment ID", http.StatusBadRequest)
		return
	}

	payment, err := h.service.GetAPPaymentWithDetails(r.Context(), id)
	if err != nil {
		h.logger.Error("get AP payment", slog.Any("error", err), slog.Int64("id", id))
		h.render(w, r, "pages/ap/ap_payment_list.html", map[string]any{
			"Errors": formErrors{"general": shared.UserSafeMessage(err)},
		}, http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/ap/ap_payment_detail.html", map[string]any{
		"Payment": payment,
	}, http.StatusOK)
}

func (h *Handler) createAPPayment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	supplierID, _ := strconv.ParseInt(r.PostFormValue("supplier_id"), 10, 64)
	amount, _ := strconv.ParseFloat(r.PostFormValue("amount"), 64)
	paidAt, _ := time.Parse("2006-01-02", r.PostFormValue("paid_at"))
	if paidAt.IsZero() {
		paidAt = time.Now()
	}

	sess := shared.SessionFromContext(r.Context())
	userID := getUserID(sess)

	var allocations []PaymentAllocationInput
	invoiceIDs := r.PostForm["ap_invoice_id"]
	allocationAmounts := r.PostForm["allocation_amount"]
	for i := 0; i < len(invoiceIDs) && i < len(allocationAmounts); i++ {
		invoiceID, err := strconv.ParseInt(invoiceIDs[i], 10, 64)
		if err != nil || invoiceID == 0 {
			continue
		}
		allocAmount, err := strconv.ParseFloat(allocationAmounts[i], 64)
		if err != nil || allocAmount <= 0 {
			continue
		}
		allocations = append(allocations, PaymentAllocationInput{
			APInvoiceID: invoiceID,
			Amount:      allocAmount,
		})
	}

	payment, err := h.service.RegisterAPPayment(r.Context(), CreateAPPaymentInput{
		SupplierID:  supplierID,
		Amount:      amount,
		PaidAt:      paidAt,
		Method:      r.PostFormValue("method"),
		Note:        r.PostFormValue("note"),
		CreatedBy:   userID,
		Allocations: allocations,
	})
	if err != nil {
		var ledgerErr *LedgerPostError
		if errors.As(err, &ledgerErr) && payment.ID != 0 {
			message := ledgerErr.Error()
			if ledgerErr.Retryable {
				message = message + ". Retry posting after updating ledger period/mapping."
			}
			h.redirectWithFlash(w, r, "/finance/ap/payments/"+strconv.FormatInt(payment.ID, 10), "warning", message)
			return
		}
		h.logger.Error("create AP payment", slog.Any("error", err))
		invoices, _ := h.service.ListAPInvoices(r.Context(), ListAPInvoicesRequest{
			Status: APStatusPosted,
			Limit:  100,
		})
		h.render(w, r, "pages/ap/ap_payment_form.html", map[string]any{
			"Errors":   formErrors{"general": shared.UserSafeMessage(err)},
			"Invoices": invoices,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/finance/ap/payments/"+strconv.FormatInt(payment.ID, 10), "success", "Payment recorded")
}

func (h *Handler) showAPAgingReport(w http.ResponseWriter, r *http.Request) {
	aging, err := h.service.CalculateAPAging(r.Context(), time.Now())
	if err != nil {
		h.logger.Error("calculate AP aging", slog.Any("error", err))
		h.render(w, r, "pages/ap/ap_aging_report.html", map[string]any{
			"Errors": formErrors{"general": shared.UserSafeMessage(err)},
		}, http.StatusInternalServerError)
		return
	}

	total := aging.Current + aging.Bucket30 + aging.Bucket60 + aging.Bucket90 + aging.Bucket120

	h.render(w, r, "pages/ap/ap_aging_report.html", map[string]any{
		"Aging": aging,
		"Total": total,
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
		Title:       "Accounts Payable",
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
	if sess == nil || sess.User() == "" {
		return 0
	}
	id, _ := strconv.ParseInt(sess.User(), 10, 64)
	return id
}
