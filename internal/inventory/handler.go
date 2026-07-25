package inventory

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

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
	pool      *pgxpool.Pool
}

// NewHandler constructs inventory handler.
func NewHandler(logger *slog.Logger, service *Service, templates *view.Engine, csrf *shared.CSRFManager, sessions *shared.SessionManager, rbac rbac.Middleware, pool *pgxpool.Pool) *Handler {
	return &Handler{logger: logger, service: service, templates: templates, csrf: csrf, sessions: sessions, rbac: rbac, pool: pool}
}

// MountRoutes registers inventory routes.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("inventory.view"))
		r.Get("/stock-card", h.handleStockCard)
		r.Get("/dashboard", h.handleDashboard)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("inventory.edit"))
		r.Get("/adjustments", h.handleListAdjustments)
		r.Get("/adjustments/new", h.showAdjustmentForm)
		r.Post("/adjustments", h.handleCreateAdjustment)
		r.Get("/adjustments/{id}", h.handleShowAdjustment)
		r.Post("/adjustments/{id}/lines", h.handleAddAdjustmentLine)
		r.Post("/adjustments/{id}/post", h.handlePostAdjustment)
		r.Get("/transfers", h.showTransferForm)
		r.Post("/transfers", h.handleTransfer)
		r.Post("/reorder-requests", h.handleCreateReorderRequests)

		r.Get("/stock-takes", h.handleListStockTakes)
		r.Get("/stock-takes/new", h.showCreateStockTakeForm)
		r.Post("/stock-takes", h.handleCreateStockTake)
		r.Get("/stock-takes/{id}", h.handleShowStockTake)
		r.Post("/stock-takes/{id}/lines", h.handleAddStockTakeLine)
		r.Post("/stock-takes/{id}/post", h.handlePostStockTake)
		r.Get("/valuation", h.handleValuation)
	})
}

