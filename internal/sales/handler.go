package sales

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

// Handler manages sales endpoints.
type Handler struct {
	logger    *slog.Logger
	service   *Service
	templates *view.Engine
	csrf      *shared.CSRFManager
	sessions  *shared.SessionManager
	rbac      rbac.Middleware
}

// NewHandler builds Handler instance.
func NewHandler(
	logger *slog.Logger,
	service *Service,
	templates *view.Engine,
	csrf *shared.CSRFManager,
	sessions *shared.SessionManager,
	rbac rbac.Middleware,
) *Handler {
	return &Handler{
		logger:    logger,
		service:   service,
		templates: templates,
		csrf:      csrf,
		sessions:  sessions,
		rbac:      rbac,
	}
}

// MountRoutes registers sales routes.
func (h *Handler) MountRoutes(r chi.Router) {
	// Customer routes
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("sales.customer.view"))
		r.Get("/customers", h.listCustomers)
		r.Get("/customers/{id}", h.showCustomer)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("sales.customer.create"))
		r.Get("/customers/new", h.showCustomerForm)
		r.Post("/customers", h.createCustomer)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("sales.customer.edit"))
		r.Get("/customers/{id}/edit", h.showEditCustomerForm)
		r.Post("/customers/{id}/edit", h.updateCustomer)
	})

	// Quotation routes
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("sales.quotation.view"))
		r.Get("/quotations", h.listQuotations)
		r.Get("/quotations/{id}", h.showQuotation)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("sales.quotation.create"))
		r.Get("/quotations/new", h.showQuotationForm)
		r.Post("/quotations", h.createQuotation)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("sales.quotation.edit"))
		r.Get("/quotations/{id}/edit", h.showEditQuotationForm)
		r.Post("/quotations/{id}/edit", h.updateQuotation)
		r.Post("/quotations/{id}/submit", h.submitQuotation)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("sales.quotation.approve"))
		r.Post("/quotations/{id}/approve", h.approveQuotation)
		r.Post("/quotations/{id}/reject", h.rejectQuotation)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("sales.order.create"))
		r.Post("/quotations/{id}/convert", h.convertQuotationToSO)
	})

	// Sales Order routes
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("sales.order.view"))
		r.Get("/orders", h.listSalesOrders)
		r.Get("/orders/{id}", h.showSalesOrder)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("sales.order.create"))
		r.Get("/orders/new", h.showSalesOrderForm)
		r.Post("/orders", h.createSalesOrder)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("sales.order.edit"))
		r.Get("/orders/{id}/edit", h.showEditSalesOrderForm)
		r.Post("/orders/{id}/edit", h.updateSalesOrder)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("sales.order.confirm"))
		r.Post("/orders/{id}/confirm", h.confirmSalesOrder)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("sales.order.cancel"))
		r.Post("/orders/{id}/cancel", h.cancelSalesOrder)
	})
}

type formErrors map[string]string

// ============================================================================
// CUSTOMER HANDLERS
// ============================================================================

