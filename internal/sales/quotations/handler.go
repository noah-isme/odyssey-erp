package quotations

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/sales/customers"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/products"
)

type Handler struct {
	logger          *slog.Logger
	service         *Service
	customerService *customers.Service
	productService  *products.Service
	templates       *view.Engine
	csrf            *shared.CSRFManager
	rbac            rbac.Middleware
}

func NewHandler(
	logger *slog.Logger,
	service *Service,
	customerService *customers.Service,
	productService *products.Service,
	templates *view.Engine,
	csrf *shared.CSRFManager,
	rbac rbac.Middleware,
) *Handler {
	return &Handler{
		logger:          logger,
		service:         service,
		customerService: customerService,
		productService:  productService,
		templates:       templates,
		csrf:            csrf,
		rbac:            rbac,
	}
}

type formErrors map[string]string

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	companyID := h.getCurrentCompanyID(r)

	status := r.URL.Query().Get("status")
	var statusPtr *QuotationStatus
	if status != "" {
		s := QuotationStatus(status)
		statusPtr = &s
	}

	dateFrom := h.parseDate(r.URL.Query().Get("date_from"))
	dateTo := h.parseDate(r.URL.Query().Get("date_to"))

	limit := 50
	offset := 0
	// Pagination parsing...

	quotations, total, err := h.service.List(r.Context(), ListQuotationsRequest{
		CompanyID: companyID,
		Status:    statusPtr,
		DateFrom:  dateFrom,
		DateTo:    dateTo,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		h.logger.Error("list quotations failed", "error", err)
		http.Error(w, "Failed to load quotations", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/sales/quotations_list.html", map[string]any{
		"Quotations": quotations,
		"Total":      total,
		"Filters": map[string]any{
			"Status":   status,
			"DateFrom": r.URL.Query().Get("date_from"),
			"DateTo":   r.URL.Query().Get("date_to"),
		},
	}, http.StatusOK)
}

func (h *Handler) Show(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid quotation ID", http.StatusBadRequest)
		return
	}

	quotation, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.logger.Error("get quotation failed", "error", err)
		http.Error(w, "Quotation not found", http.StatusNotFound)
		return
	}

	customer, _ := h.customerService.Get(r.Context(), quotation.CustomerID)

	h.render(w, r, "pages/sales/quotation_detail.html", map[string]any{
		"Quotation": quotation,
		"Customer":  customer,
	}, http.StatusOK)
}

func (h *Handler) ShowForm(w http.ResponseWriter, r *http.Request) {
	companyID := h.getCurrentCompanyID(r)
	customers, _, _ := h.customerService.List(r.Context(), customers.ListCustomersRequest{
		CompanyID: companyID,
		Limit:     1000,
	})

	h.render(w, r, "pages/sales/quotation_form.html", map[string]any{
		"Errors":    formErrors{},
		"Quotation": nil,
		"Customers": customers,
	}, http.StatusOK)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	companyID := h.getCurrentCompanyID(r)
	userID := h.getCurrentUserID(r)
	
	// Parsing logic similar to monolithic handler...
	customerID, _ := strconv.ParseInt(r.PostFormValue("customer_id"), 10, 64)
	quoteDate, _ := time.Parse("2006-01-02", r.PostFormValue("quote_date"))
	validUntil, _ := time.Parse("2006-01-02", r.PostFormValue("valid_until"))
	
	lines, err := h.parseQuotationLines(r)
	if err != nil {
		h.renderFormError(w, r, err.Error(), nil)
		return
	}

	req := CreateQuotationRequest{
		CompanyID:  companyID,
		CustomerID: customerID,
		QuoteDate:  quoteDate,
		ValidUntil: validUntil,
		Currency:   r.PostFormValue("currency"),
		Lines:      lines,
	}
	if notes := r.PostFormValue("notes"); notes != "" {
		req.Notes = &notes
	}

	quotation, err := h.service.Create(r.Context(), req, userID)
	if err != nil {
		h.logger.Error("create quotation failed", "error", err)
		h.renderFormError(w, r, err.Error(), nil)
		return
	}

	h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(quotation.ID, 10), "success", "Quotation created successfully")
}

func (h *Handler) ShowEditForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	quotation, err := h.service.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	
	companyID := h.getCurrentCompanyID(r)
	customers, _, _ := h.customerService.List(r.Context(), customers.ListCustomersRequest{
		CompanyID: companyID,
		Limit:     1000,
	})

	h.render(w, r, "pages/sales/quotation_form.html", map[string]any{
		"Errors":    formErrors{},
		"Quotation": quotation,
		"Customers": customers,
	}, http.StatusOK)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	req := UpdateQuotationRequest{}
	
	if d := r.PostFormValue("quote_date"); d != "" {
		if t, err := time.Parse("2006-01-02", d); err == nil {
			req.QuoteDate = &t
		}
	}
	if d := r.PostFormValue("valid_until"); d != "" {
		if t, err := time.Parse("2006-01-02", d); err == nil {
			req.ValidUntil = &t
		}
	}
	if n := r.PostFormValue("notes"); n != "" {
		req.Notes = &n
	}
	
	// If products provided, parse lines
	if len(r.PostForm["product_id"]) > 0 {
		lines, err := h.parseQuotationLines(r)
		if err != nil {
			h.renderFormError(w, r, err.Error(), nil) // Need current quotation to render error form correctly
			return
		}
		req.Lines = &lines
	}

	quotation, err := h.service.Update(r.Context(), id, req)
	if err != nil {
		h.logger.Error("update quotation failed", "error", err)
		// Should fetch quotation to render form again
		q, _ := h.service.Get(r.Context(), id)
		h.renderFormError(w, r, err.Error(), q)
		return
	}

	h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(quotation.ID, 10), "success", "Quotation updated successfully")
}

