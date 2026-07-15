package view

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEngine(t *testing.T) {
	engine, err := NewEngine()
	assert.NoError(t, err, "Templates should parse without error")
	assert.NotNil(t, engine)
}

func TestManagementRouteTemplatesExist(t *testing.T) {
	engine, err := NewEngine()
	require.NoError(t, err)

	requiredTemplates := []string{
		"pages/masterdata/branch_detail.html",
		"pages/masterdata/branch_form.html",
		"pages/masterdata/category_detail.html",
		"pages/masterdata/category_form.html",
		"pages/masterdata/company_detail.html",
		"pages/masterdata/company_form.html",
		"pages/masterdata/product_detail.html",
		"pages/masterdata/product_form.html",
		"pages/masterdata/supplier_detail.html",
		"pages/masterdata/supplier_form.html",
		"pages/masterdata/tax_detail.html",
		"pages/masterdata/tax_form.html",
		"pages/masterdata/unit_detail.html",
		"pages/masterdata/unit_form.html",
		"pages/masterdata/warehouse_detail.html",
		"pages/masterdata/warehouse_form.html",
		"pages/roles/form.html",
		"pages/users/form.html",
	}

	for _, name := range requiredTemplates {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, engine.templates, name)
		})
	}
}