func (h *Handler) listCustomers(w http.ResponseWriter, r *http.Request) {
	companyID := h.getCurrentCompanyID(r)

	// Parse filters
	var isActive *bool
	if r.URL.Query().Get("is_active") != "" {
		val := r.URL.Query().Get("is_active") == "true"
		isActive = &val
	}

	search := r.URL.Query().Get("search")
	var searchPtr *string
	if search != "" {
		searchPtr = &search
	}

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	customers, total, err := h.service.ListCustomers(r.Context(), ListCustomersRequest{
		CompanyID: companyID,
		IsActive:  isActive,
		Search:    searchPtr,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		h.logger.Error("list customers failed", "error", err)
		http.Error(w, "Failed to load customers", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/sales/customers_list.html", map[string]any{
		"Customers": customers,
		"Total":     total,
		"Limit":     limit,
		"Offset":    offset,
		"Filters": map[string]any{
			"IsActive": isActive,
			"Search":   searchPtr,
		},
	}, http.StatusOK)
}

func (h *Handler) showCustomer(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid customer ID", http.StatusBadRequest)
		return
	}

	customer, err := h.service.GetCustomer(r.Context(), id)
	if err != nil {
		h.logger.Error("get customer failed", "error", err, "id", id)
		http.Error(w, "Customer not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/sales/customer_detail.html", map[string]any{
		"Customer": customer,
	}, http.StatusOK)
}

func (h *Handler) showCustomerForm(w http.ResponseWriter, r *http.Request) {
	companyID := h.getCurrentCompanyID(r)

	// Generate customer code
	code, err := h.service.GenerateCustomerCode(r.Context(), companyID)
	if err != nil {
		h.logger.Error("generate customer code failed", "error", err)
		code = ""
	}

	h.render(w, r, "pages/sales/customer_form.html", map[string]any{
		"Errors":        formErrors{},
		"GeneratedCode": code,
		"Customer":      nil,
	}, http.StatusOK)
}

func (h *Handler) createCustomer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	companyID := h.getCurrentCompanyID(r)
	userID := h.getCurrentUserID(r)

	// Parse form data
	creditLimit, _ := strconv.ParseFloat(r.PostFormValue("credit_limit"), 64)
	paymentTerms, _ := strconv.Atoi(r.PostFormValue("payment_terms_days"))

	req := CreateCustomerRequest{
		Code:             r.PostFormValue("code"),
		Name:             r.PostFormValue("name"),
		CompanyID:        companyID,
		CreditLimit:      creditLimit,
		PaymentTermsDays: paymentTerms,
		Country:          r.PostFormValue("country"),
	}

	// Optional fields
	if email := r.PostFormValue("email"); email != "" {
		req.Email = &email
	}
	if phone := r.PostFormValue("phone"); phone != "" {
		req.Phone = &phone
	}
	if taxID := r.PostFormValue("tax_id"); taxID != "" {
		req.TaxID = &taxID
	}
	if addr1 := r.PostFormValue("address_line1"); addr1 != "" {
		req.AddressLine1 = &addr1
	}
	if addr2 := r.PostFormValue("address_line2"); addr2 != "" {
		req.AddressLine2 = &addr2
	}
	if city := r.PostFormValue("city"); city != "" {
		req.City = &city
	}
	if state := r.PostFormValue("state"); state != "" {
		req.State = &state
	}
	if postal := r.PostFormValue("postal_code"); postal != "" {
		req.PostalCode = &postal
	}
	if notes := r.PostFormValue("notes"); notes != "" {
		req.Notes = &notes
	}

	customer, err := h.service.CreateCustomer(r.Context(), req, userID)
	if err != nil {
		h.logger.Error("create customer failed", "error", err)
		h.render(w, r, "pages/sales/customer_form.html", map[string]any{
			"Errors":        formErrors{"general": err.Error()},
			"GeneratedCode": req.Code,
			"Customer":      nil,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/sales/customers/"+strconv.FormatInt(customer.ID, 10), "success", "Customer created successfully")
}

func (h *Handler) showEditCustomerForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid customer ID", http.StatusBadRequest)
		return
	}

	customer, err := h.service.GetCustomer(r.Context(), id)
	if err != nil {
		h.logger.Error("get customer failed", "error", err, "id", id)
		http.Error(w, "Customer not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/sales/customer_form.html", map[string]any{
		"Errors":   formErrors{},
		"Customer": customer,
	}, http.StatusOK)
}

func (h *Handler) updateCustomer(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid customer ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	req := UpdateCustomerRequest{}

	// Parse optional fields
	if name := r.PostFormValue("name"); name != "" {
		req.Name = &name
	}
	if email := r.PostFormValue("email"); email != "" {
		req.Email = &email
	}
	if phone := r.PostFormValue("phone"); phone != "" {
		req.Phone = &phone
	}
	if taxID := r.PostFormValue("tax_id"); taxID != "" {
		req.TaxID = &taxID
	}
	if creditLimit := r.PostFormValue("credit_limit"); creditLimit != "" {
		if val, err := strconv.ParseFloat(creditLimit, 64); err == nil {
			req.CreditLimit = &val
		}
	}
	if paymentTerms := r.PostFormValue("payment_terms_days"); paymentTerms != "" {
		if val, err := strconv.Atoi(paymentTerms); err == nil {
			req.PaymentTermsDays = &val
		}
	}
	if addr1 := r.PostFormValue("address_line1"); addr1 != "" {
		req.AddressLine1 = &addr1
	}
	if addr2 := r.PostFormValue("address_line2"); addr2 != "" {
		req.AddressLine2 = &addr2
	}
	if city := r.PostFormValue("city"); city != "" {
		req.City = &city
	}
	if state := r.PostFormValue("state"); state != "" {
		req.State = &state
	}
	if postal := r.PostFormValue("postal_code"); postal != "" {
		req.PostalCode = &postal
	}
	if country := r.PostFormValue("country"); country != "" {
		req.Country = &country
	}
	if notes := r.PostFormValue("notes"); notes != "" {
		req.Notes = &notes
	}
	if isActive := r.PostFormValue("is_active"); isActive != "" {
		val := isActive == "true"
		req.IsActive = &val
	}

	customer, err := h.service.UpdateCustomer(r.Context(), id, req)
	if err != nil {
		h.logger.Error("update customer failed", "error", err, "id", id)
		h.render(w, r, "pages/sales/customer_form.html", map[string]any{
			"Errors":   formErrors{"general": err.Error()},
			"Customer": customer,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/sales/customers/"+strconv.FormatInt(customer.ID, 10), "success", "Customer updated successfully")
}

// ============================================================================
// QUOTATION HANDLERS
// ============================================================================

func (h *Handler) listQuotations(w http.ResponseWriter, r *http.Request) {
	companyID := h.getCurrentCompanyID(r)

	// Parse filters
	var customerID *int64
	if cid := r.URL.Query().Get("customer_id"); cid != "" {
		if parsed, err := strconv.ParseInt(cid, 10, 64); err == nil {
			customerID = &parsed
		}
	}

	var status *QuotationStatus
	if s := r.URL.Query().Get("status"); s != "" {
		st := QuotationStatus(s)
		status = &st
	}

	var dateFrom, dateTo *time.Time
	if df := r.URL.Query().Get("date_from"); df != "" {
		if parsed, err := time.Parse("2006-01-02", df); err == nil {
			dateFrom = &parsed
		}
	}
	if dt := r.URL.Query().Get("date_to"); dt != "" {
		if parsed, err := time.Parse("2006-01-02", dt); err == nil {
			dateTo = &parsed
		}
	}

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	quotations, total, err := h.service.ListQuotations(r.Context(), ListQuotationsRequest{
		CompanyID:  companyID,
		CustomerID: customerID,
		Status:     status,
		DateFrom:   dateFrom,
		DateTo:     dateTo,
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		h.logger.Error("list quotations failed", "error", err)
		http.Error(w, "Failed to load quotations", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/sales/quotations_list.html", map[string]any{
		"Quotations": quotations,
		"Total":      total,
		"Limit":      limit,
		"Offset":     offset,
		"Filters": map[string]any{
			"CustomerID": customerID,
			"Status":     status,
			"DateFrom":   dateFrom,
			"DateTo":     dateTo,
		},
	}, http.StatusOK)
}

func (h *Handler) showQuotation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid quotation ID", http.StatusBadRequest)
		return
	}

	quotation, err := h.service.GetQuotation(r.Context(), id)
	if err != nil {
		h.logger.Error("get quotation failed", "error", err, "id", id)
		http.Error(w, "Quotation not found", http.StatusNotFound)
		return
	}

	// Get customer details
	customer, err := h.service.GetCustomer(r.Context(), quotation.CustomerID)
	if err != nil {
		h.logger.Error("get customer failed", "error", err, "customer_id", quotation.CustomerID)
	}

	h.render(w, r, "pages/sales/quotation_detail.html", map[string]any{
		"Quotation": quotation,
		"Customer":  customer,
	}, http.StatusOK)
}

func (h *Handler) showQuotationForm(w http.ResponseWriter, r *http.Request) {
	companyID := h.getCurrentCompanyID(r)

	// Get customers for dropdown
	customers, _, err := h.service.ListCustomers(r.Context(), ListCustomersRequest{
		CompanyID: companyID,
		Limit:     1000,
		Offset:    0,
	})
	if err != nil {
		h.logger.Error("list customers failed", "error", err)
		customers = []Customer{}
	}

	h.render(w, r, "pages/sales/quotation_form.html", map[string]any{
		"Errors":    formErrors{},
		"Quotation": nil,
		"Customers": customers,
	}, http.StatusOK)
}

func (h *Handler) createQuotation(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	companyID := h.getCurrentCompanyID(r)
	userID := h.getCurrentUserID(r)

	// Parse dates
	quoteDate, err := time.Parse("2006-01-02", r.PostFormValue("quote_date"))
	if err != nil {
		h.renderQuotationFormError(w, r, "Invalid quote date", nil)
		return
	}

	validUntil, err := time.Parse("2006-01-02", r.PostFormValue("valid_until"))
	if err != nil {
		h.renderQuotationFormError(w, r, "Invalid valid until date", nil)
		return
	}

	customerID, err := strconv.ParseInt(r.PostFormValue("customer_id"), 10, 64)
	if err != nil {
		h.renderQuotationFormError(w, r, "Invalid customer", nil)
		return
	}

	// Parse line items
	lines, err := h.parseQuotationLines(r)
	if err != nil {
		h.renderQuotationFormError(w, r, err.Error(), nil)
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

	quotation, err := h.service.CreateQuotation(r.Context(), req, userID)
	if err != nil {
		h.logger.Error("create quotation failed", "error", err)
		h.renderQuotationFormError(w, r, err.Error(), nil)
		return
	}

	h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(quotation.ID, 10), "success", "Quotation created successfully")
}

func (h *Handler) showEditQuotationForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid quotation ID", http.StatusBadRequest)
		return
	}

	quotation, err := h.service.GetQuotation(r.Context(), id)
	if err != nil {
		h.logger.Error("get quotation failed", "error", err, "id", id)
		http.Error(w, "Quotation not found", http.StatusNotFound)
		return
	}

	companyID := h.getCurrentCompanyID(r)
	customers, _, err := h.service.ListCustomers(r.Context(), ListCustomersRequest{
		CompanyID: companyID,
		Limit:     1000,
		Offset:    0,
	})
	if err != nil {
		h.logger.Error("list customers failed", "error", err)
		customers = []Customer{}
	}

	h.render(w, r, "pages/sales/quotation_form.html", map[string]any{
		"Errors":    formErrors{},
		"Quotation": quotation,
		"Customers": customers,
	}, http.StatusOK)
}

func (h *Handler) updateQuotation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid quotation ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	req := UpdateQuotationRequest{}

	// Parse optional dates
	if qd := r.PostFormValue("quote_date"); qd != "" {
		if parsed, err := time.Parse("2006-01-02", qd); err == nil {
			req.QuoteDate = &parsed
		}
	}
	if vu := r.PostFormValue("valid_until"); vu != "" {
		if parsed, err := time.Parse("2006-01-02", vu); err == nil {
			req.ValidUntil = &parsed
		}
	}
	if notes := r.PostFormValue("notes"); notes != "" {
		req.Notes = &notes
	}

	// Parse line items if provided
	if r.PostForm["product_id"] != nil && len(r.PostForm["product_id"]) > 0 {
		lines, err := h.parseQuotationLines(r)
		if err != nil {
			h.renderQuotationFormError(w, r, err.Error(), nil)
			return
		}
		req.Lines = &lines
	}

	quotation, err := h.service.UpdateQuotation(r.Context(), id, req)
	if err != nil {
		h.logger.Error("update quotation failed", "error", err, "id", id)
		h.renderQuotationFormError(w, r, err.Error(), quotation)
		return
	}

	h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(quotation.ID, 10), "success", "Quotation updated successfully")
}

