package delivery

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

// getSession is a helper to get session from context
func getSession(r *http.Request) *shared.Session {
	return shared.SessionFromContext(r.Context())
}

// Handler manages delivery order endpoints.
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

// MountRoutes registers delivery order routes.
func (h *Handler) MountRoutes(r chi.Router) {
	// Delivery Order routes - View
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermDeliveryOrderView))
		r.Get("/delivery-orders", h.listDeliveryOrders)
		r.Get("/delivery-orders/{id}", h.showDeliveryOrder)
	})

	// Delivery Order routes - Create
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll(shared.PermDeliveryOrderCreate))
		r.Get("/delivery-orders/new", h.showDeliveryOrderForm)
		r.Post("/delivery-orders", h.createDeliveryOrder)
	})

	// Delivery Order routes - Edit
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll(shared.PermDeliveryOrderEdit))
		r.Get("/delivery-orders/{id}/edit", h.showEditDeliveryOrderForm)
		r.Post("/delivery-orders/{id}/edit", h.updateDeliveryOrder)
	})

	// Delivery Order routes - Confirm
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll(shared.PermDeliveryOrderConfirm))
		r.Post("/delivery-orders/{id}/confirm", h.confirmDeliveryOrder)
	})

	// Delivery Order routes - Ship
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll(shared.PermDeliveryOrderShip))
		r.Post("/delivery-orders/{id}/ship", h.shipDeliveryOrder)
	})

	// Delivery Order routes - Complete
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll(shared.PermDeliveryOrderComplete))
		r.Post("/delivery-orders/{id}/complete", h.completeDeliveryOrder)
	})

	// Delivery Order routes - Cancel
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll(shared.PermDeliveryOrderCancel))
		r.Post("/delivery-orders/{id}/cancel", h.cancelDeliveryOrder)
	})

	// Sales Order Integration routes
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermDeliveryOrderView, shared.PermSalesOrderView))
		r.Get("/sales-orders/{id}/delivery-orders", h.listDeliveryOrdersBySalesOrder)
	})
}

type formErrors map[string]string

// listDeliveryOrders shows all delivery orders with filtering and pagination.
func (h *Handler) listDeliveryOrders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	companyID := h.getCurrentCompanyID(r)

	// Parse query parameters
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 20
	offset := (page - 1) * limit

	// Build request
	req := ListDeliveryOrdersRequest{
		CompanyID: companyID,
		Limit:     limit,
		Offset:    offset,
	}

	// Optional filters
	if status := r.URL.Query().Get("status"); status != "" {
		s := DeliveryOrderStatus(status)
		req.Status = &s
	}
	if soID := r.URL.Query().Get("sales_order_id"); soID != "" {
		id, err := strconv.ParseInt(soID, 10, 64)
		if err == nil {
			req.SalesOrderID = &id
		}
	}
	if warehouseID := r.URL.Query().Get("warehouse_id"); warehouseID != "" {
		id, err := strconv.ParseInt(warehouseID, 10, 64)
		if err == nil {
			req.WarehouseID = &id
		}
	}
	if search := r.URL.Query().Get("search"); search != "" {
		req.Search = &search
	}
	if dateFrom := r.URL.Query().Get("date_from"); dateFrom != "" {
		if t, err := time.Parse("2006-01-02", dateFrom); err == nil {
			req.DateFrom = &t
		}
	}
	if dateTo := r.URL.Query().Get("date_to"); dateTo != "" {
		if t, err := time.Parse("2006-01-02", dateTo); err == nil {
			req.DateTo = &t
		}
	}

	// Get delivery orders
	orders, total, err := h.service.ListDeliveryOrders(ctx, req)
	if err != nil {
		h.logger.Error("failed to list delivery orders", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Render template
	data := map[string]interface{}{
		"DeliveryOrders": orders,
		"CurrentPage":    page,
		"TotalPages":     (total + limit - 1) / limit,
		"TotalCount":     total,
		"Request":        req,
	}

	h.render(w, r, "pages/delivery/orders_list.html", data)
}

// showDeliveryOrder displays a single delivery order detail.
func (h *Handler) showDeliveryOrder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid delivery order ID", http.StatusBadRequest)
		return
	}

	order, err := h.service.GetDeliveryOrder(ctx, id)
	if err != nil {
		if err == ErrDeliveryOrderNotFound {
			http.Error(w, "Delivery order not found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get delivery order", "error", err, "id", id)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"DeliveryOrder": order,
	}

	h.render(w, r, "pages/delivery/order_detail.html", data)
}

