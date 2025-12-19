package orders

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

// Handler manages delivery order HTTP endpoints.
type Handler struct {
	logger    *slog.Logger
	service   *Service
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
}

// NewHandler creates a new handler.
func NewHandler(
	logger *slog.Logger,
	service *Service,
	templates *view.Engine,
	csrf *shared.CSRFManager,
	rbac rbac.Middleware,
) *Handler {
	return &Handler{
		logger:    logger,
		service:   service,
		templates: templates,
		csrf:      csrf,
		rbac:      rbac,
	}
}

// MountRoutes registers routes on the router.
func (h *Handler) MountRoutes(r chi.Router) {
	// View routes
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermDeliveryOrderView))
		r.Get("/", h.list)
		r.Get("/{id}", h.show)
	})

	// Create routes
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll(shared.PermDeliveryOrderCreate))
		r.Get("/new", h.showForm)
		r.Post("/", h.create)
	})

	// Edit routes
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll(shared.PermDeliveryOrderEdit))
		r.Get("/{id}/edit", h.showEditForm)
		r.Post("/{id}/edit", h.update)
	})

	// Action routes
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll(shared.PermDeliveryOrderConfirm))
		r.Post("/{id}/confirm", h.confirm)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll(shared.PermDeliveryOrderShip))
		r.Post("/{id}/ship", h.ship)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll(shared.PermDeliveryOrderComplete))
		r.Post("/{id}/complete", h.complete)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll(shared.PermDeliveryOrderCancel))
		r.Post("/{id}/cancel", h.cancel)
	})
}

// list handles GET /delivery/orders
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	companyID := getCompanyID(r)

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 20
	offset := (page - 1) * limit

	req := ListRequest{
		CompanyID: companyID,
		Limit:     limit,
		Offset:    offset,
		SortBy:    r.URL.Query().Get("sort"),
		SortDir:   r.URL.Query().Get("dir"),
	}

	if s := r.URL.Query().Get("status"); s != "" {
		status := Status(s)
		req.Status = &status
	}
	if s := r.URL.Query().Get("search"); s != "" {
		req.Search = &s
	}

	orders, total, err := h.service.List(ctx, req)
	if err != nil {
		h.logger.Error("list orders failed", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/delivery/orders_list.html", map[string]interface{}{
		"DeliveryOrders": orders,
		"CurrentPage":    page,
		"TotalPages":     (total + limit - 1) / limit,
		"TotalCount":     total,
	})
}

// show handles GET /delivery/orders/{id}
func (h *Handler) show(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	order, err := h.service.GetWithDetails(ctx, id)
	if err != nil {
		h.logger.Error("get order failed", "error", err, "id", id)
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	lines, _ := h.service.GetLinesWithDetails(ctx, id)

	h.render(w, r, "pages/delivery/order_detail.html", map[string]interface{}{
		"DeliveryOrder": order,
		"Lines":         lines,
	})
}

// showForm handles GET /delivery/orders/new
func (h *Handler) showForm(w http.ResponseWriter, r *http.Request) {
	soID := r.URL.Query().Get("sales_order_id")
	h.render(w, r, "pages/delivery/order_form.html", map[string]interface{}{
		"SalesOrderID": soID,
	})
}

// create handles POST /delivery/orders
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(r)
	companyID := getCompanyID(r)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	salesOrderID, _ := strconv.ParseInt(r.FormValue("sales_order_id"), 10, 64)
	warehouseID, _ := strconv.ParseInt(r.FormValue("warehouse_id"), 10, 64)
	deliveryDate, _ := time.Parse("2006-01-02", r.FormValue("delivery_date"))

	lines, formErr := parseLines(r)
	if formErr != nil {
		h.renderFormError(w, r, formErr)
		return
	}

	req := CreateRequest{
		CompanyID:    companyID,
		SalesOrderID: salesOrderID,
		WarehouseID:  warehouseID,
		DeliveryDate: deliveryDate,
		Lines:        lines,
	}
	if v := r.FormValue("driver_name"); v != "" {
		req.DriverName = &v
	}
	if v := r.FormValue("vehicle_number"); v != "" {
		req.VehicleNumber = &v
	}
	if v := r.FormValue("notes"); v != "" {
		req.Notes = &v
	}

	order, err := h.service.Create(ctx, req, userID)
	if err != nil {
		h.logger.Error("create order failed", "error", err)
		h.renderFormError(w, r, map[string]string{"_form": err.Error()})
		return
	}

	h.redirect(w, r, "/delivery/orders/"+strconv.FormatInt(order.ID, 10), "Order created")
}

// showEditForm handles GET /delivery/orders/{id}/edit
func (h *Handler) showEditForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	order, err := h.service.GetByID(ctx, id)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/delivery/order_edit.html", map[string]interface{}{
		"DeliveryOrder": order,
	})
}

// update handles POST /delivery/orders/{id}/edit
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	req := UpdateRequest{}
	if d := r.FormValue("delivery_date"); d != "" {
		t, _ := time.Parse("2006-01-02", d)
		req.DeliveryDate = &t
	}
	if v := r.FormValue("driver_name"); v != "" {
		req.DriverName = &v
	}
	if v := r.FormValue("notes"); v != "" {
		req.Notes = &v
	}

	if _, err := h.service.Update(ctx, id, req); err != nil {
		h.logger.Error("update failed", "error", err, "id", id)
		h.renderFormError(w, r, map[string]string{"_form": err.Error()})
		return
	}

	h.redirect(w, r, "/delivery/orders/"+strconv.FormatInt(id, 10), "Order updated")
}

