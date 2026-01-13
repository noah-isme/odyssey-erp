package procurement

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

// Handler manages procurement endpoints.
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

// MountRoutes registers procurement routes.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("procurement.view"))
		r.Get("/prs", h.showPRForm)
		r.Get("/pos", h.handleListPOs)
		r.Get("/pos/new", h.showPOForm)
		r.Get("/grns", h.handleListGRNs)
		r.Get("/grns/new", h.showGRNForm)

	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("procurement.edit"))
		r.Post("/prs", h.createPR)
		r.Post("/prs/{id}/submit", h.submitPR)
		r.Post("/pos", h.createPO)
		r.Post("/pos/{id}/submit", h.submitPO)
		r.Post("/pos/{id}/approve", h.approvePO)
		r.Post("/grns", h.createGRN)
		r.Post("/grns/{id}/post", h.postGRN)

	})
}

type formErrors map[string]string

func (h *Handler) showPRForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/procurement/pr_form.html", map[string]any{"Errors": formErrors{}}, http.StatusOK)
}

func (h *Handler) showPOForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/procurement/po_form.html", map[string]any{"Errors": formErrors{}}, http.StatusOK)
}

func (h *Handler) showGRNForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/procurement/grn_form.html", map[string]any{"Errors": formErrors{}}, http.StatusOK)
}

func (h *Handler) handleListPOs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 20
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	supplierID, _ := strconv.ParseInt(r.URL.Query().Get("supplier_id"), 10, 64)
	filters := ListFilters{
		Status:     r.URL.Query().Get("status"),
		SupplierID: supplierID,
		Search:     r.URL.Query().Get("search"),
		SortBy:     r.URL.Query().Get("sort"),
		SortDir:    r.URL.Query().Get("dir"),
	}
	items, total, err := h.service.ListPOs(r.Context(), limit, offset, filters)
	if err != nil {
		h.logger.Error("list POs", slog.Any("error", err))
		http.Error(w, "Failed to load purchase orders", http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/procurement/pos_list.html", map[string]any{
		"POs":     items,
		"Total":   total,
		"Limit":   limit,
		"Offset":  offset,
		"Filters": filters,
	}, http.StatusOK)
}

func (h *Handler) handleListGRNs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 20
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	supplierID, _ := strconv.ParseInt(r.URL.Query().Get("supplier_id"), 10, 64)
	filters := ListFilters{
		Status:     r.URL.Query().Get("status"),
		SupplierID: supplierID,
		Search:     r.URL.Query().Get("search"),
		SortBy:     r.URL.Query().Get("sort"),
		SortDir:    r.URL.Query().Get("dir"),
	}
	items, total, err := h.service.ListGRNs(r.Context(), limit, offset, filters)
	if err != nil {
		h.logger.Error("list GRNs", slog.Any("error", err))
		http.Error(w, "Failed to load goods receipts", http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/procurement/grns_list.html", map[string]any{
		"GRNs":    items,
		"Total":   total,
		"Limit":   limit,
		"Offset":  offset,
		"Filters": filters,
	}, http.StatusOK)
}

func (h *Handler) createPR(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	lines := []PRLineInput{}
	productIDs := r.PostForm["product_id"]
	qtys := r.PostForm["qty"]
	for i := range productIDs {
		pid, _ := strconv.ParseInt(productIDs[i], 10, 64)
		qty, _ := strconv.ParseFloat(qtys[i], 64)
		if pid == 0 || qty <= 0 {
			continue
		}
		lines = append(lines, PRLineInput{ProductID: pid, Qty: qty})
	}
	reqBy, _ := strconv.ParseInt(r.PostFormValue("request_by"), 10, 64)
	supplierID, _ := strconv.ParseInt(r.PostFormValue("supplier_id"), 10, 64)
	_, err := h.service.CreatePurchaseRequest(r.Context(), CreatePRInput{
		Number:     r.PostFormValue("number"),
		SupplierID: supplierID,
		RequestBy:  reqBy,
		Note:       r.PostFormValue("note"),
		Lines:      lines,
	})
	if err != nil {
		h.logger.Error("create PR", slog.Any("error", err))
		h.render(w, r, "pages/procurement/pr_form.html", map[string]any{"Errors": formErrors{"general": shared.UserSafeMessage(err)}}, http.StatusBadRequest)
		return
	}
	h.redirectWithFlash(w, r, "/procurement/prs", "success", "PR berhasil dibuat")
}

func (h *Handler) submitPR(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := h.service.SubmitPurchaseRequest(r.Context(), id, currentUser(r)); err != nil {
		h.logger.Error("submit PR", slog.Any("error", err), slog.Int64("id", id))
		h.render(w, r, "pages/procurement/pr_form.html", map[string]any{"Errors": formErrors{"general": shared.UserSafeMessage(err)}}, http.StatusBadRequest)
		return
	}
	h.redirectWithFlash(w, r, "/procurement/prs", "success", "PR dikirim untuk approval")
}