// showDeliveryOrderForm displays the form to create a new delivery order.
func (h *Handler) showDeliveryOrderForm(w http.ResponseWriter, r *http.Request) {
	sess := getSession(r)
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)

	// Optional: pre-populate from sales order
	soID := r.URL.Query().Get("sales_order_id")

	data := map[string]interface{}{
		"SalesOrderID": soID,
		"CSRFToken":    csrfToken,
	}

	h.render(w, r, "pages/delivery/order_form.html", data)
}

// createDeliveryOrder handles delivery order creation.
func (h *Handler) createDeliveryOrder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := h.getCurrentUserID(r)
	companyID := h.getCurrentCompanyID(r)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	// Parse form data
	salesOrderID, err := strconv.ParseInt(r.FormValue("sales_order_id"), 10, 64)
	if err != nil {
		h.renderDeliveryOrderFormError(w, r, formErrors{
			"sales_order_id": "Invalid sales order ID",
		})
		return
	}

	warehouseID, err := strconv.ParseInt(r.FormValue("warehouse_id"), 10, 64)
	if err != nil {
		h.renderDeliveryOrderFormError(w, r, formErrors{
			"warehouse_id": "Invalid warehouse ID",
		})
		return
	}

	deliveryDate, err := time.Parse("2006-01-02", r.FormValue("delivery_date"))
	if err != nil {
		h.renderDeliveryOrderFormError(w, r, formErrors{
			"delivery_date": "Invalid delivery date",
		})
		return
	}

	// Parse delivery order lines
	lines, parseErr := h.parseDeliveryOrderLines(r)
	if parseErr != nil {
		h.renderDeliveryOrderFormError(w, r, parseErr)
		return
	}

	// Optional fields
	var driverName, vehicleNumber, trackingNumber, notes *string
	if v := r.FormValue("driver_name"); v != "" {
		driverName = &v
	}
	if v := r.FormValue("vehicle_number"); v != "" {
		vehicleNumber = &v
	}
	if v := r.FormValue("tracking_number"); v != "" {
		trackingNumber = &v
	}
	if v := r.FormValue("notes"); v != "" {
		notes = &v
	}

	// Create request
	req := CreateDeliveryOrderRequest{
		CompanyID:      companyID,
		SalesOrderID:   salesOrderID,
		WarehouseID:    warehouseID,
		DeliveryDate:   deliveryDate,
		DriverName:     driverName,
		VehicleNumber:  vehicleNumber,
		TrackingNumber: trackingNumber,
		Notes:          notes,
		Lines:          lines,
	}

	// Create delivery order
	order, err := h.service.CreateDeliveryOrder(ctx, req, userID)
	if err != nil {
		h.logger.Error("failed to create delivery order", "error", err)
		h.renderDeliveryOrderFormError(w, r, formErrors{
			"_form": err.Error(),
		})
		return
	}

	// Redirect to detail page
	h.redirectWithFlash(w, r, "/delivery-orders/"+strconv.FormatInt(order.ID, 10), "Delivery order created successfully")
}

// showEditDeliveryOrderForm displays the form to edit a delivery order.
func (h *Handler) showEditDeliveryOrderForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid delivery order ID", http.StatusBadRequest)
		return
	}

	order, err := h.service.GetDeliveryOrder(ctx, id)
	if err != nil {
		if err == ErrDeliveryOrderNotFound {
			http.Error(w, "Delivery order not found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get delivery order", "error", err, "id", id)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	sess := getSession(r)
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)

	data := map[string]interface{}{
		"DeliveryOrder": order,
		"CSRFToken":     csrfToken,
	}

	h.render(w, r, "pages/delivery/order_edit.html", data)
}

