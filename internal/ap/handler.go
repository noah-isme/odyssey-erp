package ap

import (
	"context"
	"errors"
	"fmt"
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

// Handler manages AP endpoints.
type Handler struct {
	logger       *slog.Logger
	service      *Service
	exceptions   exceptionWorkbenchService
	templates    *view.Engine
	csrf         *shared.CSRFManager
	sessions     *shared.SessionManager
	rbac         rbac.Middleware
	debitNotePDF DebitNotePDFRenderer
	enqueueJob   func(int64, int64) error
}

// exceptionWorkbenchService is deliberately narrower than ExceptionService:
// the HTTP workbench only needs read, list, and terminal-resolution behavior.
// This keeps the handler testable without coupling it to a concrete database
// implementation.
type exceptionWorkbenchService interface {
	GetExceptionForCompany(context.Context, int64, int64) (APException, error)
	ListExceptionsForCompany(context.Context, int64, string, int64, int64, int, int) ([]APException, error)
	ResolveExceptionForCompany(context.Context, int64, int64, int64, string) error
}

// exceptionWorkbenchCommentService is optional so existing integrations that
// implement the narrower workbench interface continue to compile. The
// concrete ExceptionService implements this richer path.
type exceptionWorkbenchCommentService interface {
	ResolveExceptionForCompanyWithComment(context.Context, int64, int64, int64, string, string) error
}

const exceptionListLimit = 100

var exceptionStatuses = map[string]struct{}{
	"":          {},
	"OPEN":      {},
	"IN_REVIEW": {},
	"RESOLVED":  {},
	"REJECTED":  {},
}

// NewHandler builds Handler instance.
func NewHandler(logger *slog.Logger, service *Service, templates *view.Engine, csrf *shared.CSRFManager, sessions *shared.SessionManager, rbac rbac.Middleware) *Handler {
	var exceptions exceptionWorkbenchService
	if service != nil && service.repo != nil {
		exceptions = NewExceptionService(service.repo)
	}
	return &Handler{logger: logger, service: service, exceptions: exceptions, templates: templates, csrf: csrf, sessions: sessions, rbac: rbac}
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
		r.Get("/debit-notes", h.listDebitNotes)
		r.Get("/debit-notes/{id}", h.showDebitNote)
		r.Get("/debit-notes/{id}/pdf", h.debitNotePDFDownload)
		r.Get("/aging", h.showAPAgingReport)
	})

	// Exception workbench access is separate from ordinary AP access. In a
	// bounded profile the parent router also applies the active company scope;
	// RequireAny preserves that marker and therefore fails closed when scoped
	// permission lookup is unavailable.
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermProcurementP2PExceptionView))
		r.Get("/exceptions", h.listExceptions)
	})

	// Create/Action routes
	r.Group(func(r chi.Router) {
		r.With(h.rbac.RequireAny("finance.ap.create")).Post("/invoices", h.createAPInvoice)
		r.With(h.rbac.RequireAny("finance.ap.create")).Post("/invoices/from-grn/{grnID}", h.createInvoiceFromGRN)
		r.With(h.rbac.RequireAny("finance.ap.create")).Post("/invoices/from-po/{poID}", h.createInvoiceFromPO)
		r.With(h.rbac.RequireAny("finance.ap.post")).Post("/invoices/{id}/post", h.postInvoice)
		r.With(h.rbac.RequireAny("finance.ap.void")).Post("/invoices/{id}/void", h.voidInvoice)
		r.With(h.rbac.RequireAny("finance.ap.payment")).Post("/payments", h.createAPPayment)
		r.With(h.rbac.RequireAny(shared.PermFinanceAPDebitNoteCreate)).Post("/debit-notes/from-return/{returnID}", h.createDebitNoteFromReturn)
		r.With(h.rbac.RequireAny(shared.PermFinanceAPDebitNotePost)).Post("/debit-notes/{id}/post", h.postDebitNote)
		r.With(h.rbac.RequireAny(shared.PermFinanceAPDebitNoteVoid)).Post("/debit-notes/{id}/void", h.voidDebitNote)
		r.With(h.rbac.RequireAny(shared.PermProcurementP2PExceptionResolve)).Post("/exceptions/{id}/resolve", h.resolveException)
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

	if h.enqueueJob != nil {
		if err := h.enqueueJob(invoice.ID, invoice.CreatedBy); err != nil {
			h.logger.Error("failed to enqueue AP invoice processing", slog.Any("error", err))
		}
	}

	h.redirectWithFlash(w, r, "/finance/ap/invoices/"+strconv.FormatInt(invoice.ID, 10), "success", "AP Invoice created")
}

// SetEnqueueJob sets the callback for enqueuing AP jobs.
func (h *Handler) SetEnqueueJob(fn func(int64, int64) error) {
	h.enqueueJob = fn
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

// listExceptions renders the company-scoped AP exception queue. The repository
// applies the tenant boundary in SQL before pagination; the linked-invoice
// check remains as a defense-in-depth guard for legacy records.
// Missing company identity or invoice ownership fails closed.
func (h *Handler) listExceptions(w http.ResponseWriter, r *http.Request) {
	identity, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, "")
		return
	}
	if h.exceptions == nil || h.service == nil {
		h.logger.Error("AP exception workbench is not configured")
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}

	query := r.URL.Query()
	status := strings.ToUpper(strings.TrimSpace(query.Get("status")))
	if _, valid := exceptionStatuses[status]; !valid {
		http.Error(w, "Invalid exception status", http.StatusBadRequest)
		return
	}
	ownerID, err := parseExceptionFilterID(query.Get("owner_id"))
	if err != nil {
		http.Error(w, "Invalid owner ID", http.StatusBadRequest)
		return
	}
	invoiceID, err := parseExceptionFilterID(query.Get("invoice_id"))
	if err != nil {
		http.Error(w, "Invalid invoice ID", http.StatusBadRequest)
		return
	}
	offset, err := parseExceptionOffset(query.Get("offset"))
	if err != nil {
		http.Error(w, "Invalid offset", http.StatusBadRequest)
		return
	}

	exceptions, err := h.exceptions.ListExceptionsForCompany(r.Context(), identity.CompanyID, status, ownerID, invoiceID, exceptionListLimit, offset)
	if err != nil {
		h.logger.Error("list AP exceptions", slog.Any("error", err))
		h.render(w, r, "pages/ap/ap_exception_list.html", map[string]any{
			"Errors": formErrors{"general": shared.UserSafeMessage(err)},
		}, http.StatusInternalServerError)
		return
	}

	scoped := make([]APException, 0, len(exceptions))
	for _, exception := range exceptions {
		invoice, invoiceErr := h.service.GetAPInvoice(r.Context(), exception.APInvoiceID)
		if invoiceErr != nil {
			h.logger.Error("scope AP exception invoice", slog.Any("error", invoiceErr), slog.Int64("exception_id", exception.ID), slog.Int64("invoice_id", exception.APInvoiceID))
			h.render(w, r, "pages/ap/ap_exception_list.html", map[string]any{
				"Errors": formErrors{"general": shared.UserSafeMessage(invoiceErr)},
			}, http.StatusInternalServerError)
			return
		}
		if invoice.CompanyID == nil || *invoice.CompanyID != identity.CompanyID {
			continue
		}
		scoped = append(scoped, exception)
	}

	h.render(w, r, "pages/ap/ap_exception_list.html", map[string]any{
		"Exceptions":    scoped,
		"StatusFilter":  status,
		"OwnerFilter":   ownerID,
		"InvoiceFilter": invoiceID,
		"Offset":        offset,
	}, http.StatusOK)
}

