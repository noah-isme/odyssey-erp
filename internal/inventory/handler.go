package inventory

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

// Handler wires HTTP endpoints for inventory module.
type Handler struct {
	logger    *slog.Logger
	service   *Service
	templates *view.Engine
	csrf      *shared.CSRFManager
	sessions  *shared.SessionManager
	rbac      rbac.Middleware
}

// NewHandler constructs inventory handler.
func NewHandler(logger *slog.Logger, service *Service, templates *view.Engine, csrf *shared.CSRFManager, sessions *shared.SessionManager, rbac rbac.Middleware) *Handler {
	return &Handler{logger: logger, service: service, templates: templates, csrf: csrf, sessions: sessions, rbac: rbac}
}

// MountRoutes registers inventory routes.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("inventory.view"))
		r.Get("/stock-card", h.handleStockCard)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("inventory.edit"))
		r.Get("/adjustments", h.showAdjustmentForm)
		r.Post("/adjustments", h.handleAdjustment)
		r.Get("/transfers", h.showTransferForm)
		r.Post("/transfers", h.handleTransfer)
	})
}

type stockCardPageData struct {
	WarehouseID int64
	ProductID   int64
	From        string
	To          string
	Entries     []StockCardEntry
	Errors      map[string]string
	AppEnv      string
}

type adjustmentForm struct {
	WarehouseID int64
	ProductID   int64
	Qty         float64
	UnitCost    float64
	Note        string
	Code        string
}

type transferForm struct {
	SrcWarehouse int64
	DstWarehouse int64
	ProductID    int64
	Qty          float64
	UnitCost     float64
	Note         string
	Code         string
}

func (h *Handler) handleStockCard(w http.ResponseWriter, r *http.Request) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	data := stockCardPageData{Errors: map[string]string{}}
	q := r.URL.Query()
	if warehouseStr := q.Get("warehouse_id"); warehouseStr != "" {
		if id, err := strconv.ParseInt(warehouseStr, 10, 64); err == nil {
			data.WarehouseID = id
		} else {
			data.Errors["warehouse_id"] = "Warehouse tidak valid"
		}
	}
	if productStr := q.Get("product_id"); productStr != "" {
		if id, err := strconv.ParseInt(productStr, 10, 64); err == nil {
			data.ProductID = id
		} else {
			data.Errors["product_id"] = "Produk tidak valid"
		}
	}
	data.From = q.Get("from")
	data.To = q.Get("to")
	if data.WarehouseID != 0 && data.ProductID != 0 && len(data.Errors) == 0 {
		var fromTime, toTime time.Time
		var err error
		if data.From != "" {
			fromTime, err = time.Parse("2006-01-02", data.From)
			if err != nil {
				data.Errors["from"] = "Tanggal mulai tidak valid"
			}
		}
		if data.To != "" {
			toTime, err = time.Parse("2006-01-02", data.To)
			if err != nil {
				data.Errors["to"] = "Tanggal akhir tidak valid"
			} else {
				// Set to end of day
				toTime = toTime.Add(24*time.Hour - 1*time.Nanosecond)
			}
		}
		if len(data.Errors) == 0 {
			entries, err := h.service.GetStockCard(r.Context(), StockCardFilter{WarehouseID: data.WarehouseID, ProductID: data.ProductID, From: fromTime, To: toTime, Limit: 500})
			if err != nil {
				data.Errors["general"] = shared.UserSafeMessage(err)
				h.logger.Error("failed to get stock card", slog.Any("error", err))
			} else {
				data.Entries = entries
				h.logger.Info("got stock card",
					slog.Int("count", len(entries)),
					slog.Int64("warehouse_id", data.WarehouseID),
					slog.Int64("product_id", data.ProductID))
			}
		}
	} else {
		h.logger.Info("stock card missing filters",
			slog.Int64("warehouse_id", data.WarehouseID),
			slog.Int64("product_id", data.ProductID))
	}
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	viewData := view.TemplateData{Title: "Kartu Stok", CSRFToken: csrfToken, Flash: flash, CurrentPath: r.URL.Path, Data: data}
	if err := h.templates.Render(w, "pages/inventory/stock_card.html", viewData); err != nil {
		h.logger.Error("render stock card", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (h *Handler) showAdjustmentForm(w http.ResponseWriter, r *http.Request) {
	h.renderAdjustment(w, r, adjustmentForm{}, map[string]string{}, http.StatusOK)
}

func (h *Handler) handleAdjustment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	sess := shared.SessionFromContext(r.Context())
	form, errors := parseAdjustmentForm(r)
	if len(errors) == 0 {
		_, err := h.service.PostAdjustment(r.Context(), AdjustmentInput{
			Code:        form.Code,
			WarehouseID: form.WarehouseID,
			ProductID:   form.ProductID,
			Qty:         form.Qty,
			UnitCost:    form.UnitCost,
			Note:        form.Note,
			ActorID:     currentUserID(sess),
			RefModule:   "INVENTORY",
		})
		if err != nil {
			h.logger.Error("post adjustment failed", slog.Any("error", err))
			errors["general"] = shared.UserSafeMessage(err)
		} else {
			if sess != nil {
				sess.AddFlash(shared.FlashMessage{Kind: "success", Message: "Penyesuaian stok berhasil diposting"})
			}
			http.Redirect(w, r, "/inventory/adjustments", http.StatusSeeOther)
			return
		}
	}
	h.renderAdjustment(w, r, form, errors, http.StatusBadRequest)
}

func (h *Handler) showTransferForm(w http.ResponseWriter, r *http.Request) {
	h.renderTransfer(w, r, transferForm{}, map[string]string{}, http.StatusOK)
}

func (h *Handler) handleTransfer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	sess := shared.SessionFromContext(r.Context())
	form, errors := parseTransferForm(r)
	if len(errors) == 0 {
		_, _, err := h.service.PostTransfer(r.Context(), TransferInput{
			Code:         form.Code,
			ProductID:    form.ProductID,
			Qty:          form.Qty,
			SrcWarehouse: form.SrcWarehouse,
			DstWarehouse: form.DstWarehouse,
			UnitCost:     form.UnitCost,
			Note:         form.Note,
			ActorID:      currentUserID(sess),
			RefModule:    "INVENTORY",
		})
		if err != nil {
			h.logger.Error("post transfer failed", slog.Any("error", err))
			errors["general"] = shared.UserSafeMessage(err)
		} else {
			if sess != nil {
				sess.AddFlash(shared.FlashMessage{Kind: "success", Message: "Transfer stok berhasil"})
			}
			http.Redirect(w, r, "/inventory/transfers", http.StatusSeeOther)
			return
		}
	}
	h.renderTransfer(w, r, form, errors, http.StatusBadRequest)
}