// UpdateDeliveryOrder handles delivery order updates.
func (h *Handler) updateDeliveryOrder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := h.getCurrentUserID(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid delivery order ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	// Build update request
	req := UpdateDeliveryOrderRequest{}

	// Parse optional fields
	if dateStr := r.FormValue("delivery_date"); dateStr != "" {
		deliveryDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			h.renderDeliveryOrderFormError(w, r, formErrors{
				"delivery_date": "Invalid delivery date",
			})
			return
		}
		req.DeliveryDate = &deliveryDate
	}

	if v := r.FormValue("driver_name"); v != "" {
		req.DriverName = &v
	}
	if v := r.FormValue("vehicle_number"); v != "" {
		req.VehicleNumber = &v
	}
	if v := r.FormValue("tracking_number"); v != "" {
		req.TrackingNumber = &v
	}
	if v := r.FormValue("notes"); v != "" {
		req.Notes = &v
	}

	// Parse delivery order lines if provided
	if r.Form["line_item_id[]"] != nil {
		lines, parseErr := h.parseDeliveryOrderLines(r)
		if parseErr != nil {
			h.renderDeliveryOrderFormError(w, r, parseErr)
			return
		}
		req.Lines = &lines
	}

	// Update delivery order - service signature is (ctx, id, req) not (ctx, id, req, userID)
	order, err := h.service.UpdateDeliveryOrder(ctx, id, req)
	if err != nil {
		h.logger.Error("failed to update delivery order", "error", err, "id", id, "user_id", userID)
		h.renderDeliveryOrderFormError(w, r, formErrors{
			"_form": err.Error(),
		})
		return
	}

	// Redirect to detail page
	h.redirectWithFlash(w, r, "/delivery-orders/"+strconv.FormatInt(order.ID, 10), "Delivery order updated successfully")
}

