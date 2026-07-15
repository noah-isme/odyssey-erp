package view_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/branches"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/categories"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/companies"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/products"
	masterdatashared "github.com/odyssey-erp/odyssey-erp/internal/masterdata/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/suppliers"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/taxes"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/units"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/warehouses"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

func TestManagementTemplatesRender(t *testing.T) {
	engine, err := view.NewEngine()
	require.NoError(t, err)

	now := time.Now()
	company := companies.Company{ID: 1, Code: "COMP", Name: "Company", CreatedAt: now, UpdatedAt: now}
	branch := branches.Branch{ID: 2, CompanyID: company.ID, Code: "BR", Name: "Branch", CreatedAt: now, UpdatedAt: now}
	category := categories.Category{ID: 3, Code: "CAT", Name: "Category"}
	unit := units.Unit{ID: 4, Code: "EA", Name: "Each"}
	tax := taxes.Tax{ID: 5, Code: "VAT", Name: "VAT", Rate: 11}
	product := products.Product{ID: 6, Code: "SKU", Name: "Product", CategoryID: category.ID, UnitID: unit.ID, TaxID: tax.ID, IsActive: true}
	supplier := suppliers.Supplier{ID: 7, Code: "SUP", Name: "Supplier", IsActive: true}
	warehouse := warehouses.Warehouse{ID: 8, BranchID: branch.ID, Code: "WH", Name: "Warehouse", CreatedAt: now, UpdatedAt: now}

	tests := []struct {
		name string
		data map[string]any
	}{
		{"pages/masterdata/branch_detail.html", map[string]any{"Branch": branch}},
		{"pages/masterdata/branch_form.html", map[string]any{"Branch": branch, "Companies": []companies.Company{company}, "Errors": map[string]string{}}},
		{"pages/masterdata/category_detail.html", map[string]any{"Category": category}},
		{"pages/masterdata/category_form.html", map[string]any{"Category": category, "Errors": map[string]string{}}},
		{"pages/masterdata/company_detail.html", map[string]any{"Company": company}},
		{"pages/masterdata/company_form.html", map[string]any{"Company": company, "Errors": map[string]string{}}},
		{"pages/masterdata/product_detail.html", map[string]any{"Product": product}},
		{"pages/masterdata/product_form.html", map[string]any{"Product": product, "Categories": []categories.Category{category}, "Units": []units.Unit{unit}, "Taxes": []taxes.Tax{tax}, "Errors": map[string]string{}}},
		{"pages/masterdata/supplier_detail.html", map[string]any{"Supplier": supplier}},
		{"pages/masterdata/supplier_form.html", map[string]any{"Supplier": supplier, "Errors": map[string]string{}}},
		{"pages/masterdata/tax_detail.html", map[string]any{"Tax": tax}},
		{"pages/masterdata/tax_form.html", map[string]any{"Tax": tax, "Errors": map[string]string{}}},
		{"pages/masterdata/unit_detail.html", map[string]any{"Unit": unit}},
		{"pages/masterdata/unit_form.html", map[string]any{"Unit": unit, "Errors": map[string]string{}}},
		{"pages/masterdata/warehouse_detail.html", map[string]any{"Warehouse": warehouse}},
		{"pages/masterdata/warehouse_form.html", map[string]any{"Warehouse": warehouse, "Branches": []branches.Branch{branch}, "Errors": map[string]string{}}},
		{"pages/roles/form.html", map[string]any{"Role": map[string]string{"Name": "Manager", "Description": "Manager role"}, "Errors": map[string]string{}}},
		{"pages/users/form.html", map[string]any{"Errors": map[string]string{}}},
		{"pages/masterdata/branches_list.html", map[string]any{"Branches": []branches.Branch{}, "Filters": masterdatashared.ListFilters{}}},
		{"pages/masterdata/categories_list.html", map[string]any{"Categories": []categories.Category{}, "Filters": masterdatashared.ListFilters{}}},
		{"pages/masterdata/companies_list.html", map[string]any{"Companies": []companies.Company{}, "Filters": masterdatashared.ListFilters{}}},
		{"pages/masterdata/products_list.html", map[string]any{"Products": []products.Product{}, "Filters": masterdatashared.ListFilters{}}},
		{"pages/masterdata/suppliers_list.html", map[string]any{"Suppliers": []suppliers.Supplier{}, "Filters": masterdatashared.ListFilters{}}},
		{"pages/masterdata/taxes_list.html", map[string]any{"Taxes": []taxes.Tax{}, "Filters": masterdatashared.ListFilters{}}},
		{"pages/masterdata/units_list.html", map[string]any{"Units": []units.Unit{}, "Filters": masterdatashared.ListFilters{}}},
		{"pages/masterdata/warehouses_list.html", map[string]any{"Warehouses": []warehouses.Warehouse{}, "Filters": masterdatashared.ListFilters{}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			err := engine.Render(recorder, test.name, view.TemplateData{CSRFToken: "test-csrf", Data: test.data})
			require.NoError(t, err)
			require.Contains(t, recorder.Body.String(), "<!DOCTYPE html>")
		})
	}
}
