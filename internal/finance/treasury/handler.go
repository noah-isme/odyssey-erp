package treasury

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) MountRoutes(r chi.Router) {
	r.Post("/suppliers/{supplier_id}/bank-accounts", h.AddBankAccount)
	r.Post("/bank-accounts/{id}/approve", h.ApproveBankAccount)
	r.Get("/suppliers/{supplier_id}/bank-accounts", h.ListBankAccounts)

	r.Post("/batches", h.CreateBatch)
	r.Post("/batches/{id}/items", h.AddBatchItem)
	r.Post("/batches/{id}/approve", h.ApproveBatch)
	r.Post("/batches/{id}/export", h.ExportBatch)
	r.Post("/batches/{id}/settle", h.SettleBatch)
}

func (h *Handler) AddBankAccount(w http.ResponseWriter, r *http.Request) {
	companyIDStr := r.URL.Query().Get("company_id")
	companyID, _ := strconv.ParseInt(companyIDStr, 10, 64)

	supplierIDStr := chi.URLParam(r, "supplier_id")
	supplierID, _ := strconv.ParseInt(supplierIDStr, 10, 64)

	sess := shared.SessionFromContext(r.Context())
	actorID, _ := strconv.ParseInt(sess.User(), 10, 64)

	var payload struct {
		BankName      string `json:"bank_name"`
		AccountNumber string `json:"account_number"`
		RoutingNumber string `json:"routing_number"`
		Currency      string `json:"currency"`
		EvidenceRef   string `json:"evidence_ref"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	account, err := h.service.AddBankAccount(r.Context(), companyID, supplierID, actorID, payload.BankName, payload.AccountNumber, payload.RoutingNumber, payload.Currency, payload.EvidenceRef)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(account)
}

func (h *Handler) ApproveBankAccount(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	accountID, _ := strconv.ParseInt(idStr, 10, 64)

	sess := shared.SessionFromContext(r.Context())
	approverID, _ := strconv.ParseInt(sess.User(), 10, 64)

	account, err := h.service.ApproveBankAccount(r.Context(), accountID, approverID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(account)
}

func (h *Handler) ListBankAccounts(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented yet", http.StatusNotImplemented)
}

func (h *Handler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	companyIDStr := r.URL.Query().Get("company_id")
	companyID, _ := strconv.ParseInt(companyIDStr, 10, 64)

	sess := shared.SessionFromContext(r.Context())
	actorID, _ := strconv.ParseInt(sess.User(), 10, 64)

	var payload struct {
		ReferenceCode string `json:"reference_code"`
		Currency      string `json:"currency"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	batch, err := h.service.CreatePaymentBatch(r.Context(), companyID, payload.ReferenceCode, payload.Currency, actorID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(batch)
}

func (h *Handler) AddBatchItem(w http.ResponseWriter, r *http.Request) {
	// Omitted fully functional request parsing for brevity, assuming similar to above
	w.WriteHeader(http.StatusNotImplemented)
}

func (h *Handler) ApproveBatch(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	batchID, _ := strconv.ParseInt(idStr, 10, 64)

	sess := shared.SessionFromContext(r.Context())
	approverID, _ := strconv.ParseInt(sess.User(), 10, 64)

	batch, err := h.service.ApproveBatch(r.Context(), batchID, approverID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(batch)
}

func (h *Handler) ExportBatch(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	batchID, _ := strconv.ParseInt(idStr, 10, 64)

	sess := shared.SessionFromContext(r.Context())
	actorID, _ := strconv.ParseInt(sess.User(), 10, 64)

	encoder := &CSVEncoder{} // We can inject/configure this later based on bank policy
	payload, err := h.service.ExportBatch(r.Context(), batchID, actorID, encoder)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="batch_export.csv"`)
	_, _ = w.Write(payload)
}

func (h *Handler) SettleBatch(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	batchID, _ := strconv.ParseInt(idStr, 10, 64)

	sess := shared.SessionFromContext(r.Context())
	actorID, _ := strconv.ParseInt(sess.User(), 10, 64)

	batch, err := h.service.SettleBatch(r.Context(), batchID, actorID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(batch)
}