// confirmDeliveryOrder handles delivery order confirmation.
func (h *Handler) confirmDeliveryOrder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := h.getCurrentUserID(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid delivery order ID", http.StatusBadRequest)
		return
	}

	// Service signature is (ctx, id, confirmedBy int64)
	order, err := h.service.ConfirmDeliveryOrder(ctx, id, userID)
	if err != nil {
		h.logger.Error("failed to confirm delivery order", "error", err, "id", id)
		h.redirectWithFlash(w, r, "/delivery-orders/"+strconv.FormatInt(id, 10), "Failed to confirm: "+err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/delivery-orders/"+strconv.FormatInt(order.ID, 10), "Delivery order confirmed successfully")
}

// shipDeliveryOrder handles setting delivery order to in_transit status.
func (h *Handler) shipDeliveryOrder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := h.getCurrentUserID(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid delivery order ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	// Build request
	req := MarkInTransitRequest{
		UpdatedBy: userID,
	}

	// Optional: update tracking number
	if trackingNumber := r.FormValue("tracking_number"); trackingNumber != "" {
		req.TrackingNumber = &trackingNumber
	}

	order, err := h.service.MarkInTransit(ctx, id, req)
	if err != nil {
		h.logger.Error("failed to ship delivery order", "error", err, "id", id)
		h.redirectWithFlash(w, r, "/delivery-orders/"+strconv.FormatInt(id, 10), "Failed to ship: "+err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/delivery-orders/"+strconv.FormatInt(order.ID, 10), "Delivery order shipped successfully")
}

// completeDeliveryOrder handles setting delivery order to delivered status.
func (h *Handler) completeDeliveryOrder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := h.getCurrentUserID(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid delivery order ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	// Parse delivery date (default to now if not provided)
	deliveredAt := time.Now()
	if dateStr := r.FormValue("delivered_at"); dateStr != "" {
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			h.redirectWithFlash(w, r, "/delivery-orders/"+strconv.FormatInt(id, 10), "Invalid delivery date format")
			return
		}
		deliveredAt = parsed
	}

	req := MarkDeliveredRequest{
		DeliveredAt: deliveredAt,
		UpdatedBy:   userID,
	}

	order, err := h.service.MarkDelivered(ctx, id, req)
	if err != nil {
		h.logger.Error("failed to complete delivery order", "error", err, "id", id)
		h.redirectWithFlash(w, r, "/delivery-orders/"+strconv.FormatInt(id, 10), "Failed to complete: "+err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/delivery-orders/"+strconv.FormatInt(order.ID, 10), "Delivery order completed successfully")
}

// cancelDeliveryOrder handles delivery order cancellation.
func (h *Handler) cancelDeliveryOrder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := h.getCurrentUserID(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid delivery order ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	reason := r.FormValue("cancellation_reason")
	if reason == "" {
		h.redirectWithFlash(w, r, "/delivery-orders/"+strconv.FormatInt(id, 10), "Cancellation reason is required")
		return
	}

	req := CancelDeliveryOrderRequest{
		Reason:      reason,
		CancelledBy: userID,
	}

	order, err := h.service.CancelDeliveryOrder(ctx, id, req)
	if err != nil {
		h.logger.Error("failed to cancel delivery order", "error", err, "id", id)
		h.redirectWithFlash(w, r, "/delivery-orders/"+strconv.FormatInt(id, 10), "Failed to cancel: "+err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/delivery-orders/"+strconv.FormatInt(order.ID, 10), "Delivery order cancelled successfully")
}

// listDeliveryOrdersBySalesOrder shows all delivery orders for a specific sales order.
func (h *Handler) listDeliveryOrdersBySalesOrder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	companyID := h.getCurrentCompanyID(r)

	salesOrderID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid sales order ID", http.StatusBadRequest)
		return
	}

	// Build request
	req := ListDeliveryOrdersRequest{
		CompanyID:    companyID,
		SalesOrderID: &salesOrderID,
	}

	// Get delivery orders
	orders, _, err := h.service.ListDeliveryOrders(ctx, req)
	if err != nil {
		h.logger.Error("failed to list delivery orders by sales order", "error", err, "sales_order_id", salesOrderID)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Render template
	data := map[string]interface{}{
		"DeliveryOrders": orders,
		"SalesOrderID":   salesOrderID,
	}

	h.render(w, r, "pages/delivery/orders_by_so.html", data)
}

// render is a helper to render templates.
func (h *Handler) render(w http.ResponseWriter, r *http.Request, template string, data map[string]interface{}) {
	// Get session for flash messages
	sess := getSession(r)
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}

	// Get CSRF token
	csrfToken := ""
	if sess != nil {
		csrfToken, _ = h.csrf.EnsureToken(r.Context(), sess)
	}

	// Build template data
	templateData := view.TemplateData{
		CSRFToken: csrfToken,
		Flash:     flash,
		Data:      data,
	}

	if err := h.templates.Render(w, template, templateData); err != nil {
		h.logger.Error("failed to render template", "error", err, "template", template)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// redirectWithFlash redirects with a flash message.
func (h *Handler) redirectWithFlash(w http.ResponseWriter, r *http.Request, url, message string) {
	sess := getSession(r)
	if sess != nil {
		sess.AddFlash(shared.FlashMessage{
			Kind:    "success",
			Message: message,
		})
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}

// getCurrentUserID retrieves the current user ID from context.
func (h *Handler) getCurrentUserID(r *http.Request) int64 {
	userID, ok := r.Context().Value("user_id").(int64)
	if !ok {
		return 0
	}
	return userID
}

// getCurrentCompanyID retrieves the current company ID from context.
func (h *Handler) getCurrentCompanyID(r *http.Request) int64 {
	companyID, ok := r.Context().Value("company_id").(int64)
	if !ok {
		return 0
	}
	return companyID
}

// parseDeliveryOrderLines parses delivery order lines from form data.
func (h *Handler) parseDeliveryOrderLines(r *http.Request) ([]CreateDeliveryOrderLineReq, formErrors) {
	var lines []CreateDeliveryOrderLineReq
	errors := make(formErrors)

	// Parse line items (format: so_line_id[], product_id[], quantity[], notes[])
	soLineIDs := r.Form["so_line_id[]"]
	productIDs := r.Form["product_id[]"]
	quantities := r.Form["quantity[]"]
	notes := r.Form["line_notes[]"]

	if len(soLineIDs) == 0 {
		errors["lines"] = "At least one line item is required"
		return nil, errors
	}

	if len(soLineIDs) != len(productIDs) || len(soLineIDs) != len(quantities) {
		errors["lines"] = "Mismatched line item data"
		return nil, errors
	}

	for i := 0; i < len(soLineIDs); i++ {
		soLineID, err := strconv.ParseInt(soLineIDs[i], 10, 64)
		if err != nil {
			errors["line_"+strconv.Itoa(i)] = "Invalid SO line ID"
			continue
		}

		productID, err := strconv.ParseInt(productIDs[i], 10, 64)
		if err != nil {
			errors["line_"+strconv.Itoa(i)] = "Invalid product ID"
			continue
		}

		qty, err := strconv.ParseFloat(quantities[i], 64)
		if err != nil || qty <= 0 {
			errors["line_"+strconv.Itoa(i)] = "Invalid quantity"
			continue
		}

		var lineNote *string
		if i < len(notes) && notes[i] != "" {
			lineNote = &notes[i]
		}

		lines = append(lines, CreateDeliveryOrderLineReq{
			SalesOrderLineID:  soLineID,
			ProductID:         productID,
			QuantityToDeliver: qty,
			Notes:             lineNote,
		})
	}

	if len(errors) > 0 {
		return nil, errors
	}

	return lines, nil
}

// renderDeliveryOrderFormError renders form with errors.
func (h *Handler) renderDeliveryOrderFormError(w http.ResponseWriter, r *http.Request, errors formErrors) {
	sess := getSession(r)
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)

	data := map[string]interface{}{
		"Errors":    errors,
		"CSRFToken": csrfToken,
		"FormData":  r.Form,
	}
	h.render(w, r, "pages/delivery/order_form.html", data)
}
