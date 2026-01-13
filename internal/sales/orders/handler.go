package orders

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/products"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/sales/customers"
	"github.com/odyssey-erp/odyssey-erp/internal/sales/quotations"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type Handler struct {
	logger           *slog.Logger
	service          *Service
	customerService  *customers.Service
	quotationService *quotations.Service
	productService   *products.Service
	templates        *view.Engine
	csrf             *shared.CSRFManager
	rbac             rbac.Middleware
}

func NewHandler(
	logger *slog.Logger,
	service *Service,
	customerService *customers.Service,
	quotationService *quotations.Service,
	productService *products.Service,
	templates *view.Engine,
	csrf *shared.CSRFManager,
	rbac rbac.Middleware,
) *Handler {
	return &Handler{
		logger:           logger,
		service:          service,
		customerService:  customerService,
		quotationService: quotationService,
		productService:   productService,
		templates:        templates,
		csrf:             csrf,
		rbac:             rbac,
	}
}

type formErrors map[string]string

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	companyID := h.getCurrentCompanyID(r)

	status := r.URL.Query().Get("status")
	var statusPtr *SalesOrderStatus
	if status != "" {
		s := SalesOrderStatus(status)
		statusPtr = &s
	}

	dateFrom := h.parseDate(r.URL.Query().Get("date_from"))
	dateTo := h.parseDate(r.URL.Query().Get("date_to"))

	limit := 50
	offset := 0

	orders, total, err := h.service.List(r.Context(), ListSalesOrdersRequest{
		CompanyID: companyID,
		Status:    statusPtr,
		DateFrom:  dateFrom,
		DateTo:    dateTo,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		h.logger.Error("list orders failed", "error", err)
		http.Error(w, "Failed to load orders", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/sales/orders_list.html", map[string]any{
		"Orders": orders,
		"Total":  total,
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
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	order, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.logger.Error("get order failed", "error", err)
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	customer, _ := h.customerService.Get(r.Context(), order.CustomerID)

	var quotation *quotations.Quotation
	if order.QuotationID != nil {
		quotation, _ = h.quotationService.Get(r.Context(), *order.QuotationID)
	}

	h.render(w, r, "pages/sales/order_detail.html", map[string]any{
		"Order":     order,
		"Customer":  customer,
		"Quotation": quotation,
	}, http.StatusOK)
}

func (h *Handler) ShowForm(w http.ResponseWriter, r *http.Request) {
	companyID := h.getCurrentCompanyID(r)
	customers, _, _ := h.customerService.List(r.Context(), customers.ListCustomersRequest{
		CompanyID: companyID,
		Limit:     1000,
	})

	h.render(w, r, "pages/sales/order_form.html", map[string]any{
		"Errors":    formErrors{},
		"Order":     nil,
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

	customerID, _ := strconv.ParseInt(r.PostFormValue("customer_id"), 10, 64)
	orderDate, _ := time.Parse("2006-01-02", r.PostFormValue("order_date"))

	var quotationID *int64
	if qID := r.PostFormValue("quotation_id"); qID != "" {
		if id, err := strconv.ParseInt(qID, 10, 64); err == nil {
			quotationID = &id
		}
	}

	lines, err := h.parseSalesOrderLines(r)
	if err != nil {
		h.renderFormError(w, r, shared.UserSafeMessage(err), nil)
		return
	}

	req := CreateSalesOrderRequest{
		CompanyID:   companyID,
		CustomerID:  customerID,
		QuotationID: quotationID,
		OrderDate:   orderDate,
		Currency:    r.PostFormValue("currency"),
		Lines:       lines,
	}
	if d := r.PostFormValue("expected_delivery_date"); d != "" {
		if t, err := time.Parse("2006-01-02", d); err == nil {
			req.ExpectedDeliveryDate = &t
		}
	}
	if n := r.PostFormValue("notes"); n != "" {
		req.Notes = &n
	}

	order, err := h.service.Create(r.Context(), req, userID)
	if err != nil {
		h.logger.Error("create order failed", "error", err)
		h.renderFormError(w, r, shared.UserSafeMessage(err), nil)
		return
	}

	h.redirectWithFlash(w, r, "/sales/orders/"+strconv.FormatInt(order.ID, 10), "success", "Sales order created successfully")
}

func (h *Handler) ShowEditForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	order, err := h.service.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	companyID := h.getCurrentCompanyID(r)
	customers, _, _ := h.customerService.List(r.Context(), customers.ListCustomersRequest{
		CompanyID: companyID,
		Limit:     1000,
	})

	h.render(w, r, "pages/sales/order_form.html", map[string]any{
		"Errors":    formErrors{},
		"Order":     order,
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

	req := UpdateSalesOrderRequest{}

	if d := r.PostFormValue("order_date"); d != "" {
		if t, err := time.Parse("2006-01-02", d); err == nil {
			req.OrderDate = &t
		}
	}
	if d := r.PostFormValue("expected_delivery_date"); d != "" {
		if t, err := time.Parse("2006-01-02", d); err == nil {
			req.ExpectedDeliveryDate = &t
		}
	}
	if n := r.PostFormValue("notes"); n != "" {
		req.Notes = &n
	}

	if len(r.PostForm["product_id"]) > 0 {
		lines, err := h.parseSalesOrderLines(r)
		if err != nil {
			h.renderFormError(w, r, shared.UserSafeMessage(err), nil)
			return
		}
		req.Lines = &lines
	}

	order, err := h.service.Update(r.Context(), id, req)
	if err != nil {
		h.logger.Error("update order failed", "error", err)
		o, _ := h.service.Get(r.Context(), id)
		h.renderFormError(w, r, shared.UserSafeMessage(err), o)
		return
	}

	h.redirectWithFlash(w, r, "/sales/orders/"+strconv.FormatInt(order.ID, 10), "success", "Sales order updated successfully")
}

func (h *Handler) ConvertFromQuotation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid quotation ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	quotation, err := h.quotationService.Get(r.Context(), id)
	if err != nil {
		h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(id, 10), "error", "Quotation not found")
		return
	}
	if quotation.Status != quotations.QuotationStatusApproved {
		h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(id, 10), "error", "Quotation must be approved before conversion")
		return
	}
	if len(quotation.Lines) == 0 {
		h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(id, 10), "error", "Quotation has no line items")
		return
	}

	orderDate := time.Now()
	if d := r.PostFormValue("order_date"); d != "" {
		if t, err := time.Parse("2006-01-02", d); err == nil {
			orderDate = t
		}
	}

	var expectedDeliveryDate *time.Time
	if d := r.PostFormValue("expected_delivery_date"); d != "" {
		if t, err := time.Parse("2006-01-02", d); err == nil {
			expectedDeliveryDate = &t
		}
	}

	lines := make([]CreateSalesOrderLineReq, 0, len(quotation.Lines))
	for _, line := range quotation.Lines {
		lines = append(lines, CreateSalesOrderLineReq{
			ProductID:       line.ProductID,
			Description:     line.Description,
			Quantity:        line.Quantity,
			UOM:             line.UOM,
			UnitPrice:       line.UnitPrice,
			DiscountPercent: line.DiscountPercent,
			TaxPercent:      line.TaxPercent,
			LineOrder:       line.LineOrder,
			Notes:           line.Notes,
		})
	}

	req := CreateSalesOrderRequest{
		CompanyID:            quotation.CompanyID,
		CustomerID:           quotation.CustomerID,
		QuotationID:          &quotation.ID,
		OrderDate:            orderDate,
		ExpectedDeliveryDate: expectedDeliveryDate,
		Currency:             quotation.Currency,
		Lines:                lines,
		Notes:                quotation.Notes,
	}

	order, err := h.service.Create(r.Context(), req, h.getCurrentUserID(r))
	if err != nil {
		h.logger.Error("convert quotation to order failed", "error", err, "quotation_id", id)
		h.redirectWithFlash(w, r, "/sales/quotations/"+strconv.FormatInt(id, 10), "error", shared.UserSafeMessage(err))
		return
	}

	h.redirectWithFlash(w, r, "/sales/orders/"+strconv.FormatInt(order.ID, 10), "success", "Sales order created from quotation")
}

func (h *Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID := h.getCurrentUserID(r)

	_, err := h.service.Confirm(r.Context(), id, userID)
	if err != nil {
		h.logger.Error("confirm order failed", "error", err, "id", id)
		h.redirectWithFlash(w, r, "/sales/orders/"+strconv.FormatInt(id, 10), "error", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, "/sales/orders/"+strconv.FormatInt(id, 10), "success", "Sales order confirmed")
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID := h.getCurrentUserID(r)
	reason := r.PostFormValue("reason") // assuming posted via form or query? Original used form.

	_, err := h.service.Cancel(r.Context(), id, userID, reason)
	if err != nil {
		h.logger.Error("cancel order failed", "error", err, "id", id)
		h.redirectWithFlash(w, r, "/sales/orders/"+strconv.FormatInt(id, 10), "error", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, "/sales/orders/"+strconv.FormatInt(id, 10), "success", "Sales order cancelled")
}

// Helpers
func (h *Handler) parseSalesOrderLines(r *http.Request) ([]CreateSalesOrderLineReq, error) {
	productIDs := r.PostForm["product_id"]
	quantities := r.PostForm["quantity"]
	uoms := r.PostForm["uom"]
	unitPrices := r.PostForm["unit_price"]
	discountPercents := r.PostForm["discount_percent"]
	taxPercents := r.PostForm["tax_percent"]

	if len(productIDs) == 0 {
		return nil, nil
	}

	lines := make([]CreateSalesOrderLineReq, 0, len(productIDs))
	for i := range productIDs {
		pid, _ := strconv.ParseInt(productIDs[i], 10, 64)
		qty, _ := strconv.ParseFloat(quantities[i], 64)
		price, _ := strconv.ParseFloat(unitPrices[i], 64)
		dist, _ := strconv.ParseFloat(discountPercents[i], 64)
		tax, _ := strconv.ParseFloat(taxPercents[i], 64)

		lines = append(lines, CreateSalesOrderLineReq{
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

func (h *Handler) renderFormError(w http.ResponseWriter, r *http.Request, msg string, o *SalesOrder) {
	companyID := h.getCurrentCompanyID(r)
	customers, _, _ := h.customerService.List(r.Context(), customers.ListCustomersRequest{CompanyID: companyID, Limit: 1000})

	h.render(w, r, "pages/sales/order_form.html", map[string]any{
		"Errors":    formErrors{"general": msg},
		"Order":     o,
		"Customers": customers,
	}, http.StatusBadRequest)
}

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
	h.templates.Render(w, tmpl, viewData)
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
	if sess != nil && sess.User() != "" {
		if id, err := strconv.ParseInt(sess.User(), 10, 64); err == nil {
			return id
		}
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
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}
