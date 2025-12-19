package masterdata

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// Handler manages master data endpoints.
type Handler struct {
	logger    *slog.Logger
	service   Service
	templates *view.Engine
	csrf      *shared.CSRFManager
	sessions  *shared.SessionManager
	rbac      rbac.Middleware
}

// NewHandler builds Handler instance.
func NewHandler(logger *slog.Logger, service Service, templates *view.Engine, csrf *shared.CSRFManager, sessions *shared.SessionManager, rbac rbac.Middleware) *Handler {
	return &Handler{logger: logger, service: service, templates: templates, csrf: csrf, sessions: sessions, rbac: rbac}
}

// MountRoutes registers master data routes.
func (h *Handler) MountRoutes(r chi.Router) {
	// Companies
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("master.view"))
		r.Get("/companies", h.listCompanies)
		r.Get("/companies/{id}", h.showCompany)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("master.edit"))
		r.Get("/companies/new", h.showCompanyForm)
		r.Post("/companies", h.createCompany)
		r.Get("/companies/{id}/edit", h.showEditCompanyForm)
		r.Post("/companies/{id}/edit", h.updateCompany)
		r.Post("/companies/{id}/delete", h.deleteCompany)
	})

	// Branches
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("master.view"))
		r.Get("/branches", h.listBranches)
		r.Get("/branches/{id}", h.showBranch)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("master.edit"))
		r.Get("/branches/new", h.showBranchForm)
		r.Post("/branches", h.createBranch)
		r.Get("/branches/{id}/edit", h.showEditBranchForm)
		r.Post("/branches/{id}/edit", h.updateBranch)
		r.Post("/branches/{id}/delete", h.deleteBranch)
	})

	// Warehouses
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("master.view"))
		r.Get("/warehouses", h.listWarehouses)
		r.Get("/warehouses/{id}", h.showWarehouse)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("master.edit"))
		r.Get("/warehouses/new", h.showWarehouseForm)
		r.Post("/warehouses", h.createWarehouse)
		r.Get("/warehouses/{id}/edit", h.showEditWarehouseForm)
		r.Post("/warehouses/{id}/edit", h.updateWarehouse)
		r.Post("/warehouses/{id}/delete", h.deleteWarehouse)
	})

	// Units
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("master.view"))
		r.Get("/units", h.listUnits)
		r.Get("/units/{id}", h.showUnit)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("master.edit"))
		r.Get("/units/new", h.showUnitForm)
		r.Post("/units", h.createUnit)
		r.Get("/units/{id}/edit", h.showEditUnitForm)
		r.Post("/units/{id}/edit", h.updateUnit)
		r.Post("/units/{id}/delete", h.deleteUnit)
	})

	// Taxes
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("master.view"))
		r.Get("/taxes", h.listTaxes)
		r.Get("/taxes/{id}", h.showTax)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("master.edit"))
		r.Get("/taxes/new", h.showTaxForm)
		r.Post("/taxes", h.createTax)
		r.Get("/taxes/{id}/edit", h.showEditTaxForm)
		r.Post("/taxes/{id}/edit", h.updateTax)
		r.Post("/taxes/{id}/delete", h.deleteTax)
	})

	// Categories
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("master.view"))
		r.Get("/categories", h.listCategories)
		r.Get("/categories/{id}", h.showCategory)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("master.edit"))
		r.Get("/categories/new", h.showCategoryForm)
		r.Post("/categories", h.createCategory)
		r.Get("/categories/{id}/edit", h.showEditCategoryForm)
		r.Post("/categories/{id}/edit", h.updateCategory)
		r.Post("/categories/{id}/delete", h.deleteCategory)
	})

	// Suppliers
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("master.view"))
		r.Get("/suppliers", h.listSuppliers)
		r.Get("/suppliers/{id}", h.showSupplier)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("master.edit"))
		r.Get("/suppliers/new", h.showSupplierForm)
		r.Post("/suppliers", h.createSupplier)
		r.Get("/suppliers/{id}/edit", h.showEditSupplierForm)
		r.Post("/suppliers/{id}/edit", h.updateSupplier)
		r.Post("/suppliers/{id}/delete", h.deleteSupplier)
	})

	// Products
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("master.view"))
		r.Get("/products", h.listProducts)
		r.Get("/products/{id}", h.showProduct)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("master.edit"))
		r.Get("/products/new", h.showProductForm)
		r.Post("/products", h.createProduct)
		r.Get("/products/{id}/edit", h.showEditProductForm)
		r.Post("/products/{id}/edit", h.updateProduct)
		r.Post("/products/{id}/delete", h.deleteProduct)
	})
}

// ============================================================================
// COMPANY HANDLERS
// ============================================================================

