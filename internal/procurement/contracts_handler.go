package procurement

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Contract Handlers

func (h *Handler) createContract(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	effectiveFrom, err := time.Parse(time.RFC3339, r.PostFormValue("effective_from"))
	if err != nil {
		http.Error(w, "effective_from must be RFC3339", http.StatusBadRequest)
		return
	}

	var effectiveTo *time.Time
	if toStr := r.PostFormValue("effective_to"); toStr != "" {
		to, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			http.Error(w, "effective_to must be RFC3339", http.StatusBadRequest)
			return
		}
		effectiveTo = &to
	}

	input := CreateContractInput{
		CompanyID:         currentCompany(r),
		SupplierID:        parseInt64(r.PostFormValue("supplier_id")),
		Currency:          r.PostFormValue("currency"),
		EffectiveFrom:     effectiveFrom,
		EffectiveTo:       effectiveTo,
		PaymentTerms:      r.PostFormValue("payment_terms"),
		Incoterms:         r.PostFormValue("incoterms"),
		RenewalNoticeDays: formInt(r, "renewal_notice_days", 30),
		CreatedBy:         currentUser(r),
		Note:              r.PostFormValue("note"),
	}

	// Parse price lines
	for index, productID := range r.PostForm["product_id"] {
		if productID == "" {
			continue
		}
		line := ContractPriceLineInput{
			ProductID:    parseInt64(productID),
			LeadTimeDays: parseIntAt(r.PostForm["lead_time_days"], index, 0),
		}

		// Parse Money values
		minQty, _ := accountingmoney.Parse(formValueAt(r.PostForm["min_quantity"], index), 4)
		unitPrice, _ := accountingmoney.Parse(formValueAt(r.PostForm["unit_price"], index), 4)
		taxRate, _ := accountingmoney.Parse(formValueAt(r.PostForm["tax_rate"], index), 2)
		moq, _ := accountingmoney.Parse(formValueAt(r.PostForm["moq"], index), 4)

		line.MinQuantity = minQty
		line.UnitPrice = unitPrice
		line.TaxRate = taxRate
		line.MOQ = moq

		input.PriceLines = append(input.PriceLines, line)
	}

	contract, err := h.contracts.CreateContractDraft(r.Context(), input)
	if err != nil {
		h.contractError(w, err)
		return
	}

	writeContractJSON(w, http.StatusCreated, contract)
}

func (h *Handler) getContract(w http.ResponseWriter, r *http.Request) {
	contractID := routeInt64(r, "id")
	contract, err := h.contracts.repo.GetContract(r.Context(), contractID)
	if err != nil {
		h.contractError(w, err)
		return
	}
	writeContractJSON(w, http.StatusOK, contract)
}

func (h *Handler) listContracts(w http.ResponseWriter, r *http.Request) {
	if h.contractUnavailable(w) {
		return
	}
	limit := queryInt64(r, "limit", 100)
	offset := queryInt64(r, "offset", 0)
	contracts, err := h.contracts.ListContracts(
		r.Context(),
		currentCompany(r),
		queryInt64(r, "supplier_id", 0),
		r.URL.Query().Get("status"),
		int(limit),
		int(offset),
	)
	if err != nil {
		h.contractError(w, err)
		return
	}
	writeContractJSON(w, http.StatusOK, contracts)
}

func (h *Handler) approveContract(w http.ResponseWriter, r *http.Request) {
	contractID := routeInt64(r, "id")
	approvedBy := currentUser(r)

	if err := h.contracts.ApproveContract(r.Context(), contractID, approvedBy); err != nil {
		h.contractError(w, err)
		return
	}

	contract, err := h.contracts.repo.GetContract(r.Context(), contractID)
	if err != nil {
		h.contractError(w, err)
		return
	}

	writeContractJSON(w, http.StatusOK, contract)
}

func (h *Handler) rejectContract(w http.ResponseWriter, r *http.Request) {
	contractID := routeInt64(r, "id")

	if err := h.contracts.RejectContract(r.Context(), contractID); err != nil {
		h.contractError(w, err)
		return
	}

	writeContractJSON(w, http.StatusOK, map[string]string{"status": "DRAFT"})
}

func (h *Handler) terminateContract(w http.ResponseWriter, r *http.Request) {
	contractID := routeInt64(r, "id")

	if err := h.contracts.TerminateContract(r.Context(), contractID); err != nil {
		h.contractError(w, err)
		return
	}

	writeContractJSON(w, http.StatusOK, map[string]string{"status": "TERMINATED"})
}

// Scorecard Handlers

