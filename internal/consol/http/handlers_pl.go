package http

import (
	"net/http"

	"github.com/odyssey-erp/odyssey-erp/internal/consol"
)

// ProfitLossHandler wires HTTP interactions for the consolidated P&L feature.
type ProfitLossHandler struct {
	service *consol.ProfitLossService
}

// NewProfitLossHandler constructs a new P&L handler.
func NewProfitLossHandler(service *consol.ProfitLossService) *ProfitLossHandler {
	return &ProfitLossHandler{service: service}
}

// HandleGet renders the server side P&L page.
func (h *ProfitLossHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
}

// HandleExportCSV serves the CSV export of the consolidated P&L statement.
func (h *ProfitLossHandler) HandleExportCSV(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
}

// HandleExportPDF serves the PDF export of the consolidated P&L statement.
func (h *ProfitLossHandler) HandleExportPDF(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
}
