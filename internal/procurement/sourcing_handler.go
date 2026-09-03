package procurement

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

func (h *Handler) sourcingUnavailable(w http.ResponseWriter) bool {
	if h.sourcing != nil {
		return false
	}
	http.Error(w, "RFQ sourcing is not configured", http.StatusServiceUnavailable)
	return true
}

func (h *Handler) createRFQ(w http.ResponseWriter, r *http.Request) {
	if h.sourcingUnavailable(w) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	dueAt, err := time.Parse(time.RFC3339, r.PostFormValue("response_due_at"))
	if err != nil {
		http.Error(w, "response_due_at must be RFC3339", http.StatusBadRequest)
		return
	}
	input := CreateRFQInput{CompanyID: currentCompany(r), CreatedBy: currentUser(r), Number: r.PostFormValue("number"), Currency: r.PostFormValue("currency"), ResponseDueAt: dueAt, CommercialTerms: r.PostFormValue("commercial_terms"), Weights: RFQWeights{Price: formInt(r, "price_weight", 50), LeadTime: formInt(r, "lead_time_weight", 20), Terms: formInt(r, "terms_weight", 10), SupplierRating: formInt(r, "supplier_rating_weight", 20)}}
	for index, product := range r.PostForm["product_id"] {
		input.Lines = append(input.Lines, RFQLineInput{ProductID: parseInt64(product), PRLineID: parseInt64(formValueAt(r.PostForm["pr_line_id"], index)), Quantity: formValueAt(r.PostForm["quantity"], index), Note: formValueAt(r.PostForm["line_note"], index)})
	}
	for _, supplier := range r.PostForm["supplier_id"] {
		input.SupplierIDs = append(input.SupplierIDs, parseInt64(supplier))
	}
	rfq, err := h.sourcing.CreateRFQ(r.Context(), input)
	if err != nil {
		h.sourcingError(w, err)
		return
	}
	writeSourcingJSON(w, http.StatusCreated, rfq)
}

func (h *Handler) issueRFQ(w http.ResponseWriter, r *http.Request) {
	if h.sourcingUnavailable(w) {
		return
	}
	if err := h.sourcing.IssueRFQ(r.Context(), routeInt64(r, "id"), currentUser(r)); err != nil {
		h.sourcingError(w, err)
		return
	}
	writeSourcingJSON(w, http.StatusOK, map[string]string{"status": "ISSUED"})
}
func (h *Handler) closeRFQ(w http.ResponseWriter, r *http.Request) {
	if h.sourcingUnavailable(w) {
		return
	}
	if err := h.sourcing.CloseRFQ(r.Context(), routeInt64(r, "id"), currentUser(r)); err != nil {
		h.sourcingError(w, err)
		return
	}
	writeSourcingJSON(w, http.StatusOK, map[string]string{"status": "CLOSED"})
}
func (h *Handler) compareRFQ(w http.ResponseWriter, r *http.Request) {
	if h.sourcingUnavailable(w) {
		return
	}
	entries, err := h.sourcing.SnapshotComparison(r.Context(), routeInt64(r, "id"), currentUser(r))
	if err != nil {
		h.sourcingError(w, err)
		return
	}
	writeSourcingJSON(w, http.StatusOK, entries)
}