// resolveException performs only terminal transitions exposed by the
// workbench. The service repeats the validation so non-HTTP callers cannot
// update an exception to an arbitrary status.
func (h *Handler) resolveException(w http.ResponseWriter, r *http.Request) {
	id, err := parsePositiveID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid exception ID", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	resolution := strings.ToUpper(strings.TrimSpace(r.PostFormValue("resolution")))
	if resolution != "RESOLVED" && resolution != "REJECTED" {
		http.Error(w, "Invalid exception resolution", http.StatusBadRequest)
		return
	}
	identity, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, "")
		return
	}
	if h.exceptions == nil || h.service == nil {
		h.logger.Error("AP exception workbench is not configured")
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}

	exception, err := h.exceptions.GetExceptionForCompany(r.Context(), identity.CompanyID, id)
	if err != nil {
		h.logger.Error("get AP exception for resolution", slog.Any("error", err), slog.Int64("exception_id", id))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	invoice, err := h.service.GetAPInvoice(r.Context(), exception.APInvoiceID)
	if err != nil {
		h.logger.Error("scope AP exception resolution", slog.Any("error", err), slog.Int64("exception_id", id), slog.Int64("invoice_id", exception.APInvoiceID))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if invoice.CompanyID == nil || *invoice.CompanyID != identity.CompanyID {
		// Do not reveal whether another company's exception exists.
		http.NotFound(w, r)
		return
	}

	var resolveErr error
	if commentService, ok := h.exceptions.(exceptionWorkbenchCommentService); ok {
		resolveErr = commentService.ResolveExceptionForCompanyWithComment(r.Context(), identity.CompanyID, id, identity.UserID, resolution, strings.TrimSpace(r.PostFormValue("comment")))
	} else {
		resolveErr = h.exceptions.ResolveExceptionForCompany(r.Context(), identity.CompanyID, id, identity.UserID, resolution)
	}
	if resolveErr != nil {
		h.logger.Error("resolve AP exception", slog.Any("error", resolveErr), slog.Int64("exception_id", id))
		h.redirectWithFlash(w, r, "/finance/ap/exceptions", "error", shared.UserSafeMessage(resolveErr))
		return
	}
	h.redirectWithFlash(w, r, "/finance/ap/exceptions", "success", "AP exception updated")
}

func parseExceptionFilterID(raw string) (int64, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	return parsePositiveID(raw)
}

func parseExceptionOffset(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || offset < 0 {
		return 0, errors.New("offset must be zero or greater")
	}
	return offset, nil
}

func parsePositiveID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("id must be positive")
	}
	return id, nil
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
	if err := h.templates.RenderStatus(w, template, viewData, status); err != nil {
		h.logger.Error("render template", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
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