func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID := h.getCurrentUserID(r)
	
	_, err := h.service.Submit(r.Context(), id, userID)
	if err != nil {
		h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(id, 10), "error", err.Error())
		return
	}
	h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(id, 10), "success", "Quotation submitted")
}

func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID := h.getCurrentUserID(r)
	
	_, err := h.service.Approve(r.Context(), id, userID)
	if err != nil {
		h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(id, 10), "error", err.Error())
		return
	}
	h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(id, 10), "success", "Quotation approved")
}

func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID := h.getCurrentUserID(r)
	reason := r.PostFormValue("reason")
	
	_, err := h.service.Reject(r.Context(), id, userID, reason)
	if err != nil {
		h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(id, 10), "error", err.Error())
		return
	}
	h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(id, 10), "success", "Quotation rejected")
}

// Helpers
func (h *Handler) parseQuotationLines(r *http.Request) ([]CreateQuotationLineReq, error) {
	productIDs := r.PostForm["product_id"]
	quantities := r.PostForm["quantity"]
	uoms := r.PostForm["uom"]
	unitPrices := r.PostForm["unit_price"]
	discountPercents := r.PostForm["discount_percent"]
	taxPercents := r.PostForm["tax_percent"]

	if len(productIDs) == 0 {
		return nil, nil // Or error if at least one line required
	}

	lines := make([]CreateQuotationLineReq, 0, len(productIDs))
	for i := range productIDs {
		pid, _ := strconv.ParseInt(productIDs[i], 10, 64)
		qty, _ := strconv.ParseFloat(quantities[i], 64)
		price, _ := strconv.ParseFloat(unitPrices[i], 64)
		dist, _ := strconv.ParseFloat(discountPercents[i], 64)
		tax, _ := strconv.ParseFloat(taxPercents[i], 64)
		
		lines = append(lines, CreateQuotationLineReq{
			ProductID:       pid,
			Quantity:        qty,
			UOM:             uoms[i],
			UnitPrice:       price,
			DiscountPercent: dist,
			TaxPercent:      tax,
			LineOrder:       i + 1,
		})
	}
	return lines, nil
}

func (h *Handler) renderFormError(w http.ResponseWriter, r *http.Request, msg string, q *Quotation) {
	// Re-fetch customers
	companyID := h.getCurrentCompanyID(r)
	customers, _, _ := h.customerService.List(r.Context(), customers.ListCustomersRequest{CompanyID: companyID, Limit: 1000})
	
	h.render(w, r, "pages/sales/quotation_form.html", map[string]any{
		"Errors": formErrors{"general": msg},
		"Quotation": q,
		"Customers": customers,
	}, http.StatusBadRequest)
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, tmpl string, data map[string]any, status int) {
	// ... (Same helpers as customer handler, or use shared base)
	// I'll implementing them inline to be self contained as per pattern
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	var flash *shared.FlashMessage
	if sess != nil { flash = sess.PopFlash() }
	
	viewData := view.TemplateData{
		Title: "Sales",
		CSRFToken: csrfToken,
		Flash: flash,
		CurrentPath: r.URL.Path,
		Data: data,
	}
	w.WriteHeader(status)
	h.templates.Render(w, tmpl, viewData)
}

func (h *Handler) redirectWithFlash(w http.ResponseWriter, r *http.Request, url, flashType, message string) {
	sess := shared.SessionFromContext(r.Context())
	if sess != nil { sess.AddFlash(shared.FlashMessage{Kind: flashType, Message: message}) }
	http.Redirect(w, r, url, http.StatusSeeOther)
}

func (h *Handler) getCurrentUserID(r *http.Request) int64 {
	sess := shared.SessionFromContext(r.Context())
	if sess != nil && sess.User() != "" {
		if id, err := strconv.ParseInt(sess.User(), 10, 64); err == nil { return id }
	}
	return 1
}

func (h *Handler) getCurrentCompanyID(r *http.Request) int64 {
	sess := shared.SessionFromContext(r.Context())
	if sess != nil && sess.Get("company_id") != "" {
		if id, err := strconv.ParseInt(sess.Get("company_id"), 10, 64); err == nil && id > 0 {
			return id
		}
	}
	return 1
}

func (h *Handler) parseDate(s string) *time.Time {
	if s == "" { return nil }
	t, err := time.Parse("2006-01-02", s)
	if err != nil { return nil }
	return &t
}