func (h *Handler) createPO(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	prID, _ := strconv.ParseInt(r.PostFormValue("pr_id"), 10, 64)
	expectedDate, _ := time.Parse("2006-01-02", r.PostFormValue("expected_date"))
	_, err := h.service.CreatePOFromPR(r.Context(), CreatePOInput{
		PRID:         prID,
		Number:       r.PostFormValue("number"),
		Currency:     r.PostFormValue("currency"),
		ExpectedDate: expectedDate,
		Note:         r.PostFormValue("note"),
	})
	if err != nil {
		h.logger.Error("create PO", slog.Any("error", err))
		h.render(w, r, "pages/procurement/po_form.html", map[string]any{"Errors": formErrors{"general": shared.UserSafeMessage(err)}}, http.StatusBadRequest)
		return
	}
	h.redirectWithFlash(w, r, "/procurement/pos", "success", "PO berhasil dibuat")
}

func (h *Handler) submitPO(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := h.service.SubmitPurchaseOrder(r.Context(), id, currentUser(r)); err != nil {
		h.logger.Error("submit PO", slog.Any("error", err), slog.Int64("id", id))
		h.render(w, r, "pages/procurement/po_form.html", map[string]any{"Errors": formErrors{"general": shared.UserSafeMessage(err)}}, http.StatusBadRequest)
		return
	}
	h.redirectWithFlash(w, r, "/procurement/pos", "success", "PO diajukan")
}

func (h *Handler) approvePO(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := h.service.ApprovePurchaseOrder(r.Context(), id, currentUser(r)); err != nil {
		h.logger.Error("approve PO", slog.Any("error", err), slog.Int64("id", id))
		h.render(w, r, "pages/procurement/po_form.html", map[string]any{"Errors": formErrors{"general": shared.UserSafeMessage(err)}}, http.StatusBadRequest)
		return
	}
	h.redirectWithFlash(w, r, "/procurement/pos", "success", "PO disetujui")
}

func (h *Handler) createGRN(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	poID, _ := strconv.ParseInt(r.PostFormValue("po_id"), 10, 64)
	warehouseID, _ := strconv.ParseInt(r.PostFormValue("warehouse_id"), 10, 64)
	supplierID, _ := strconv.ParseInt(r.PostFormValue("supplier_id"), 10, 64)
	receivedAt, _ := time.Parse("2006-01-02", r.PostFormValue("received_at"))
	productIDs := r.PostForm["product_id"]
	qtys := r.PostForm["qty"]
	costs := r.PostForm["unit_cost"]
	var lines []GRNLineInput
	for i := range productIDs {
		pid, _ := strconv.ParseInt(productIDs[i], 10, 64)
		qty, _ := strconv.ParseFloat(qtys[i], 64)
		cost, _ := strconv.ParseFloat(costs[i], 64)
		if pid == 0 || qty <= 0 {
			continue
		}
		lines = append(lines, GRNLineInput{ProductID: pid, Qty: qty, UnitCost: cost})
	}
	_, err := h.service.CreateGoodsReceipt(r.Context(), CreateGRNInput{
		POID:        poID,
		WarehouseID: warehouseID,
		SupplierID:  supplierID,
		Number:      r.PostFormValue("number"),
		ReceivedAt:  receivedAt,
		Note:        r.PostFormValue("note"),
		Lines:       lines,
	})
	if err != nil {
		h.logger.Error("create GRN", slog.Any("error", err))
		h.render(w, r, "pages/procurement/grn_form.html", map[string]any{"Errors": formErrors{"general": shared.UserSafeMessage(err)}}, http.StatusBadRequest)
		return
	}
	h.redirectWithFlash(w, r, "/procurement/grns", "success", "GRN dibuat")
}

func (h *Handler) postGRN(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := h.service.PostGoodsReceipt(r.Context(), id); err != nil {
		h.logger.Error("post GRN", slog.Any("error", err), slog.Int64("id", id))
		h.render(w, r, "pages/procurement/grn_form.html", map[string]any{"Errors": formErrors{"general": shared.UserSafeMessage(err)}}, http.StatusBadRequest)
		return
	}
	h.redirectWithFlash(w, r, "/procurement/grns", "success", "GRN diposting")
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, template string, data map[string]any, status int) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	viewData := view.TemplateData{Title: "Procurement", CSRFToken: csrfToken, Flash: flash, CurrentPath: r.URL.Path, Data: data}
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

func currentUser(r *http.Request) int64 {
	sess := shared.SessionFromContext(r.Context())
	if sess == nil {
		return 0
	}
	id, _ := strconv.ParseInt(sess.User(), 10, 64)
	return id
}