func (h *Handler) submitQuotation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid quotation ID", http.StatusBadRequest)
		return
	}

	userID := h.getCurrentUserID(r)

	quotation, err := h.service.SubmitQuotation(r.Context(), id, userID)
	if err != nil {
		h.logger.Error("submit quotation failed", "error", err, "id", id)
		h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(id, 10), "error", err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(quotation.ID, 10), "success", "Quotation submitted for approval")
}

func (h *Handler) approveQuotation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid quotation ID", http.StatusBadRequest)
		return
	}

	userID := h.getCurrentUserID(r)

	quotation, err := h.service.ApproveQuotation(r.Context(), id, userID)
	if err != nil {
		h.logger.Error("approve quotation failed", "error", err, "id", id)
		h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(id, 10), "error", err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(quotation.ID, 10), "success", "Quotation approved successfully")
}

func (h *Handler) rejectQuotation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid quotation ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	userID := h.getCurrentUserID(r)
	reason := r.PostFormValue("reason")
	if reason == "" {
		reason = "Rejected"
	}

	quotation, err := h.service.RejectQuotation(r.Context(), id, userID, reason)
	if err != nil {
		h.logger.Error("reject quotation failed", "error", err, "id", id)
		h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(id, 10), "error", err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(quotation.ID, 10), "success", "Quotation rejected")
}

