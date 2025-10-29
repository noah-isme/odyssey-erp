package http

import (
	"net/http"

	"github.com/odyssey-erp/odyssey-erp/internal/consol"
)

// BalanceSheetHandler wires the HTTP layer for consolidated balance sheet endpoints.
type BalanceSheetHandler struct {
	service *consol.BalanceSheetService
}

// NewBalanceSheetHandler constructs the handler instance.
func NewBalanceSheetHandler(service *consol.BalanceSheetService) *BalanceSheetHandler {
	return &BalanceSheetHandler{service: service}
}

// HandleGet renders the consolidated balance sheet page.
func (h *BalanceSheetHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
}

// HandleExportCSV handles CSV exports of the balance sheet.
func (h *BalanceSheetHandler) HandleExportCSV(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
}

// HandleExportPDF handles PDF exports of the balance sheet.
func (h *BalanceSheetHandler) HandleExportPDF(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
}