func (h *Handler) renderAdjustment(w http.ResponseWriter, r *http.Request, form adjustmentForm, errors map[string]string, status int) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	viewData := view.TemplateData{Title: "Penyesuaian Stok", CSRFToken: csrfToken, Flash: flash, CurrentPath: r.URL.Path, Data: map[string]any{"Form": form, "Errors": errors}}
	w.WriteHeader(status)
	if err := h.templates.Render(w, "pages/inventory/adjustment_form.html", viewData); err != nil {
		h.logger.Error("render adjustment", slog.Any("error", err))
	}
}

func (h *Handler) renderTransfer(w http.ResponseWriter, r *http.Request, form transferForm, errors map[string]string, status int) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	viewData := view.TemplateData{Title: "Transfer Stok", CSRFToken: csrfToken, Flash: flash, CurrentPath: r.URL.Path, Data: map[string]any{"Form": form, "Errors": errors}}
	w.WriteHeader(status)
	if err := h.templates.Render(w, "pages/inventory/transfer_form.html", viewData); err != nil {
		h.logger.Error("render transfer", slog.Any("error", err))
	}
}

func parseAdjustmentForm(r *http.Request) (adjustmentForm, map[string]string) {
	errors := make(map[string]string)
	form := adjustmentForm{Note: r.PostFormValue("note"), Code: r.PostFormValue("code")}
	if warehouseID, err := strconv.ParseInt(r.PostFormValue("warehouse_id"), 10, 64); err == nil {
		form.WarehouseID = warehouseID
	} else {
		errors["warehouse_id"] = "Warehouse wajib diisi"
	}
	if productID, err := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64); err == nil {
		form.ProductID = productID
	} else {
		errors["product_id"] = "Produk wajib diisi"
	}
	if qty, err := strconv.ParseFloat(r.PostFormValue("qty"), 64); err == nil {
		form.Qty = qty
	} else {
		errors["qty"] = "Qty tidak valid"
	}
	if costStr := r.PostFormValue("unit_cost"); costStr != "" {
		if cost, err := strconv.ParseFloat(costStr, 64); err == nil {
			form.UnitCost = cost
		} else {
			errors["unit_cost"] = "Biaya tidak valid"
		}
	}
	return form, errors
}

func parseTransferForm(r *http.Request) (transferForm, map[string]string) {
	errors := make(map[string]string)
	form := transferForm{Note: r.PostFormValue("note"), Code: r.PostFormValue("code")}
	if src, err := strconv.ParseInt(r.PostFormValue("src_warehouse"), 10, 64); err == nil {
		form.SrcWarehouse = src
	} else {
		errors["src_warehouse"] = "Gudang asal wajib"
	}
	if dst, err := strconv.ParseInt(r.PostFormValue("dst_warehouse"), 10, 64); err == nil {
		form.DstWarehouse = dst
	} else {
		errors["dst_warehouse"] = "Gudang tujuan wajib"
	}
	if productID, err := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64); err == nil {
		form.ProductID = productID
	} else {
		errors["product_id"] = "Produk wajib"
	}
	if qty, err := strconv.ParseFloat(r.PostFormValue("qty"), 64); err == nil {
		form.Qty = qty
	} else {
		errors["qty"] = "Qty tidak valid"
	}
	if cost, err := strconv.ParseFloat(r.PostFormValue("unit_cost"), 64); err == nil {
		form.UnitCost = cost
	} else {
		errors["unit_cost"] = "Biaya tidak valid"
	}
	return form, errors
}

func currentUserID(sess *shared.Session) int64 {
	if sess == nil {
		return 0
	}
	id, _ := strconv.ParseInt(sess.User(), 10, 64)
	return id
}
