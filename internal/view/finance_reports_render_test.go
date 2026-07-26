package view

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/reports"
)

// sampleBalances mirrors the shape the accounting handlers pass to the report
// builders, covering every balance sheet classification.
func sampleBalances() []reports.AccountBalance {
	return []reports.AccountBalance{
		{ID: 1, Code: "1110", Name: "Kas", Type: "ASSET", Opening: 10_000, Debit: 5_000, Credit: 2_000},
		{ID: 2, Code: "1210", Name: "Piutang Usaha", Type: "ASSET", Debit: 7_500},
		{ID: 3, Code: "2110", Name: "Hutang Usaha", Type: "LIABILITY", Credit: 4_000},
		{ID: 4, Code: "3100", Name: "Modal Disetor", Type: "EQUITY", Credit: 16_500},
	}
}

// These templates previously referenced fields that did not exist on the report
// structs. Parsing succeeded, so the existing "template exists" tests passed
// while every request died part-way through execution.
func TestFinanceReportTemplatesExecute(t *testing.T) {
	engine, err := NewEngine()
	require.NoError(t, err)

	balances := sampleBalances()

	cases := []struct {
		name     string
		template string
		data     map[string]any
		contains []string
	}{
		{
			name:     "trial balance",
			template: "pages/finance/trial_balance.html",
			data:     map[string]any{"Report": reports.BuildTrialBalance(balances)},
			contains: []string{"Kas", "Piutang Usaha", "Hutang Usaha", "Modal Disetor"},
		},
		{
			name:     "balance sheet",
			template: "pages/finance/balance_sheet.html",
			data:     map[string]any{"Report": reports.BuildBalanceSheet(balances), "AsOfDate": time.Now()},
			contains: []string{"Kas", "Hutang Usaha", "Modal Disetor", "Total Assets", "Total Liabilities"},
		},
		{
			// A fresh install has no postings at all; the empty-section branch
			// resolves its label from the enclosing section, not the range.
			name:     "balance sheet with no accounts",
			template: "pages/finance/balance_sheet.html",
			data:     map[string]any{"Report": reports.BuildBalanceSheet(nil), "AsOfDate": time.Now()},
			contains: []string{"No Assets accounts.", "No Liabilities accounts.", "No Equity accounts."},
		},
		{
			name:     "trial balance with no accounts",
			template: "pages/finance/trial_balance.html",
			data:     map[string]any{"Report": reports.BuildTrialBalance(nil)},
			contains: []string{"No data available for the selected period."},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			err := engine.Render(recorder, tc.template, TemplateData{Title: tc.name, Data: tc.data})
			require.NoError(t, err, "template must execute, not just parse")

			body := recorder.Body.String()
			for _, want := range tc.contains {
				assert.Contains(t, body, want)
			}
		})
	}
}

// Render must buffer: executing straight into the ResponseWriter commits a 200
// on the first write, leaving callers unable to turn a mid-render failure into
// a 5xx. That is what let the broken report templates serve truncated HTML
// under a success status.
func TestRenderLeavesResponseUntouchedOnTemplateError(t *testing.T) {
	engine, err := NewEngine()
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	// "Report" carries the wrong type, so execution fails once it reaches the
	// table body - after the page header would already have been written
	// unbuffered. This is the shape of the original bug: the templates named
	// fields the report structs did not have.
	err = engine.Render(recorder, "pages/finance/trial_balance.html", TemplateData{
		Title: "Trial Balance",
		Data:  map[string]any{"Report": "not a report"},
	})

	require.Error(t, err, "a bad field reference must surface as an error")
	assert.Empty(t, recorder.Body.String(), "no partial HTML may reach the client")
	assert.False(t, recorder.Flushed, "response must not be committed")
}

// Handlers that need a non-200 status must go through RenderStatus rather than
// calling WriteHeader themselves. Writing the header first commits the response,
// so a template failure would be served as the intended status with an empty
// body - the shape that hid broken pages across 22 handlers.
func TestRenderStatusAppliesStatusOnSuccess(t *testing.T) {
	engine, err := NewEngine()
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	err = engine.RenderStatus(recorder, "pages/finance/trial_balance.html", TemplateData{
		Title: "Trial Balance",
		Data:  map[string]any{"Report": reports.BuildTrialBalance(sampleBalances())},
	}, http.StatusNotFound)

	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, recorder.Code, "the requested status must be used")
	assert.Contains(t, recorder.Body.String(), "Kas")
}

func TestRenderStatusLeavesResponseUntouchedOnTemplateError(t *testing.T) {
	engine, err := NewEngine()
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	err = engine.RenderStatus(recorder, "pages/finance/trial_balance.html", TemplateData{
		Title: "Trial Balance",
		Data:  map[string]any{"Report": "not a report"},
	}, http.StatusUnprocessableEntity)

	require.Error(t, err)
	assert.Empty(t, recorder.Body.String(), "no partial HTML may reach the client")
	assert.False(t, recorder.Flushed, "response must not be committed")
	// The caller must still be able to send its own error status.
	http.Error(recorder, "boom", http.StatusInternalServerError)
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
}
