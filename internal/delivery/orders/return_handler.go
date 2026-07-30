package orders

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

func (h *Handler) showReturnForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid delivery ID", http.StatusBadRequest)
		return
	}
	order, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, "Delivery order not found", http.StatusNotFound)
		return
	}
	h.render(w, r, "pages/delivery/return_form.html", map[string]interface{}{"DeliveryOrder": order, "Today": time.Now().Format("2006-01-02")})
}

func (h *Handler) createReturn(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid delivery ID", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}
	returnDate, err := time.Parse("2006-01-02", r.FormValue("return_date"))
	if err != nil {
		http.Error(w, "Invalid return date", http.StatusBadRequest)
		return
	}
	lineIDs := r.Form["delivery_order_line_id[]"]
	productIDs := r.Form["product_id[]"]
	quantities := r.Form["quantity[]"]
	prices := r.Form["unit_price[]"]
	warehouses := r.Form["restock_warehouse_id[]"]
	lines := make([]CreateReturnLineReq, 0, len(lineIDs))
	for i := range lineIDs {
		quantity, _ := strconv.ParseFloat(valueAt(quantities, i), 64)
		if quantity <= 0 {
			continue
		}
		lineID, _ := strconv.ParseInt(lineIDs[i], 10, 64)
		productID, _ := strconv.ParseInt(valueAt(productIDs, i), 10, 64)
		price, _ := strconv.ParseFloat(valueAt(prices, i), 64)
		warehouseID, _ := strconv.ParseInt(valueAt(warehouses, i), 10, 64)
		lines = append(lines, CreateReturnLineReq{DeliveryOrderLineID: lineID, ProductID: productID, QuantityReturned: quantity, UnitPrice: price, RestockWarehouseID: &warehouseID, LineOrder: i})
	}
	returned, err := h.service.CreateReturnDeliveryOrder(r.Context(), CreateReturnRequest{
		CompanyID: getCompanyID(r), OriginalDeliveryOrderID: id, ReturnDate: returnDate,
		Reason: r.FormValue("reason"), Lines: lines,
	}, getUserID(r))
	if err != nil {
		h.redirect(w, r, "/delivery/orders/"+strconv.FormatInt(id, 10)+"/returns/new", shared.UserSafeMessage(err))
		return
	}
	h.redirect(w, r, "/delivery/orders/returns/"+strconv.FormatInt(returned.ID, 10), "Return delivery order created")
}

func (h *Handler) showReturn(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "returnID"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid return ID", http.StatusBadRequest)
		return
	}
	returned, err := h.service.GetReturnByID(r.Context(), id)
	if err != nil {
		http.Error(w, "Return delivery order not found", http.StatusNotFound)
		return
	}
	h.render(w, r, "pages/delivery/return_detail.html", map[string]interface{}{"Return": returned})
}

func (h *Handler) confirmReturn(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "returnID"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid return ID", http.StatusBadRequest)
		return
	}
	if _, err := h.service.ConfirmReturnDeliveryOrder(r.Context(), id, getUserID(r)); err != nil {
		h.redirect(w, r, returnURL(id), shared.UserSafeMessage(err))
		return
	}
	h.redirect(w, r, returnURL(id), "Inventory restocked")
}

func (h *Handler) cancelReturn(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "returnID"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid return ID", http.StatusBadRequest)
		return
	}
	if _, err := h.service.CancelReturnDeliveryOrder(r.Context(), id, getUserID(r), r.FormValue("reason")); err != nil {
		h.redirect(w, r, returnURL(id), shared.UserSafeMessage(err))
		return
	}
	h.redirect(w, r, returnURL(id), "Return delivery order cancelled")
}

func valueAt(values []string, index int) string {
	if index >= len(values) {
		return ""
	}
	return values[index]
}

func returnURL(id int64) string {
	return "/delivery/orders/returns/" + strconv.FormatInt(id, 10)
}