func (h *Handler) createScorecard(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	periodStart, err := time.Parse(time.RFC3339, r.PostFormValue("period_start"))
	if err != nil {
		http.Error(w, "period_start must be RFC3339", http.StatusBadRequest)
		return
	}

	periodEnd, err := time.Parse(time.RFC3339, r.PostFormValue("period_end"))
	if err != nil {
		http.Error(w, "period_end must be RFC3339", http.StatusBadRequest)
		return
	}

	input := CreateScorecardInput{
		CompanyID:   currentCompany(r),
		SupplierID:  parseInt64(r.PostFormValue("supplier_id")),
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		CreatedBy:   currentUser(r),
		Note:        r.PostFormValue("note"),
	}

	// Optional reviewer assessment score
	if reviewerScoreStr := r.PostFormValue("reviewer_assessment_score"); reviewerScoreStr != "" {
		if reviewerScore, err := accountingmoney.Parse(reviewerScoreStr, 2); err == nil {
			input.ReviewerAssessmentScore = &reviewerScore
		}
	}

	scorecard, err := h.scorecards.CreateDraftScorecard(r.Context(), input)
	if err != nil {
		h.scorecardError(w, err)
		return
	}

	writeScorecardJSON(w, http.StatusCreated, scorecard)
}

func (h *Handler) getScorecard(w http.ResponseWriter, r *http.Request) {
	scorecardID := routeInt64(r, "id")
	scorecard, err := h.scorecards.repo.GetScorecard(r.Context(), scorecardID)
	if err != nil {
		h.scorecardError(w, err)
		return
	}
	writeScorecardJSON(w, http.StatusOK, scorecard)
}

func (h *Handler) publishScorecard(w http.ResponseWriter, r *http.Request) {
	scorecardID := routeInt64(r, "id")
	publishedBy := currentUser(r)

	if err := h.scorecards.PublishScorecard(r.Context(), scorecardID, publishedBy); err != nil {
		h.scorecardError(w, err)
		return
	}

	scorecard, err := h.scorecards.repo.GetScorecard(r.Context(), scorecardID)
	if err != nil {
		h.scorecardError(w, err)
		return
	}

	writeScorecardJSON(w, http.StatusOK, scorecard)
}

// PO Variance Handlers

func (h *Handler) createPOVariance(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	input := CreatePOVarianceInput{
		CompanyID:      currentCompany(r),
		POID:           parseInt64(r.PostFormValue("po_id")),
		POLineID:       parseInt64(r.PostFormValue("po_line_id")),
		VarianceType:   VarianceType(r.PostFormValue("variance_type")),
		VarianceReason: r.PostFormValue("variance_reason"),
		Note:           r.PostFormValue("note"),
	}

	// Optional contract ID
	if contractIDStr := r.PostFormValue("contract_id"); contractIDStr != "" {
		contractID := parseInt64(contractIDStr)
		input.ContractID = &contractID
	}

	// Optional variance percentage
	if variancePctStr := r.PostFormValue("variance_percentage"); variancePctStr != "" {
		if variancePct, err := accountingmoney.Parse(variancePctStr, 2); err == nil {
			input.VariancePercentage = &variancePct
		}
	}

	varianceID, err := h.contracts.repo.CreatePOVariance(r.Context(), input)
	if err != nil {
		h.contractError(w, err)
		return
	}

	variance, err := h.contracts.repo.GetPOVariance(r.Context(), varianceID)
	if err != nil {
		h.contractError(w, err)
		return
	}

	writeContractJSON(w, http.StatusCreated, variance)
}

func (h *Handler) approvePOVariance(w http.ResponseWriter, r *http.Request) {
	varianceID := routeInt64(r, "id")
	approvedBy := currentUser(r)

	if err := h.contracts.repo.ApprovePOVariance(r.Context(), varianceID, approvedBy); err != nil {
		h.contractError(w, err)
		return
	}

	variance, err := h.contracts.repo.GetPOVariance(r.Context(), varianceID)
	if err != nil {
		h.contractError(w, err)
		return
	}

	writeContractJSON(w, http.StatusOK, variance)
}

func (h *Handler) listPendingVariances(w http.ResponseWriter, r *http.Request) {
	if h.contractUnavailable(w) {
		return
	}
	limit := queryInt64(r, "limit", 100)
	offset := queryInt64(r, "offset", 0)
	variances, err := h.contracts.ListPendingVariances(r.Context(), currentCompany(r), int(limit), int(offset))
	if err != nil {
		h.contractError(w, err)
		return
	}
	writeContractJSON(w, http.StatusOK, variances)
}

// Helper functions
var _ = (*Handler).contractUnavailable

func (h *Handler) contractUnavailable(w http.ResponseWriter) bool {
	if h.contracts != nil {
		return false
	}
	http.Error(w, "Contracts module is not configured", http.StatusServiceUnavailable)
	return true
}

func (h *Handler) contractError(w http.ResponseWriter, err error) {
	shared.WriteErrorStatus(w, http.StatusInternalServerError, err)
}

func (h *Handler) scorecardError(w http.ResponseWriter, err error) {
	shared.WriteErrorStatus(w, http.StatusInternalServerError, err)
}

func writeContractJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeScorecardJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Form helper functions

func parseIntAt(values []string, index int, defaultVal int) int {
	if index >= len(values) || values[index] == "" {
		return defaultVal
	}
	if v, err := strconv.Atoi(values[index]); err == nil {
		return v
	}
	return defaultVal
}

func queryInt64(r *http.Request, name string, defaultVal int64) int64 {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultVal
	}
	if v, err := strconv.ParseInt(val, 10, 64); err == nil {
		return v
	}
	return defaultVal
}
