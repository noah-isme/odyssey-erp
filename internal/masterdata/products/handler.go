package products

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/categories"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/taxes"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/units"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	internalShared "github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type Handler struct {
	logger          *slog.Logger
	service         *Service
	categoryService *categories.Service
	unitService     *units.Service
	taxService      *taxes.Service
	templates       *view.Engine
	csrf            *internalShared.CSRFManager
	sessions        *internalShared.SessionManager
	rbac            rbac.Middleware
}

func NewHandler(
	logger *slog.Logger,
	service *Service,
	categoryService *categories.Service,
	unitService *units.Service,
	taxService *taxes.Service,
	templates *view.Engine,
	csrf *internalShared.CSRFManager,
	sessions *internalShared.SessionManager,
	rbac rbac.Middleware,
) *Handler {
	return &Handler{
		logger:          logger,
		service:         service,
		categoryService: categoryService,
		unitService:     unitService,
		taxService:      taxService,
		templates:       templates,
		csrf:            csrf,
		sessions:        sessions,
		rbac:            rbac,
	}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 10
	}

	filters := shared.ListFilters{
		Page:    page,
		Limit:   limit,
		Search:  r.URL.Query().Get("search"),
		SortBy:  r.URL.Query().Get("sort"),
		SortDir: r.URL.Query().Get("dir"),
	}

	if r.URL.Query().Get("category_id") != "" {
		if id, err := strconv.ParseInt(r.URL.Query().Get("category_id"), 10, 64); err == nil {
			filters.CategoryID = &id
		}
	}
	if r.URL.Query().Get("is_active") != "" {
		isActive := r.URL.Query().Get("is_active") == "true"
		filters.IsActive = &isActive
	}

	products, total, err := h.service.List(r.Context(), filters)
	if err != nil {
		h.logger.Error("list products failed", "error", err)
		http.Error(w, "Failed to load products", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/masterdata/products_list.html", map[string]any{
		"Products": products,
		"Filters":  filters,
		"Total":    total,
	}, http.StatusOK)
}

func (h *Handler) Show(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid product ID", http.StatusBadRequest)
		return
	}

	product, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.logger.Error("get product failed", "error", err, "id", id)
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/masterdata/product_detail.html", map[string]any{
		"Product": product,
	}, http.StatusOK)
}

func (h *Handler) Form(w http.ResponseWriter, r *http.Request) {
	cats, _, _ := h.categoryService.List(r.Context(), shared.ListFilters{})
	us, _, _ := h.unitService.List(r.Context(), shared.ListFilters{})
	ts, _, _ := h.taxService.List(r.Context(), shared.ListFilters{})

	h.render(w, r, "pages/masterdata/product_form.html", map[string]any{
		"Errors":     map[string]string{},
		"Product":    nil,
		"Categories": cats,
		"Units":      us,
		"Taxes":      ts,
	}, http.StatusOK)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	catID, _ := strconv.ParseInt(r.PostFormValue("category_id"), 10, 64)
	unitID, _ := strconv.ParseInt(r.PostFormValue("unit_id"), 10, 64)
	taxID, _ := strconv.ParseInt(r.PostFormValue("tax_id"), 10, 64)
	price, _ := strconv.ParseFloat(r.PostFormValue("price"), 64)
	cost, _ := strconv.ParseFloat(r.PostFormValue("cost"), 64)
	active := r.PostFormValue("is_active") == "on"

	product := Product{
		Code:       r.PostFormValue("code"),
		Name:       r.PostFormValue("name"),
		CategoryID: catID,
		UnitID:     unitID,
		TaxID:      taxID,
		Price:      price,
		Cost:       cost,
		IsActive:   active,
	}

	created, err := h.service.Create(r.Context(), product)
	if err != nil {
		cats, _, _ := h.categoryService.List(r.Context(), shared.ListFilters{})
		us, _, _ := h.unitService.List(r.Context(), shared.ListFilters{})
		ts, _, _ := h.taxService.List(r.Context(), shared.ListFilters{})
		h.render(w, r, "pages/masterdata/product_form.html", map[string]any{
			"Errors":     map[string]string{"general": err.Error()},
			"Product":    nil,
			"Categories": cats,
			"Units":      us,
			"Taxes":      ts,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/products/"+strconv.FormatInt(created.ID, 10), "success", "Product created successfully")
}

func (h *Handler) EditForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid product ID", http.StatusBadRequest)
		return
	}

	product, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.logger.Error("get product failed", "error", err, "id", id)
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}

	cats, _, _ := h.categoryService.List(r.Context(), shared.ListFilters{})
	us, _, _ := h.unitService.List(r.Context(), shared.ListFilters{})
	ts, _, _ := h.taxService.List(r.Context(), shared.ListFilters{})

	h.render(w, r, "pages/masterdata/product_form.html", map[string]any{
		"Errors":     map[string]string{},
		"Product":    product,
		"Categories": cats,
		"Units":      us,
		"Taxes":      ts,
	}, http.StatusOK)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid product ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	catID, _ := strconv.ParseInt(r.PostFormValue("category_id"), 10, 64)
	unitID, _ := strconv.ParseInt(r.PostFormValue("unit_id"), 10, 64)
	taxID, _ := strconv.ParseInt(r.PostFormValue("tax_id"), 10, 64)
	price, _ := strconv.ParseFloat(r.PostFormValue("price"), 64)
	cost, _ := strconv.ParseFloat(r.PostFormValue("cost"), 64)
	active := r.PostFormValue("is_active") == "on"

	product := Product{
		Code:       r.PostFormValue("code"),
		Name:       r.PostFormValue("name"),
		CategoryID: catID,
		UnitID:     unitID,
		TaxID:      taxID,
		Price:      price,
		Cost:       cost,
		IsActive:   active,
	}

	err = h.service.Update(r.Context(), id, product)
	if err != nil {
		cats, _, _ := h.categoryService.List(r.Context(), shared.ListFilters{})
		us, _, _ := h.unitService.List(r.Context(), shared.ListFilters{})
		ts, _, _ := h.taxService.List(r.Context(), shared.ListFilters{})
		h.render(w, r, "pages/masterdata/product_form.html", map[string]any{
			"Errors":     map[string]string{"general": err.Error()},
			"Product":    product,
			"Categories": cats,
			"Units":      us,
			"Taxes":      ts,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/products/"+strconv.FormatInt(id, 10), "success", "Product updated successfully")
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid product ID", http.StatusBadRequest)
		return
	}

	err = h.service.Delete(r.Context(), id)
	if err != nil {
		h.redirectWithFlash(w, r, "/masterdata/products", "error", err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/products", "success", "Product deleted successfully")
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, template string, data map[string]any, status int) {
	sess := internalShared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	var flash *internalShared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	viewData := view.TemplateData{
		Title:       "Master Data",
		CSRFToken:   csrfToken,
		Flash:       flash,
		CurrentPath: r.URL.Path,
		Data:        data,
	}
	w.WriteHeader(status)
	if err := h.templates.Render(w, template, viewData); err != nil {
		h.logger.Error("render template", "error", err, "template", template)
	}
}

func (h *Handler) redirectWithFlash(w http.ResponseWriter, r *http.Request, location, kind, message string) {
	if sess := internalShared.SessionFromContext(r.Context()); sess != nil {
		sess.AddFlash(internalShared.FlashMessage{Kind: kind, Message: message})
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}