func (h *Handler) createBid(w http.ResponseWriter, r *http.Request) {
	if h.sourcingUnavailable(w) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	fxDate, err := time.Parse("2006-01-02", r.PostFormValue("fx_rate_date"))
	if err != nil {
		http.Error(w, "fx_rate_date must be YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	input := CreateBidInput{RFQID: parseInt64(r.PostFormValue("rfq_id")), SupplierID: parseInt64(r.PostFormValue("supplier_id")), CompanyID: currentCompany(r), CreatedBy: currentUser(r), Currency: r.PostFormValue("currency"), FXRate: r.PostFormValue("fx_rate"), FXRateDate: fxDate, PaymentTerms: r.PostFormValue("payment_terms"), SourceReference: r.PostFormValue("source_reference")}
	for index, rfqLineID := range r.PostForm["rfq_line_id"] {
		unitPrice, err := accountingmoney.Parse(formValueAt(r.PostForm["unit_price"], index), 4)
		if err != nil {
			h.sourcingError(w, ErrValidation)
			return
		}
		tax, err := accountingmoney.Parse(defaultFormValue(formValueAt(r.PostForm["tax_amount"], index), "0"), 4)
		if err != nil {
			h.sourcingError(w, ErrValidation)
			return
		}
		freight, err := accountingmoney.Parse(defaultFormValue(formValueAt(r.PostForm["freight_amount"], index), "0"), 4)
		if err != nil {
			h.sourcingError(w, ErrValidation)
			return
		}
		input.Lines = append(input.Lines, BidLineInput{RFQLineID: parseInt64(rfqLineID), Quantity: formValueAt(r.PostForm["quantity"], index), UnitPrice: unitPrice, TaxAmount: tax, FreightAmount: freight, MinimumOrderQuantity: formValueAt(r.PostForm["minimum_order_quantity"], index), LeadTimeDays: formIntAt(r.PostForm["lead_time_days"], index), CommercialScore: formIntAt(r.PostForm["commercial_score"], index), SupplierRatingScore: formIntAt(r.PostForm["supplier_rating_score"], index), Note: formValueAt(r.PostForm["line_note"], index)})
	}
	id, err := h.sourcing.CreateBid(r.Context(), input)
	if err != nil {
		h.sourcingError(w, err)
		return
	}
	writeSourcingJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (h *Handler) submitBid(w http.ResponseWriter, r *http.Request) {
	if h.sourcingUnavailable(w) {
		return
	}
	if err := h.sourcing.SubmitBid(r.Context(), routeInt64(r, "id"), currentUser(r)); err != nil {
		h.sourcingError(w, err)
		return
	}
	writeSourcingJSON(w, http.StatusOK, map[string]string{"status": "SUBMITTED"})
}

func (h *Handler) createAward(w http.ResponseWriter, r *http.Request) {
	if h.sourcingUnavailable(w) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	input := CreateAwardInput{RFQID: parseInt64(r.PostFormValue("rfq_id")), CompanyID: currentCompany(r), ExpectedWarehouseID: parseInt64(r.PostFormValue("expected_warehouse_id")), CreatedBy: currentUser(r), Note: r.PostFormValue("note")}
	for index, rfqLineID := range r.PostForm["rfq_line_id"] {
		input.Lines = append(input.Lines, AwardLineInput{RFQLineID: parseInt64(rfqLineID), BidLineID: parseInt64(formValueAt(r.PostForm["bid_line_id"], index)), Quantity: formValueAt(r.PostForm["quantity"], index)})
	}
	award, err := h.sourcing.CreateAward(r.Context(), input)
	if err != nil {
		h.sourcingError(w, err)
		return
	}
	writeSourcingJSON(w, http.StatusCreated, award)
}

func (h *Handler) submitAward(w http.ResponseWriter, r *http.Request) {
	if h.sourcingUnavailable(w) {
		return
	}
	if err := h.sourcing.SubmitAward(r.Context(), routeInt64(r, "id"), currentUser(r)); err != nil {
		h.sourcingError(w, err)
		return
	}
	writeSourcingJSON(w, http.StatusOK, map[string]string{"status": "APPROVAL"})
}
func (h *Handler) sourcingError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if err == ErrNotFound {
		status = http.StatusNotFound
	}
	shared.WriteErrorStatus(w, status, err)
}
func writeSourcingJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func routeInt64(r *http.Request, name string) int64 { return parseInt64(chi.URLParam(r, name)) }
func parseInt64(value string) int64                 { parsed, _ := strconv.ParseInt(value, 10, 64); return parsed }
func formInt(r *http.Request, key string, fallback int) int {
	value := r.PostFormValue(key)
	if value == "" {
		return fallback
	}
	return formIntAt([]string{value}, 0)
}
func formIntAt(values []string, index int) int {
	value, _ := strconv.Atoi(formValueAt(values, index))
	return value
}
func formValueAt(values []string, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}
	return values[index]
}
func defaultFormValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