func (h *Handler) convertQuotationToSO(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid quotation ID", http.StatusBadRequest)
		return
	}

	userID := h.getCurrentUserID(r)
	orderDate := time.Now()

	// Parse optional order date from form
	if err := r.ParseForm(); err == nil {
		if od := r.PostFormValue("order_date"); od != "" {
			if parsed, err := time.Parse("2006-01-02", od); err == nil {
				orderDate = parsed
			}
		}
	}

	salesOrder, err := h.service.ConvertQuotationToSalesOrder(r.Context(), id, userID, orderDate)
	if err != nil {
		h.logger.Error("convert quotation to SO failed", "error", err, "id", id)
		h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(id, 10), "error", err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/sales/orders/"+strconv.FormatInt(salesOrder.ID, 10), "success", "Sales order created from quotation")
}

// ============================================================================
// SALES ORDER HANDLERS
// ============================================================================

func (h *Handler) listSalesOrders(w http.ResponseWriter, r *http.Request) {
	companyID := h.getCurrentCompanyID(r)

	// Parse filters
	var customerID *int64
	if cid := r.URL.Query().Get("customer_id"); cid != "" {
		if parsed, err := strconv.ParseInt(cid, 10, 64); err == nil {
			customerID = &parsed
		}
	}

	var status *SalesOrderStatus
	if s := r.URL.Query().Get("status"); s != "" {
		st := SalesOrderStatus(s)
		status = &st
	}

	var dateFrom, dateTo *time.Time
	if df := r.URL.Query().Get("date_from"); df != "" {
		if parsed, err := time.Parse("2006-01-02", df); err == nil {
			dateFrom = &parsed
		}
	}
	if dt := r.URL.Query().Get("date_to"); dt != "" {
		if parsed, err := time.Parse("2006-01-02", dt); err == nil {
			dateTo = &parsed
		}
	}

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	orders, total, err := h.service.ListSalesOrders(r.Context(), ListSalesOrdersRequest{
		CompanyID:  companyID,
		CustomerID: customerID,
		Status:     status,
		DateFrom:   dateFrom,
		DateTo:     dateTo,
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		h.logger.Error("list sales orders failed", "error", err)
		http.Error(w, "Failed to load sales orders", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/sales/orders_list.html", map[string]any{
		"Orders": orders,
		"Total":  total,
		"Limit":  limit,
		"Offset": offset,
		"Filters": map[string]any{
			"CustomerID": customerID,
			"Status":     status,
			"DateFrom":   dateFrom,
			"DateTo":     dateTo,
		},
	}, http.StatusOK)
}

func (h *Handler) showSalesOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid sales order ID", http.StatusBadRequest)
		return
	}

	order, err := h.service.GetSalesOrder(r.Context(), id)
	if err != nil {
		h.logger.Error("get sales order failed", "error", err, "id", id)
		http.Error(w, "Sales order not found", http.StatusNotFound)
		return
	}

	// Get customer details
	customer, err := h.service.GetCustomer(r.Context(), order.CustomerID)
	if err != nil {
		h.logger.Error("get customer failed", "error", err, "customer_id", order.CustomerID)
	}

	// Get quotation if exists
	var quotation *Quotation
	if order.QuotationID != nil {
		quotation, err = h.service.GetQuotation(r.Context(), *order.QuotationID)
		if err != nil {
			h.logger.Error("get quotation failed", "error", err, "quotation_id", *order.QuotationID)
		}
	}

	h.render(w, r, "pages/sales/order_detail.html", map[string]any{
		"Order":     order,
		"Customer":  customer,
		"Quotation": quotation,
	}, http.StatusOK)
}