// confirm handles POST /delivery/orders/{id}/confirm
func (h *Handler) confirm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID := getUserID(r)

	if _, err := h.service.Confirm(ctx, id, userID); err != nil {
		h.redirect(w, r, "/delivery/orders/"+strconv.FormatInt(id, 10), "Failed: "+err.Error())
		return
	}

	h.redirect(w, r, "/delivery/orders/"+strconv.FormatInt(id, 10), "Order confirmed")
}

// ship handles POST /delivery/orders/{id}/ship
func (h *Handler) ship(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID := getUserID(r)

	req := MarkInTransitRequest{UpdatedBy: userID}
	if t := r.FormValue("tracking_number"); t != "" {
		req.TrackingNumber = &t
	}

	if _, err := h.service.MarkInTransit(ctx, id, req); err != nil {
		h.redirect(w, r, "/delivery/orders/"+strconv.FormatInt(id, 10), "Failed: "+err.Error())
		return
	}

	h.redirect(w, r, "/delivery/orders/"+strconv.FormatInt(id, 10), "Order shipped")
}

// complete handles POST /delivery/orders/{id}/complete
func (h *Handler) complete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID := getUserID(r)

	deliveredAt := time.Now()
	if d := r.FormValue("delivered_at"); d != "" {
		deliveredAt, _ = time.Parse("2006-01-02", d)
	}

	req := MarkDeliveredRequest{
		DeliveredAt: deliveredAt,
		UpdatedBy:   userID,
	}

	if _, err := h.service.MarkDelivered(ctx, id, req); err != nil {
		h.redirect(w, r, "/delivery/orders/"+strconv.FormatInt(id, 10), "Failed: "+err.Error())
		return
	}

	h.redirect(w, r, "/delivery/orders/"+strconv.FormatInt(id, 10), "Order completed")
}

// cancel handles POST /delivery/orders/{id}/cancel
func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID := getUserID(r)

	reason := r.FormValue("cancellation_reason")
	if reason == "" {
		h.redirect(w, r, "/delivery/orders/"+strconv.FormatInt(id, 10), "Reason required")
		return
	}

	req := CancelRequest{Reason: reason, CancelledBy: userID}

	if _, err := h.service.Cancel(ctx, id, req); err != nil {
		h.redirect(w, r, "/delivery/orders/"+strconv.FormatInt(id, 10), "Failed: "+err.Error())
		return
	}

	h.redirect(w, r, "/delivery/orders/"+strconv.FormatInt(id, 10), "Order cancelled")
}

// Helpers

func (h *Handler) render(w http.ResponseWriter, r *http.Request, tmpl string, data map[string]interface{}) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken := ""
	var flash *shared.FlashMessage
	if sess != nil {
		csrfToken, _ = h.csrf.EnsureToken(r.Context(), sess)
		flash = sess.PopFlash()
	}

	if data == nil {
		data = make(map[string]interface{})
	}
	data["CSRFToken"] = csrfToken

	td := view.TemplateData{CSRFToken: csrfToken, Flash: flash, Data: data}
	if err := h.templates.Render(w, tmpl, td); err != nil {
		h.logger.Error("render failed", "error", err, "template", tmpl)
		http.Error(w, "Render Error", http.StatusInternalServerError)
	}
}

func (h *Handler) redirect(w http.ResponseWriter, r *http.Request, url, message string) {
	if sess := shared.SessionFromContext(r.Context()); sess != nil {
		sess.AddFlash(shared.FlashMessage{Kind: "success", Message: message})
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}

func (h *Handler) renderFormError(w http.ResponseWriter, r *http.Request, errors map[string]string) {
	h.render(w, r, "pages/delivery/order_form.html", map[string]interface{}{
		"Errors":   errors,
		"FormData": r.Form,
	})
}

func getUserID(r *http.Request) int64 {
	if id, ok := r.Context().Value("user_id").(int64); ok {
		return id
	}
	return 0
}

func getCompanyID(r *http.Request) int64 {
	if id, ok := r.Context().Value("company_id").(int64); ok {
		return id
	}
	return 0
}

func parseLines(r *http.Request) ([]CreateLineReq, map[string]string) {
	soLineIDs := r.Form["so_line_id[]"]
	productIDs := r.Form["product_id[]"]
	quantities := r.Form["quantity[]"]

	if len(soLineIDs) == 0 {
		return nil, map[string]string{"lines": "At least one line required"}
	}

	var lines []CreateLineReq
	for i := range soLineIDs {
		soLineID, _ := strconv.ParseInt(soLineIDs[i], 10, 64)
		productID, _ := strconv.ParseInt(productIDs[i], 10, 64)
		qty, _ := strconv.ParseFloat(quantities[i], 64)

		lines = append(lines, CreateLineReq{
			SalesOrderLineID:  soLineID,
			ProductID:         productID,
			QuantityToDeliver: qty,
			LineOrder:         i,
		})
	}

	return lines, nil
}