func (h *Handler) handleCreateReorderRequests(w http.ResponseWriter, r *http.Request) {
	sess, _ := h.sessions.Load(r.Context(), r)
	count, err := h.service.CreateReorderRequests(r.Context(), currentUserID(sess))
	if err != nil {
		if sess != nil {
			sess.AddFlash(shared.FlashMessage{Kind: "error", Message: shared.UserSafeMessage(err)})
		}
		_ = h.sessions.Commit(r.Context(), w, r, sess)
		http.Redirect(w, r, "/inventory/dashboard", http.StatusSeeOther)
		return
	}
	if sess != nil {
		sess.AddFlash(shared.FlashMessage{Kind: "success", Message: fmt.Sprintf("Created %d draft reorder request(s)", count)})
	}
	_ = h.sessions.Commit(r.Context(), w, r, sess)
	http.Redirect(w, r, "/procurement/prs", http.StatusSeeOther)
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

func (h *Handler) handleListAdjustments(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListAdjustments(r.Context())
	if err != nil {
		h.logger.Error("list adjustments", slog.Any("error", err))
		http.Error(w, "Gagal memuat daftar penyesuaian", http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/inventory/adjustment_list.html", map[string]any{
		"Adjustments": items,
	}, http.StatusOK)
}

func (h *Handler) showAdjustmentForm(w http.ResponseWriter, r *http.Request) {
	warehouses, err := h.getWarehouses(r.Context())
	if err != nil {
		h.logger.Error("get warehouses", slog.Any("error", err))
		http.Error(w, "Gagal memuat gudang", http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/inventory/adjustment_form.html", map[string]any{
		"Warehouses": warehouses,
	}, http.StatusOK)
}

func (h *Handler) handleCreateAdjustment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	warehouseID, _ := strconv.ParseInt(r.PostFormValue("warehouse_id"), 10, 64)
	adjustmentAt, _ := time.Parse("2006-01-02", r.PostFormValue("adjustment_at"))
	if adjustmentAt.IsZero() {
		adjustmentAt = time.Now()
	}

	sess := shared.SessionFromContext(r.Context())

	id, err := h.service.CreateAdjustment(r.Context(), CreateAdjustmentInput{
		WarehouseID:  warehouseID,
		AdjustmentAt: adjustmentAt,
		Note:         r.PostFormValue("note"),
	}, currentUserID(sess))

	if err != nil {
		h.logger.Error("create adjustment", slog.Any("error", err))
		if sess != nil {
			sess.AddFlash(shared.FlashMessage{Kind: "error", Message: shared.UserSafeMessage(err)})
		}
		http.Redirect(w, r, "/inventory/adjustments/new", http.StatusSeeOther)
		return
	}
	if sess != nil {
		sess.AddFlash(shared.FlashMessage{Kind: "success", Message: "Dokumen penyesuaian dibuat"})
	}
	http.Redirect(w, r, "/inventory/adjustments/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (h *Handler) handleShowAdjustment(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	adj, err := h.service.GetAdjustment(r.Context(), id)
	if err != nil {
		h.logger.Error("get adjustment", slog.Any("error", err), slog.Int64("id", id))
		http.Error(w, "Penyesuaian tidak ditemukan", http.StatusNotFound)
		return
	}
	products, _ := h.getProducts(r.Context())

	h.render(w, r, "pages/inventory/adjustment_detail.html", map[string]any{
		"Adjustment": adj,
		"Products":   products,
	}, http.StatusOK)
}

func (h *Handler) handleAddAdjustmentLine(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	productID, _ := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	qty, _ := strconv.ParseFloat(r.PostFormValue("qty"), 64)

	sess := shared.SessionFromContext(r.Context())
	err := h.service.AddAdjustmentLine(r.Context(), id, AddAdjustmentLineInput{
		ProductID: productID,
		Qty:       qty,
		Note:      r.PostFormValue("note"),
	})
	if err != nil {
		h.logger.Error("add adjustment line", slog.Any("error", err))
		if sess != nil {
			sess.AddFlash(shared.FlashMessage{Kind: "error", Message: shared.UserSafeMessage(err)})
		}
		http.Redirect(w, r, "/inventory/adjustments/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
		return
	}
	if sess != nil {
		sess.AddFlash(shared.FlashMessage{Kind: "success", Message: "Item ditambahkan"})
	}
	http.Redirect(w, r, "/inventory/adjustments/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (h *Handler) handlePostAdjustment(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	sess := shared.SessionFromContext(r.Context())
	userID := currentUserID(sess)

	if err := h.service.PostAdjustmentDocument(r.Context(), id, userID); err != nil {
		h.logger.Error("post adjustment", slog.Any("error", err), slog.Int64("id", id))
		if sess != nil {
			sess.AddFlash(shared.FlashMessage{Kind: "error", Message: shared.UserSafeMessage(err)})
		}
		http.Redirect(w, r, "/inventory/adjustments/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
		return
	}
	if sess != nil {
		sess.AddFlash(shared.FlashMessage{Kind: "success", Message: "Penyesuaian stok berhasil diposting"})
	}
	http.Redirect(w, r, "/inventory/adjustments/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
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

func (h *Handler) renderTransfer(w http.ResponseWriter, r *http.Request, form transferForm, errors map[string]string, status int) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}

	warehouses, _ := h.getWarehouses(r.Context())
	products, _ := h.getProducts(r.Context())

	viewData := view.TemplateData{
		Title:       "Transfer Stok",
		CSRFToken:   csrfToken,
		Flash:       flash,
		CurrentPath: r.URL.Path,
		Data: map[string]any{
			"Form":       form,
			"Errors":     errors,
			"Warehouses": warehouses,
			"Products":   products,
		},
	}
	w.WriteHeader(status)
	if err := h.templates.Render(w, "pages/inventory/transfer_form.html", viewData); err != nil {
		h.logger.Error("render transfer", slog.Any("error", err))
	}
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

func (h *Handler) handleListStockTakes(w http.ResponseWriter, r *http.Request) {
	st, err := h.service.repo.ListStockTakes(r.Context())
	if err != nil {
		h.logger.Error("list stock takes failed", slog.Any("error", err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/inventory/stock_take_list.html", map[string]any{
		"StockTakes": st,
	}, http.StatusOK)
}

func (h *Handler) showCreateStockTakeForm(w http.ResponseWriter, r *http.Request) {
	warehouses, _ := h.getWarehouses(r.Context())
	h.render(w, r, "pages/inventory/stock_take_form.html", map[string]any{
		"Warehouses": warehouses,
	}, http.StatusOK)
}

func (h *Handler) handleCreateStockTake(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	warehouseID, _ := strconv.ParseInt(r.FormValue("warehouse_id"), 10, 64)
	note := r.FormValue("note")
	takenAt, _ := time.Parse("2006-01-02", r.FormValue("taken_at"))
	if takenAt.IsZero() {
		takenAt = time.Now()
	}

	sess := shared.SessionFromContext(r.Context())
	id, err := h.service.CreateStockTake(r.Context(), CreateStockTakeInput{
		WarehouseID: warehouseID,
		Note:        note,
		TakenAt:     takenAt,
		CreatedBy:   currentUserID(sess),
	})

	if err != nil {
		h.logger.Error("create stock take failed", slog.Any("error", err))
		// render with error...
		http.Error(w, "Failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/inventory/stock-takes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (h *Handler) handleShowStockTake(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	st, err := h.service.repo.GetStockTake(r.Context(), id)
	if err != nil {
		http.Error(w, "Stock Take not found", http.StatusNotFound)
		return
	}

	products, _ := h.getProducts(r.Context())

	h.render(w, r, "pages/inventory/stock_take_detail.html", map[string]any{
		"StockTake": st,
		"Products":  products,
	}, http.StatusOK)
}

func (h *Handler) handleAddStockTakeLine(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	productID, _ := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
	physicalQty, _ := strconv.ParseFloat(r.FormValue("physical_qty"), 64)
	note := r.FormValue("note")

	err := h.service.AddStockTakeLine(r.Context(), AddStockTakeLineInput{
		StockTakeID: id,
		ProductID:   productID,
		PhysicalQty: physicalQty,
		Note:        note,
	})

	if err != nil {
		h.logger.Error("add line failed", slog.Any("error", err))
		// flash error and redirect
	}

	http.Redirect(w, r, "/inventory/stock-takes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (h *Handler) handlePostStockTake(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	sess := shared.SessionFromContext(r.Context())

	if err := h.service.PostStockTake(r.Context(), id, currentUserID(sess)); err != nil {
		h.logger.Error("post stock take failed", slog.Any("error", err))
		// flash error
	} else {
		if sess != nil {
			sess.AddFlash(shared.FlashMessage{Kind: "success", Message: "Stock Take posted successfully"})
		}
	}

	http.Redirect(w, r, "/inventory/stock-takes/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (h *Handler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	alerts, err := h.service.GetReorderAlerts(r.Context())
	if err != nil {
		h.logger.Error("failed to get reorder alerts", slog.Any("error", err))
	}

	h.render(w, r, "pages/inventory/dashboard.html", map[string]any{
		"Alerts": alerts,
	}, http.StatusOK)
}

func (h *Handler) handleValuation(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	warehouseID, _ := strconv.ParseInt(q.Get("warehouse_id"), 10, 64)
	method := q.Get("method") // "AVG" or "FIFO"
	if method == "" {
		method = "AVG"
	}

	var entries []ValuationEntry
	var err error

	if method == "FIFO" {
		entries, err = h.service.GetFIFOValuation(r.Context(), warehouseID)
	} else {
		entries, err = h.service.GetValuation(r.Context(), warehouseID)
	}

	if err != nil {
		h.logger.Error("valuation failed", slog.Any("error", err), slog.String("method", method))
		http.Error(w, "Gagal memuat laporan valuasi", http.StatusInternalServerError)
		return
	}

	warehouses, _ := h.getWarehouses(r.Context())

	var totalValuation float64
	for _, e := range entries {
		totalValuation += e.TotalValue
	}

	h.render(w, r, "pages/inventory/valuation.html", map[string]any{
		"Entries":        entries,
		"Warehouses":     warehouses,
		"WarehouseID":    warehouseID,
		"TotalValuation": totalValuation,
		"Method":         method,
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
		Title:       "Inventory",
		CSRFToken:   csrfToken,
		Flash:       flash,
		CurrentPath: r.URL.Path,
		Data:        data,
	}
	w.WriteHeader(status)
	if err := h.templates.Render(w, template, viewData); err != nil {
		h.logger.Error("render template", slog.Any("error", err), slog.String("template", template))
	}
}

func (h *Handler) getWarehouses(ctx context.Context) ([]map[string]any, error) {
	rows, err := h.pool.Query(ctx, "SELECT id, name FROM warehouses ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []map[string]any
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		res = append(res, map[string]any{"ID": id, "Name": name})
	}
	return res, nil
}

func (h *Handler) getProducts(ctx context.Context) ([]map[string]any, error) {
	rows, err := h.pool.Query(ctx, "SELECT id, name, sku FROM products WHERE is_active = true ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []map[string]any
	for rows.Next() {
		var id int64
		var name, sku string
		if err := rows.Scan(&id, &name, &sku); err != nil {
			return nil, err
		}
		res = append(res, map[string]any{"ID": id, "Name": name, "SKU": sku})
	}
	return res, nil
}