func (h *Handler) showSalesOrderForm(w http.ResponseWriter, r *http.Request) {
	companyID := h.getCurrentCompanyID(r)

	// Get customers for dropdown
	customers, _, err := h.service.ListCustomers(r.Context(), ListCustomersRequest{
		CompanyID: companyID,
		Limit:     1000,
		Offset:    0,
	})
	if err != nil {
		h.logger.Error("list customers failed", "error", err)
		customers = []Customer{}
	}

	h.render(w, r, "pages/sales/order_form.html", map[string]any{
		"Errors":    formErrors{},
		"Order":     nil,
		"Customers": customers,
	}, http.StatusOK)
}

func (h *Handler) createSalesOrder(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	companyID := h.getCurrentCompanyID(r)
	userID := h.getCurrentUserID(r)

	// Parse dates
	orderDate, err := time.Parse("2006-01-02", r.PostFormValue("order_date"))
	if err != nil {
		h.renderSalesOrderFormError(w, r, "Invalid order date", nil)
		return
	}

	customerID, err := strconv.ParseInt(r.PostFormValue("customer_id"), 10, 64)
	if err != nil {
		h.renderSalesOrderFormError(w, r, "Invalid customer", nil)
		return
	}

	// Parse line items
	lines, err := h.parseSalesOrderLines(r)
	if err != nil {
		h.renderSalesOrderFormError(w, r, err.Error(), nil)
		return
	}

	req := CreateSalesOrderRequest{
		CompanyID:  companyID,
		CustomerID: customerID,
		OrderDate:  orderDate,
		Currency:   r.PostFormValue("currency"),
		Lines:      lines,
	}

	// Optional fields
	if edd := r.PostFormValue("expected_delivery_date"); edd != "" {
		if parsed, err := time.Parse("2006-01-02", edd); err == nil {
			req.ExpectedDeliveryDate = &parsed
		}
	}
	if notes := r.PostFormValue("notes"); notes != "" {
		req.Notes = &notes
	}

	order, err := h.service.CreateSalesOrder(r.Context(), req, userID)
	if err != nil {
		h.logger.Error("create sales order failed", "error", err)
		h.renderSalesOrderFormError(w, r, err.Error(), nil)
		return
	}

	h.redirectWithFlash(w, r, "/sales/orders/"+strconv.FormatInt(order.ID, 10), "success", "Sales order created successfully")
}

