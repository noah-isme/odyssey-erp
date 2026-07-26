// This file is an external test package on purpose: internal/ar imports
// internal/view, so a test inside package view cannot import the AR domain
// types without creating an import cycle.
package view_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/ar"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

func sampleARInvoice() ar.ARInvoice {
	return ar.ARInvoice{
		ID: 7, Number: "INV-2026-0007", CustomerID: 3, CustomerName: "PT Nusantara",
		Currency: "IDR", Subtotal: 10_000_000, TaxAmount: 1_100_000, Total: 11_100_000,
		Status: ar.ARStatusPosted, DueAt: time.Now().Add(720 * time.Hour), CreatedAt: time.Now(),
	}
}

// The AR module previously had no list or detail view: both routes rendered the
// creation form, which never references the invoice data passed to it. These
// execute the new templates against the real domain types.
func TestARInvoiceTemplatesExecute(t *testing.T) {
	engine, err := view.NewEngine()
	require.NoError(t, err)
	invoice := sampleARInvoice()

	t.Run("list", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		err := engine.Render(recorder, "pages/ar/ar_invoice_list.html", view.TemplateData{
			Title: "AR Invoices",
			Data:  map[string]any{"Invoices": []ar.ARInvoice{invoice}, "StatusFilter": ar.ARStatusPosted},
		})
		require.NoError(t, err)
		body := recorder.Body.String()
		for _, want := range []string{"INV-2026-0007", "PT Nusantara", "/finance/ar/invoices/7"} {
			assert.Contains(t, body, want)
		}
	})

	t.Run("list falls back to the customer id when the name is missing", func(t *testing.T) {
		bare := invoice
		bare.CustomerName = ""
		recorder := httptest.NewRecorder()
		err := engine.Render(recorder, "pages/ar/ar_invoice_list.html", view.TemplateData{
			Title: "AR Invoices",
			Data:  map[string]any{"Invoices": []ar.ARInvoice{bare}, "StatusFilter": ar.ARInvoiceStatus("")},
		})
		require.NoError(t, err)
		assert.Contains(t, recorder.Body.String(), "Customer #3")
	})

	t.Run("list with no invoices", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		err := engine.Render(recorder, "pages/ar/ar_invoice_list.html", view.TemplateData{
			Title: "AR Invoices",
			Data:  map[string]any{"Invoices": []ar.ARInvoice{}, "StatusFilter": ar.ARInvoiceStatus("")},
		})
		require.NoError(t, err)
		assert.Contains(t, recorder.Body.String(), "No invoices found")
	})

	t.Run("detail", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		err := engine.Render(recorder, "pages/ar/ar_invoice_detail.html", view.TemplateData{
			Title: "AR Invoice",
			Data: map[string]any{"Invoice": &ar.ARInvoiceWithDetails{
				ARInvoice: invoice,
				Lines: []ar.ARInvoiceLine{{
					ProductID: 2, Description: "Jasa konsultasi", Quantity: 2,
					UnitPrice: 5_000_000, TaxAmount: 1_100_000, Total: 11_100_000,
				}},
				Payments: []ar.ARPaymentSummary{{
					Number: "ARP-0001", Amount: 5_000_000, AllocatedAmount: 5_000_000,
					PaidAt: time.Now(), Method: "TRANSFER",
				}},
				PaidAmount: 5_000_000,
				Balance:    6_100_000,
			}},
		})
		require.NoError(t, err)
		body := recorder.Body.String()
		for _, want := range []string{"INV-2026-0007", "PT Nusantara", "Jasa konsultasi", "ARP-0001"} {
			assert.Contains(t, body, want)
		}
	})

	// A freshly created invoice has neither lines nor payments yet.
	t.Run("detail with no lines or payments", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		err := engine.Render(recorder, "pages/ar/ar_invoice_detail.html", view.TemplateData{
			Title: "AR Invoice",
			Data:  map[string]any{"Invoice": &ar.ARInvoiceWithDetails{ARInvoice: invoice}},
		})
		require.NoError(t, err)
		body := recorder.Body.String()
		assert.Contains(t, body, "No line items recorded.")
		assert.Contains(t, body, "No payments recorded.")
	})
}