func (h *Handler) listCompanies(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 10
	}
	filters := ListFilters{
		Page:    page,
		Limit:   limit,
		Search:  r.URL.Query().Get("search"),
		SortBy:  r.URL.Query().Get("sort"),
		SortDir: r.URL.Query().Get("dir"),
	}

	companies, total, err := h.service.ListCompanies(r.Context(), filters)
	if err != nil {
		h.logger.Error("list companies failed", "error", err)
		http.Error(w, "Failed to load companies", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/masterdata/companies_list.html", map[string]any{
		"Companies": companies,
		"Filters":   filters,
		"Total":     total,
	}, http.StatusOK)
}

func (h *Handler) showCompany(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid company ID", http.StatusBadRequest)
		return
	}

	company, err := h.service.GetCompany(r.Context(), id)
	if err != nil {
		h.logger.Error("get company failed", "error", err, "id", id)
		http.Error(w, "Company not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/masterdata/company_detail.html", map[string]any{
		"Company": company,
	}, http.StatusOK)
}

func (h *Handler) showCompanyForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/masterdata/company_form.html", map[string]any{
		"Errors":  formErrors{},
		"Company": nil,
	}, http.StatusOK)
}

func (h *Handler) createCompany(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	company := Company{
		Code:    r.PostFormValue("code"),
		Name:    r.PostFormValue("name"),
		Address: r.PostFormValue("address"),
		TaxID:   r.PostFormValue("tax_id"),
	}

	created, err := h.service.CreateCompany(r.Context(), company)
	if err != nil {
		h.render(w, r, "pages/masterdata/company_form.html", map[string]any{
			"Errors":  formErrors{"general": err.Error()},
			"Company": nil,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/companies/"+strconv.FormatInt(created.ID, 10), "success", "Company created successfully")
}

func (h *Handler) showEditCompanyForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid company ID", http.StatusBadRequest)
		return
	}

	company, err := h.service.GetCompany(r.Context(), id)
	if err != nil {
		h.logger.Error("get company failed", "error", err, "id", id)
		http.Error(w, "Company not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/masterdata/company_form.html", map[string]any{
		"Errors":  formErrors{},
		"Company": company,
	}, http.StatusOK)
}

func (h *Handler) updateCompany(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid company ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	company := Company{
		Code:    r.PostFormValue("code"),
		Name:    r.PostFormValue("name"),
		Address: r.PostFormValue("address"),
		TaxID:   r.PostFormValue("tax_id"),
	}

	err = h.service.UpdateCompany(r.Context(), id, company)
	if err != nil {
		h.render(w, r, "pages/masterdata/company_form.html", map[string]any{
			"Errors":  formErrors{"general": err.Error()},
			"Company": company,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/companies/"+strconv.FormatInt(id, 10), "success", "Company updated successfully")
}

func (h *Handler) deleteCompany(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid company ID", http.StatusBadRequest)
		return
	}

	err = h.service.DeleteCompany(r.Context(), id)
	if err != nil {
		h.redirectWithFlash(w, r, "/masterdata/companies", "error", err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/companies", "success", "Company deleted successfully")
}

// ============================================================================
// BRANCH HANDLERS
// ============================================================================

func (h *Handler) listBranches(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 10
	}

	filters := ListFilters{
		Page:    page,
		Limit:   limit,
		Search:  r.URL.Query().Get("search"),
		SortBy:  r.URL.Query().Get("sort"),
		SortDir: r.URL.Query().Get("dir"),
	}

	companyIDStr := r.URL.Query().Get("company_id")
	if companyIDStr != "" {
		if parsed, err := strconv.ParseInt(companyIDStr, 10, 64); err == nil {
			filters.CompanyID = &parsed
		}
	}

	branches, total, err := h.service.ListBranches(r.Context(), filters)
	if err != nil {
		h.logger.Error("list branches failed", "error", err)
		http.Error(w, "Failed to load branches", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/masterdata/branches_list.html", map[string]any{
		"Branches": branches,
		"Filters":  filters,
		"Total":    total,
	}, http.StatusOK)
}

func (h *Handler) showBranch(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid branch ID", http.StatusBadRequest)
		return
	}

	branch, err := h.service.GetBranch(r.Context(), id)
	if err != nil {
		h.logger.Error("get branch failed", "error", err, "id", id)
		http.Error(w, "Branch not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/masterdata/branch_detail.html", map[string]any{
		"Branch": branch,
	}, http.StatusOK)
}

func (h *Handler) showBranchForm(w http.ResponseWriter, r *http.Request) {
	companies, _, err := h.service.ListCompanies(r.Context(), ListFilters{})
	if err != nil {
		h.logger.Error("list companies failed", "error", err)
		companies = []Company{}
	}

	h.render(w, r, "pages/masterdata/branch_form.html", map[string]any{
		"Errors":    formErrors{},
		"Branch":    nil,
		"Companies": companies,
	}, http.StatusOK)
}

func (h *Handler) createBranch(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	companyID, _ := strconv.ParseInt(r.PostFormValue("company_id"), 10, 64)
	branch := Branch{
		CompanyID: companyID,
		Code:      r.PostFormValue("code"),
		Name:      r.PostFormValue("name"),
		Address:   r.PostFormValue("address"),
	}

	created, err := h.service.CreateBranch(r.Context(), branch)
	if err != nil {
		companies, _, _ := h.service.ListCompanies(r.Context(), ListFilters{})
		h.render(w, r, "pages/masterdata/branch_form.html", map[string]any{
			"Errors":    formErrors{"general": err.Error()},
			"Branch":    nil,
			"Companies": companies,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/branches/"+strconv.FormatInt(created.ID, 10), "success", "Branch created successfully")
}

func (h *Handler) showEditBranchForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid branch ID", http.StatusBadRequest)
		return
	}

	branch, err := h.service.GetBranch(r.Context(), id)
	if err != nil {
		h.logger.Error("get branch failed", "error", err, "id", id)
		http.Error(w, "Branch not found", http.StatusNotFound)
		return
	}

	companies, _, err := h.service.ListCompanies(r.Context(), ListFilters{})
	if err != nil {
		h.logger.Error("list companies failed", "error", err)
		companies = []Company{}
	}

	h.render(w, r, "pages/masterdata/branch_form.html", map[string]any{
		"Errors":    formErrors{},
		"Branch":    branch,
		"Companies": companies,
	}, http.StatusOK)
}

func (h *Handler) updateBranch(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid branch ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	companyID, _ := strconv.ParseInt(r.PostFormValue("company_id"), 10, 64)
	branch := Branch{
		CompanyID: companyID,
		Code:      r.PostFormValue("code"),
		Name:      r.PostFormValue("name"),
		Address:   r.PostFormValue("address"),
	}

	err = h.service.UpdateBranch(r.Context(), id, branch)
	if err != nil {
		companies, _, _ := h.service.ListCompanies(r.Context(), ListFilters{})
		h.render(w, r, "pages/masterdata/branch_form.html", map[string]any{
			"Errors":    formErrors{"general": err.Error()},
			"Branch":    branch,
			"Companies": companies,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/branches/"+strconv.FormatInt(id, 10), "success", "Branch updated successfully")
}

func (h *Handler) deleteBranch(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid branch ID", http.StatusBadRequest)
		return
	}

	err = h.service.DeleteBranch(r.Context(), id)
	if err != nil {
		h.redirectWithFlash(w, r, "/masterdata/branches", "error", err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/branches", "success", "Branch deleted successfully")
}

// ============================================================================
// WAREHOUSE HANDLERS
// ============================================================================

func (h *Handler) listWarehouses(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 10
	}

	filters := ListFilters{
		Page:    page,
		Limit:   limit,
		Search:  r.URL.Query().Get("search"),
		SortBy:  r.URL.Query().Get("sort"),
		SortDir: r.URL.Query().Get("dir"),
	}

	branchIDStr := r.URL.Query().Get("branch_id")
	if branchIDStr != "" {
		if parsed, err := strconv.ParseInt(branchIDStr, 10, 64); err == nil {
			filters.BranchID = &parsed
		}
	}

	warehouses, total, err := h.service.ListWarehouses(r.Context(), filters)
	if err != nil {
		h.logger.Error("list warehouses failed", "error", err)
		http.Error(w, "Failed to load warehouses", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/masterdata/warehouses_list.html", map[string]any{
		"Warehouses": warehouses,
		"Filters":    filters,
		"Total":      total,
	}, http.StatusOK)
}

func (h *Handler) showWarehouse(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid warehouse ID", http.StatusBadRequest)
		return
	}

	warehouse, err := h.service.GetWarehouse(r.Context(), id)
	if err != nil {
		h.logger.Error("get warehouse failed", "error", err, "id", id)
		http.Error(w, "Warehouse not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/masterdata/warehouse_detail.html", map[string]any{
		"Warehouse": warehouse,
	}, http.StatusOK)
}

func (h *Handler) showWarehouseForm(w http.ResponseWriter, r *http.Request) {
	branches, _, err := h.service.ListBranches(r.Context(), ListFilters{})
	if err != nil {
		h.logger.Error("list branches failed", "error", err)
		branches = []Branch{}
	}

	h.render(w, r, "pages/masterdata/warehouse_form.html", map[string]any{
		"Errors":    formErrors{},
		"Warehouse": nil,
		"Branches":  branches,
	}, http.StatusOK)
}

func (h *Handler) createWarehouse(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	branchID, _ := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	warehouse := Warehouse{
		BranchID: branchID,
		Code:     r.PostFormValue("code"),
		Name:     r.PostFormValue("name"),
		Address:  r.PostFormValue("address"),
	}

	created, err := h.service.CreateWarehouse(r.Context(), warehouse)
	if err != nil {
		branches, _, _ := h.service.ListBranches(r.Context(), ListFilters{})
		h.render(w, r, "pages/masterdata/warehouse_form.html", map[string]any{
			"Errors":    formErrors{"general": err.Error()},
			"Warehouse": nil,
			"Branches":  branches,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/warehouses/"+strconv.FormatInt(created.ID, 10), "success", "Warehouse created successfully")
}

func (h *Handler) showEditWarehouseForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid warehouse ID", http.StatusBadRequest)
		return
	}

	warehouse, err := h.service.GetWarehouse(r.Context(), id)
	if err != nil {
		h.logger.Error("get warehouse failed", "error", err, "id", id)
		http.Error(w, "Warehouse not found", http.StatusNotFound)
		return
	}

	branches, _, err := h.service.ListBranches(r.Context(), ListFilters{})
	if err != nil {
		h.logger.Error("list branches failed", "error", err)
		branches = []Branch{}
	}

	h.render(w, r, "pages/masterdata/warehouse_form.html", map[string]any{
		"Errors":    formErrors{},
		"Warehouse": warehouse,
		"Branches":  branches,
	}, http.StatusOK)
}

func (h *Handler) updateWarehouse(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid warehouse ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	branchID, _ := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	warehouse := Warehouse{
		BranchID: branchID,
		Code:     r.PostFormValue("code"),
		Name:     r.PostFormValue("name"),
		Address:  r.PostFormValue("address"),
	}

	err = h.service.UpdateWarehouse(r.Context(), id, warehouse)
	if err != nil {
		branches, _, _ := h.service.ListBranches(r.Context(), ListFilters{})
		h.render(w, r, "pages/masterdata/warehouse_form.html", map[string]any{
			"Errors":    formErrors{"general": err.Error()},
			"Warehouse": warehouse,
			"Branches":  branches,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/warehouses/"+strconv.FormatInt(id, 10), "success", "Warehouse updated successfully")
}

func (h *Handler) deleteWarehouse(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid warehouse ID", http.StatusBadRequest)
		return
	}

	err = h.service.DeleteWarehouse(r.Context(), id)
	if err != nil {
		h.redirectWithFlash(w, r, "/masterdata/warehouses", "error", err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/warehouses", "success", "Warehouse deleted successfully")
}

// ============================================================================
// UNIT HANDLERS
// ============================================================================

func (h *Handler) listUnits(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 10
	}

	filters := ListFilters{
		Page:    page,
		Limit:   limit,
		Search:  r.URL.Query().Get("search"),
		SortBy:  r.URL.Query().Get("sort"),
		SortDir: r.URL.Query().Get("dir"),
	}

	units, total, err := h.service.ListUnits(r.Context(), filters)
	if err != nil {
		h.logger.Error("list units failed", "error", err)
		http.Error(w, "Failed to load units", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/masterdata/units_list.html", map[string]any{
		"Units":   units,
		"Filters": filters,
		"Total":   total,
	}, http.StatusOK)
}

func (h *Handler) showUnit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid unit ID", http.StatusBadRequest)
		return
	}

	unit, err := h.service.GetUnit(r.Context(), id)
	if err != nil {
		h.logger.Error("get unit failed", "error", err, "id", id)
		http.Error(w, "Unit not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/masterdata/unit_detail.html", map[string]any{
		"Unit": unit,
	}, http.StatusOK)
}

func (h *Handler) showUnitForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/masterdata/unit_form.html", map[string]any{
		"Errors": formErrors{},
		"Unit":   nil,
	}, http.StatusOK)
}

func (h *Handler) createUnit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	unit := Unit{
		Code: r.PostFormValue("code"),
		Name: r.PostFormValue("name"),
	}

	created, err := h.service.CreateUnit(r.Context(), unit)
	if err != nil {
		h.render(w, r, "pages/masterdata/unit_form.html", map[string]any{
			"Errors": formErrors{"general": err.Error()},
			"Unit":   nil,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/units/"+strconv.FormatInt(created.ID, 10), "success", "Unit created successfully")
}

func (h *Handler) showEditUnitForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid unit ID", http.StatusBadRequest)
		return
	}

	unit, err := h.service.GetUnit(r.Context(), id)
	if err != nil {
		h.logger.Error("get unit failed", "error", err, "id", id)
		http.Error(w, "Unit not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/masterdata/unit_form.html", map[string]any{
		"Errors": formErrors{},
		"Unit":   unit,
	}, http.StatusOK)
}

func (h *Handler) updateUnit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid unit ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	unit := Unit{
		Code: r.PostFormValue("code"),
		Name: r.PostFormValue("name"),
	}

	err = h.service.UpdateUnit(r.Context(), id, unit)
	if err != nil {
		h.render(w, r, "pages/masterdata/unit_form.html", map[string]any{
			"Errors": formErrors{"general": err.Error()},
			"Unit":   unit,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/units/"+strconv.FormatInt(id, 10), "success", "Unit updated successfully")
}

func (h *Handler) deleteUnit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid unit ID", http.StatusBadRequest)
		return
	}

	err = h.service.DeleteUnit(r.Context(), id)
	if err != nil {
		h.redirectWithFlash(w, r, "/masterdata/units", "error", err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/units", "success", "Unit deleted successfully")
}

// ============================================================================
// TAX HANDLERS
// ============================================================================

func (h *Handler) listTaxes(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 10
	}

	filters := ListFilters{
		Page:    page,
		Limit:   limit,
		Search:  r.URL.Query().Get("search"),
		SortBy:  r.URL.Query().Get("sort"),
		SortDir: r.URL.Query().Get("dir"),
	}

	taxes, total, err := h.service.ListTaxes(r.Context(), filters)
	if err != nil {
		h.logger.Error("list taxes failed", "error", err)
		http.Error(w, "Failed to load taxes", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/masterdata/taxes_list.html", map[string]any{
		"Taxes":   taxes,
		"Filters": filters,
		"Total":   total,
	}, http.StatusOK)
}

func (h *Handler) showTax(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid tax ID", http.StatusBadRequest)
		return
	}

	tax, err := h.service.GetTax(r.Context(), id)
	if err != nil {
		h.logger.Error("get tax failed", "error", err, "id", id)
		http.Error(w, "Tax not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/masterdata/tax_detail.html", map[string]any{
		"Tax": tax,
	}, http.StatusOK)
}

func (h *Handler) showTaxForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/masterdata/tax_form.html", map[string]any{
		"Errors": formErrors{},
		"Tax":    nil,
	}, http.StatusOK)
}

func (h *Handler) createTax(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	rate, _ := strconv.ParseFloat(r.PostFormValue("rate"), 64)
	tax := Tax{
		Code: r.PostFormValue("code"),
		Name: r.PostFormValue("name"),
		Rate: rate,
	}

	created, err := h.service.CreateTax(r.Context(), tax)
	if err != nil {
		h.render(w, r, "pages/masterdata/tax_form.html", map[string]any{
			"Errors": formErrors{"general": err.Error()},
			"Tax":    nil,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/taxes/"+strconv.FormatInt(created.ID, 10), "success", "Tax created successfully")
}

func (h *Handler) showEditTaxForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid tax ID", http.StatusBadRequest)
		return
	}

	tax, err := h.service.GetTax(r.Context(), id)
	if err != nil {
		h.logger.Error("get tax failed", "error", err, "id", id)
		http.Error(w, "Tax not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/masterdata/tax_form.html", map[string]any{
		"Errors": formErrors{},
		"Tax":    tax,
	}, http.StatusOK)
}

func (h *Handler) updateTax(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid tax ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	rate, _ := strconv.ParseFloat(r.PostFormValue("rate"), 64)
	tax := Tax{
		Code: r.PostFormValue("code"),
		Name: r.PostFormValue("name"),
		Rate: rate,
	}

	err = h.service.UpdateTax(r.Context(), id, tax)
	if err != nil {
		h.render(w, r, "pages/masterdata/tax_form.html", map[string]any{
			"Errors": formErrors{"general": err.Error()},
			"Tax":    tax,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/taxes/"+strconv.FormatInt(id, 10), "success", "Tax updated successfully")
}

func (h *Handler) deleteTax(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid tax ID", http.StatusBadRequest)
		return
	}

	err = h.service.DeleteTax(r.Context(), id)
	if err != nil {
		h.redirectWithFlash(w, r, "/masterdata/taxes", "error", err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/taxes", "success", "Tax deleted successfully")
}

// ============================================================================
// CATEGORY HANDLERS
// ============================================================================

func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 10
	}

	filters := ListFilters{
		Page:    page,
		Limit:   limit,
		Search:  r.URL.Query().Get("search"),
		SortBy:  r.URL.Query().Get("sort"),
		SortDir: r.URL.Query().Get("dir"),
	}

	isActiveStr := r.URL.Query().Get("is_active")
	if isActiveStr != "" {
		val := isActiveStr == "true"
		filters.IsActive = &val
	}

	categories, total, err := h.service.ListCategories(r.Context(), filters)
	if err != nil {
		h.logger.Error("list categories failed", "error", err)
		http.Error(w, "Failed to load categories", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/masterdata/categories_list.html", map[string]any{
		"Categories": categories,
		"Filters":    filters,
		"Total":      total,
	}, http.StatusOK)
}

func (h *Handler) showCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid category ID", http.StatusBadRequest)
		return
	}

	category, err := h.service.GetCategory(r.Context(), id)
	if err != nil {
		h.logger.Error("get category failed", "error", err, "id", id)
		http.Error(w, "Category not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/masterdata/category_detail.html", map[string]any{
		"Category": category,
	}, http.StatusOK)
}

func (h *Handler) showCategoryForm(w http.ResponseWriter, r *http.Request) {
	categories, _, err := h.service.ListCategories(r.Context(), ListFilters{})
	if err != nil {
		h.logger.Error("list categories failed", "error", err)
		categories = []Category{}
	}

	h.render(w, r, "pages/masterdata/category_form.html", map[string]any{
		"Errors":     formErrors{},
		"Category":   nil,
		"Categories": categories,
	}, http.StatusOK)
}

func (h *Handler) createCategory(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	var parentID *int64
	if pid := r.PostFormValue("parent_id"); pid != "" {
		if parsed, err := strconv.ParseInt(pid, 10, 64); err == nil {
			parentID = &parsed
		}
	}

	category := Category{
		Code:     r.PostFormValue("code"),
		Name:     r.PostFormValue("name"),
		ParentID: parentID,
	}

	created, err := h.service.CreateCategory(r.Context(), category)
	if err != nil {
		categories, _, _ := h.service.ListCategories(r.Context(), ListFilters{})
		h.render(w, r, "pages/masterdata/category_form.html", map[string]any{
			"Errors":     formErrors{"general": err.Error()},
			"Category":   nil,
			"Categories": categories,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/categories/"+strconv.FormatInt(created.ID, 10), "success", "Category created successfully")
}

func (h *Handler) showEditCategoryForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid category ID", http.StatusBadRequest)
		return
	}

	category, err := h.service.GetCategory(r.Context(), id)
	if err != nil {
		h.logger.Error("get category failed", "error", err, "id", id)
		http.Error(w, "Category not found", http.StatusNotFound)
		return
	}

	categories, _, err := h.service.ListCategories(r.Context(), ListFilters{})
	if err != nil {
		h.logger.Error("list categories failed", "error", err)
		categories = []Category{}
	}

	h.render(w, r, "pages/masterdata/category_form.html", map[string]any{
		"Errors":     formErrors{},
		"Category":   category,
		"Categories": categories,
	}, http.StatusOK)
}

func (h *Handler) updateCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid category ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	var parentID *int64
	if pid := r.PostFormValue("parent_id"); pid != "" {
		if parsed, err := strconv.ParseInt(pid, 10, 64); err == nil {
			parentID = &parsed
		}
	}

	category := Category{
		Code:     r.PostFormValue("code"),
		Name:     r.PostFormValue("name"),
		ParentID: parentID,
	}

	err = h.service.UpdateCategory(r.Context(), id, category)
	if err != nil {
		categories, _, _ := h.service.ListCategories(r.Context(), ListFilters{})
		h.render(w, r, "pages/masterdata/category_form.html", map[string]any{
			"Errors":     formErrors{"general": err.Error()},
			"Category":   category,
			"Categories": categories,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/categories/"+strconv.FormatInt(id, 10), "success", "Category updated successfully")
}

func (h *Handler) deleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid category ID", http.StatusBadRequest)
		return
	}

	err = h.service.DeleteCategory(r.Context(), id)
	if err != nil {
		h.redirectWithFlash(w, r, "/masterdata/categories", "error", err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/categories", "success", "Category deleted successfully")
}

// ============================================================================
// SUPPLIER HANDLERS
// ============================================================================

func (h *Handler) listSuppliers(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 10
	}

	filters := ListFilters{
		Page:    page,
		Limit:   limit,
		Search:  r.URL.Query().Get("search"),
		SortBy:  r.URL.Query().Get("sort"),
		SortDir: r.URL.Query().Get("dir"),
	}

	isActiveStr := r.URL.Query().Get("is_active")
	if isActiveStr != "" {
		val := isActiveStr == "true"
		filters.IsActive = &val
	}

	suppliers, total, err := h.service.ListSuppliers(r.Context(), filters)
	if err != nil {
		h.logger.Error("list suppliers failed", "error", err)
		http.Error(w, "Failed to load suppliers", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/masterdata/suppliers_list.html", map[string]any{
		"Suppliers": suppliers,
		"Filters":   filters,
		"Total":     total,
	}, http.StatusOK)
}

func (h *Handler) showSupplier(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid supplier ID", http.StatusBadRequest)
		return
	}

	supplier, err := h.service.GetSupplier(r.Context(), id)
	if err != nil {
		h.logger.Error("get supplier failed", "error", err, "id", id)
		http.Error(w, "Supplier not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/masterdata/supplier_detail.html", map[string]any{
		"Supplier": supplier,
	}, http.StatusOK)
}

func (h *Handler) showSupplierForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/masterdata/supplier_form.html", map[string]any{
		"Errors":   formErrors{},
		"Supplier": nil,
	}, http.StatusOK)
}

func (h *Handler) createSupplier(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	supplier := Supplier{
		Code:     r.PostFormValue("code"),
		Name:     r.PostFormValue("name"),
		Phone:    r.PostFormValue("phone"),
		Email:    r.PostFormValue("email"),
		Address:  r.PostFormValue("address"),
		IsActive: r.PostFormValue("is_active") == "true",
	}

	created, err := h.service.CreateSupplier(r.Context(), supplier)
	if err != nil {
		h.render(w, r, "pages/masterdata/supplier_form.html", map[string]any{
			"Errors":   formErrors{"general": err.Error()},
			"Supplier": nil,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/suppliers/"+strconv.FormatInt(created.ID, 10), "success", "Supplier created successfully")
}

func (h *Handler) showEditSupplierForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid supplier ID", http.StatusBadRequest)
		return
	}

	supplier, err := h.service.GetSupplier(r.Context(), id)
	if err != nil {
		h.logger.Error("get supplier failed", "error", err, "id", id)
		http.Error(w, "Supplier not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/masterdata/supplier_form.html", map[string]any{
		"Errors":   formErrors{},
		"Supplier": supplier,
	}, http.StatusOK)
}

func (h *Handler) updateSupplier(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid supplier ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	supplier := Supplier{
		Code:     r.PostFormValue("code"),
		Name:     r.PostFormValue("name"),
		Phone:    r.PostFormValue("phone"),
		Email:    r.PostFormValue("email"),
		Address:  r.PostFormValue("address"),
		IsActive: r.PostFormValue("is_active") == "true",
	}

	err = h.service.UpdateSupplier(r.Context(), id, supplier)
	if err != nil {
		h.render(w, r, "pages/masterdata/supplier_form.html", map[string]any{
			"Errors":   formErrors{"general": err.Error()},
			"Supplier": supplier,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/suppliers/"+strconv.FormatInt(id, 10), "success", "Supplier updated successfully")
}

func (h *Handler) deleteSupplier(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid supplier ID", http.StatusBadRequest)
		return
	}

	err = h.service.DeleteSupplier(r.Context(), id)
	if err != nil {
		h.redirectWithFlash(w, r, "/masterdata/suppliers", "error", err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/suppliers", "success", "Supplier deleted successfully")
}

// ============================================================================
// PRODUCT HANDLERS
// ============================================================================

func (h *Handler) listProducts(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 10
	}

	filters := ListFilters{
		Page:    page,
		Limit:   limit,
		Search:  r.URL.Query().Get("search"),
		SortBy:  r.URL.Query().Get("sort"),
		SortDir: r.URL.Query().Get("dir"),
	}

	categoryIDStr := r.URL.Query().Get("category_id")
	if categoryIDStr != "" {
		if parsed, err := strconv.ParseInt(categoryIDStr, 10, 64); err == nil {
			filters.CategoryID = &parsed
		}
	}

	isActiveStr := r.URL.Query().Get("is_active")
	if isActiveStr != "" {
		val := isActiveStr == "true"
		filters.IsActive = &val
	}

	products, total, err := h.service.ListProducts(r.Context(), filters)
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

func (h *Handler) showProduct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid product ID", http.StatusBadRequest)
		return
	}

	product, err := h.service.GetProduct(r.Context(), id)
	if err != nil {
		h.logger.Error("get product failed", "error", err, "id", id)
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/masterdata/product_detail.html", map[string]any{
		"Product": product,
	}, http.StatusOK)
}

func (h *Handler) showProductForm(w http.ResponseWriter, r *http.Request) {
	categories, _, err := h.service.ListCategories(r.Context(), ListFilters{})
	if err != nil {
		h.logger.Error("list categories failed", "error", err)
		categories = []Category{}
	}

	units, _, err := h.service.ListUnits(r.Context(), ListFilters{})
	if err != nil {
		h.logger.Error("list units failed", "error", err)
		units = []Unit{}
	}

	taxes, _, err := h.service.ListTaxes(r.Context(), ListFilters{})
	if err != nil {
		h.logger.Error("list taxes failed", "error", err)
		taxes = []Tax{}
	}

	h.render(w, r, "pages/masterdata/product_form.html", map[string]any{
		"Errors":     formErrors{},
		"Product":    nil,
		"Categories": categories,
		"Units":      units,
		"Taxes":      taxes,
	}, http.StatusOK)
}

func (h *Handler) createProduct(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	categoryID, _ := strconv.ParseInt(r.PostFormValue("category_id"), 10, 64)
	unitID, _ := strconv.ParseInt(r.PostFormValue("unit_id"), 10, 64)
	price, _ := strconv.ParseFloat(r.PostFormValue("price"), 64)

	var taxID *int64
	if tid := r.PostFormValue("tax_id"); tid != "" {
		if parsed, err := strconv.ParseInt(tid, 10, 64); err == nil {
			taxID = &parsed
		}
	}

	product := Product{
		SKU:        r.PostFormValue("sku"),
		Name:       r.PostFormValue("name"),
		CategoryID: categoryID,
		UnitID:     unitID,
		Price:      price,
		TaxID:      taxID,
		IsActive:   r.PostFormValue("is_active") == "true",
	}

	created, err := h.service.CreateProduct(r.Context(), product)
	if err != nil {
		categories, _, _ := h.service.ListCategories(r.Context(), ListFilters{})
		units, _, _ := h.service.ListUnits(r.Context(), ListFilters{})
		taxes, _, _ := h.service.ListTaxes(r.Context(), ListFilters{})
		h.render(w, r, "pages/masterdata/product_form.html", map[string]any{
			"Errors":     formErrors{"general": err.Error()},
			"Product":    nil,
			"Categories": categories,
			"Units":      units,
			"Taxes":      taxes,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/products/"+strconv.FormatInt(created.ID, 10), "success", "Product created successfully")
}

func (h *Handler) showEditProductForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid product ID", http.StatusBadRequest)
		return
	}

	product, err := h.service.GetProduct(r.Context(), id)
	if err != nil {
		h.logger.Error("get product failed", "error", err, "id", id)
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}

	categories, _, err := h.service.ListCategories(r.Context(), ListFilters{})
	if err != nil {
		h.logger.Error("list categories failed", "error", err)
		categories = []Category{}
	}

	units, _, err := h.service.ListUnits(r.Context(), ListFilters{})
	if err != nil {
		h.logger.Error("list units failed", "error", err)
		units = []Unit{}
	}

	taxes, _, err := h.service.ListTaxes(r.Context(), ListFilters{})
	if err != nil {
		h.logger.Error("list taxes failed", "error", err)
		taxes = []Tax{}
	}

	h.render(w, r, "pages/masterdata/product_form.html", map[string]any{
		"Errors":     formErrors{},
		"Product":    product,
		"Categories": categories,
		"Units":      units,
		"Taxes":      taxes,
	}, http.StatusOK)
}

func (h *Handler) updateProduct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid product ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	categoryID, _ := strconv.ParseInt(r.PostFormValue("category_id"), 10, 64)
	unitID, _ := strconv.ParseInt(r.PostFormValue("unit_id"), 10, 64)
	price, _ := strconv.ParseFloat(r.PostFormValue("price"), 64)

	var taxID *int64
	if tid := r.PostFormValue("tax_id"); tid != "" {
		if parsed, err := strconv.ParseInt(tid, 10, 64); err == nil {
			taxID = &parsed
		}
	}

	product := Product{
		SKU:        r.PostFormValue("sku"),
		Name:       r.PostFormValue("name"),
		CategoryID: categoryID,
		UnitID:     unitID,
		Price:      price,
		TaxID:      taxID,
		IsActive:   r.PostFormValue("is_active") == "true",
	}

	err = h.service.UpdateProduct(r.Context(), id, product)
	if err != nil {
		categories, _, _ := h.service.ListCategories(r.Context(), ListFilters{})
		units, _, _ := h.service.ListUnits(r.Context(), ListFilters{})
		taxes, _, _ := h.service.ListTaxes(r.Context(), ListFilters{})
		h.render(w, r, "pages/masterdata/product_form.html", map[string]any{
			"Errors":     formErrors{"general": err.Error()},
			"Product":    product,
			"Categories": categories,
			"Units":      units,
			"Taxes":      taxes,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/products/"+strconv.FormatInt(id, 10), "success", "Product updated successfully")
}

func (h *Handler) deleteProduct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid product ID", http.StatusBadRequest)
		return
	}

	err = h.service.DeleteProduct(r.Context(), id)
	if err != nil {
		h.redirectWithFlash(w, r, "/masterdata/products", "error", err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/products", "success", "Product deleted successfully")
}

// ============================================================================
// HELPER METHODS
// ============================================================================

type formErrors map[string]string

func (h *Handler) render(w http.ResponseWriter, r *http.Request, template string, data map[string]any, status int) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	var flash *shared.FlashMessage
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
	if sess := shared.SessionFromContext(r.Context()); sess != nil {
		sess.AddFlash(shared.FlashMessage{Kind: kind, Message: message})
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}