func (h *Handler) showEditSalesOrderForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid sales order ID", http.StatusBadRequest)
		return
	}

	order, err := h.service.GetSalesOrder(r.Context(), id)
	if err != nil {
		h.logger.Error("get sales order failed", "error", err, "id", id)
		http.Error(w, "Sales order not found", http.StatusNotFound)
		return
	}

	companyID := h.getCurrentCompanyID(r)
	customers, _, err := h.service.ListCustomers(r.Context(), ListCustomersRequest{
		CompanyID: companyID,
		Limit:     1000,
		Offset:    0,
	})
	if err != nil {
		h.logger.Error("list customers failed", "error", err)
		customers = []Customer{}
	}

	h.render(w, r, "pages/sales/order_form.html", map[string]any{
		"Errors":    formErrors{},
		"Order":     order,
		"Customers": customers,
	}, http.StatusOK)
}

func (h *Handler) updateSalesOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid sales order ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	req := UpdateSalesOrderRequest{}

	// Parse optional dates
	if od := r.PostFormValue("order_date"); od != "" {
		if parsed, err := time.Parse("2006-01-02", od); err == nil {
			req.OrderDate = &parsed
		}
	}
	if edd := r.PostFormValue("expected_delivery_date"); edd != "" {
		if parsed, err := time.Parse("2006-01-02", edd); err == nil {
			req.ExpectedDeliveryDate = &parsed
		}
	}
	if notes := r.PostFormValue("notes"); notes != "" {
		req.Notes = &notes
	}

	// Parse line items if provided
	if r.PostForm["product_id"] != nil && len(r.PostForm["product_id"]) > 0 {
		lines, err := h.parseSalesOrderLines(r)
		if err != nil {
			h.renderSalesOrderFormError(w, r, err.Error(), nil)
			return
		}
		req.Lines = &lines
	}

	order, err := h.service.UpdateSalesOrder(r.Context(), id, req)
	if err != nil {
		h.logger.Error("update sales order failed", "error", err, "id", id)
		h.renderSalesOrderFormError(w, r, err.Error(), order)
		return
	}

	h.redirectWithFlash(w, r, "/sales/orders/"+strconv.FormatInt(order.ID, 10), "success", "Sales order updated successfully")
}

func (h *Handler) confirmSalesOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid sales order ID", http.StatusBadRequest)
		return
	}

	userID := h.getCurrentUserID(r)

	order, err := h.service.ConfirmSalesOrder(r.Context(), id, userID)
	if err != nil {
		h.logger.Error("confirm sales order failed", "error", err, "id", id)
		h.redirectWithFlash(w, r, "/sales/orders/"+strconv.FormatInt(id, 10), "error", err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/sales/orders/"+strconv.FormatInt(order.ID, 10), "success", "Sales order confirmed successfully")
}

func (h *Handler) cancelSalesOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid sales order ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	userID := h.getCurrentUserID(r)
	reason := r.PostFormValue("reason")
	if reason == "" {
		reason = "Cancelled"
	}

	order, err := h.service.CancelSalesOrder(r.Context(), id, userID, reason)
	if err != nil {
		h.logger.Error("cancel sales order failed", "error", err, "id", id)
		h.redirectWithFlash(w, r, "/sales/orders/"+strconv.FormatInt(id, 10), "error", err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/sales/orders/"+strconv.FormatInt(order.ID, 10), "success", "Sales order cancelled")
}

// ============================================================================
// HELPER METHODS
// ============================================================================

func (h *Handler) render(w http.ResponseWriter, r *http.Request, tmpl string, data map[string]any, status int) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)

	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}

	viewData := view.TemplateData{
		Title:       "Sales",
		CSRFToken:   csrfToken,
		Flash:       flash,
		CurrentPath: r.URL.Path,
		Data:        data,
	}

	w.WriteHeader(status)
	if err := h.templates.Render(w, tmpl, viewData); err != nil {
		h.logger.Error("template render failed", "error", err, "template", tmpl)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *Handler) redirectWithFlash(w http.ResponseWriter, r *http.Request, url, flashType, message string) {
	sess := shared.SessionFromContext(r.Context())
	if sess != nil {
		sess.AddFlash(shared.FlashMessage{Kind: flashType, Message: message})
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}

func (h *Handler) getCurrentUserID(r *http.Request) int64 {
	sess := shared.SessionFromContext(r.Context())
	if sess != nil {
		if userIDStr := sess.User(); userIDStr != "" {
			if userID, err := strconv.ParseInt(userIDStr, 10, 64); err == nil {
				return userID
			}
		}
	}
	return 1 // Default user for development
}

func (h *Handler) getCurrentCompanyID(r *http.Request) int64 {
	sess := shared.SessionFromContext(r.Context())
	if sess != nil {
		if companyIDStr := sess.Get("company_id"); companyIDStr != "" {
			if companyID, err := strconv.ParseInt(companyIDStr, 10, 64); err == nil {
				return companyID
			}
		}
	}
	return 1 // Default company for development
}

func (h *Handler) parseQuotationLines(r *http.Request) ([]CreateQuotationLineReq, error) {
	productIDs := r.PostForm["product_id"]
	quantities := r.PostForm["quantity"]
	uoms := r.PostForm["uom"]
	unitPrices := r.PostForm["unit_price"]
	discountPercents := r.PostForm["discount_percent"]
	taxPercents := r.PostForm["tax_percent"]

	if len(productIDs) == 0 {
		return nil, ErrNotFound
	}

	lines := make([]CreateQuotationLineReq, 0, len(productIDs))
	for i := range productIDs {
		productID, err := strconv.ParseInt(productIDs[i], 10, 64)
		if err != nil || productID <= 0 {
			continue
		}

		quantity, err := strconv.ParseFloat(quantities[i], 64)
		if err != nil || quantity <= 0 {
			continue
		}

		unitPrice, err := strconv.ParseFloat(unitPrices[i], 64)
		if err != nil || unitPrice < 0 {
			continue
		}

		line := CreateQuotationLineReq{
			ProductID: productID,
			Quantity:  quantity,
			UOM:       uoms[i],
			UnitPrice: unitPrice,
			LineOrder: i + 1,
		}

		if len(discountPercents) > i {
			if dp, err := strconv.ParseFloat(discountPercents[i], 64); err == nil {
				line.DiscountPercent = dp
			}
		}

		if len(taxPercents) > i {
			if tp, err := strconv.ParseFloat(taxPercents[i], 64); err == nil {
				line.TaxPercent = tp
			}
		}

		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return nil, ErrNotFound
	}

	return lines, nil
}

func (h *Handler) parseSalesOrderLines(r *http.Request) ([]CreateSalesOrderLineReq, error) {
	productIDs := r.PostForm["product_id"]
	quantities := r.PostForm["quantity"]
	uoms := r.PostForm["uom"]
	unitPrices := r.PostForm["unit_price"]
	discountPercents := r.PostForm["discount_percent"]
	taxPercents := r.PostForm["tax_percent"]

	if len(productIDs) == 0 {
		return nil, ErrNotFound
	}

	lines := make([]CreateSalesOrderLineReq, 0, len(productIDs))
	for i := range productIDs {
		productID, err := strconv.ParseInt(productIDs[i], 10, 64)
		if err != nil || productID <= 0 {
			continue
		}

		quantity, err := strconv.ParseFloat(quantities[i], 64)
		if err != nil || quantity <= 0 {
			continue
		}

		unitPrice, err := strconv.ParseFloat(unitPrices[i], 64)
		if err != nil || unitPrice < 0 {
			continue
		}

		line := CreateSalesOrderLineReq{
			ProductID: productID,
			Quantity:  quantity,
			UOM:       uoms[i],
			UnitPrice: unitPrice,
			LineOrder: i + 1,
		}

		if len(discountPercents) > i {
			if dp, err := strconv.ParseFloat(discountPercents[i], 64); err == nil {
				line.DiscountPercent = dp
			}
		}

		if len(taxPercents) > i {
			if tp, err := strconv.ParseFloat(taxPercents[i], 64); err == nil {
				line.TaxPercent = tp
			}
		}

		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return nil, ErrNotFound
	}

	return lines, nil
}

func (h *Handler) renderQuotationFormError(w http.ResponseWriter, r *http.Request, errMsg string, quotation *Quotation) {
	companyID := h.getCurrentCompanyID(r)
	customers, _, _ := h.service.ListCustomers(r.Context(), ListCustomersRequest{
		CompanyID: companyID,
		Limit:     1000,
		Offset:    0,
	})

	h.render(w, r, "pages/sales/quotation_form.html", map[string]any{
		"Errors":    formErrors{"general": errMsg},
		"Quotation": quotation,
		"Customers": customers,
	}, http.StatusBadRequest)
}

func (h *Handler) renderSalesOrderFormError(w http.ResponseWriter, r *http.Request, errMsg string, order *SalesOrder) {
	companyID := h.getCurrentCompanyID(r)
	customers, _, _ := h.service.ListCustomers(r.Context(), ListCustomersRequest{
		CompanyID: companyID,
		Limit:     1000,
		Offset:    0,
	})

	h.render(w, r, "pages/sales/order_form.html", map[string]any{
		"Errors":    formErrors{"general": errMsg},
		"Order":     order,
		"Customers": customers,
	}, http.StatusBadRequest)
}
