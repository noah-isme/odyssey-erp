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
	r.Delete("/batches/{id}/items/{item_id}", h.RemoveBatchItem)
	r.Post("/batches/{id}/approve", h.ApproveBatch)
	r.Post("/batches/{id}/export", h.ExportBatch)
	r.Post("/batches/{id}/settle", h.SettleBatch)
}

func (h *Handler) AddBankAccount(w http.ResponseWriter, r *http.Request) {
	identity, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		writeTreasuryError(w, http.StatusUnauthorized, shared.ErrUnauthorized)
		return
	}
	supplierID, ok := treasuryParamID(r, "supplier_id")
	if !ok {
		writeTreasuryError(w, http.StatusBadRequest, shared.ErrInvalidInput)
		return
	}

	var payload struct {
		BankName      string `json:"bank_name"`
		AccountNumber string `json:"account_number"`
		RoutingNumber string `json:"routing_number"`
		Currency      string `json:"currency"`
		EvidenceRef   string `json:"evidence_ref"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeTreasuryError(w, http.StatusBadRequest, shared.ErrInvalidInput)
		return
	}

	account, err := h.service.AddBankAccount(r.Context(), identity.CompanyID, supplierID, identity.UserID, payload.BankName, payload.AccountNumber, payload.RoutingNumber, payload.Currency, payload.EvidenceRef)
	if err != nil {
		writeTreasuryError(w, http.StatusBadRequest, err)
		return
	}
	writeTreasuryJSON(w, http.StatusCreated, account)
}

func (h *Handler) ApproveBankAccount(w http.ResponseWriter, r *http.Request) {
	identity, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		writeTreasuryError(w, http.StatusUnauthorized, shared.ErrUnauthorized)
		return
	}
	accountID, ok := treasuryParamID(r, "id")
	if !ok {
		writeTreasuryError(w, http.StatusBadRequest, shared.ErrInvalidInput)
		return
	}

	account, err := h.service.ApproveBankAccount(r.Context(), identity.CompanyID, accountID, identity.UserID)
	if err != nil {
		writeTreasuryError(w, http.StatusForbidden, err)
		return
	}
	writeTreasuryJSON(w, http.StatusOK, account)
}

func (h *Handler) ListBankAccounts(w http.ResponseWriter, r *http.Request) {
	identity, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		writeTreasuryError(w, http.StatusUnauthorized, shared.ErrUnauthorized)
		return
	}
	supplierID, ok := treasuryParamID(r, "supplier_id")
	if !ok {
		writeTreasuryError(w, http.StatusBadRequest, shared.ErrInvalidInput)
		return
	}

	accounts, err := h.service.ListBankAccounts(r.Context(), identity.CompanyID, supplierID)
	if err != nil {
		writeTreasuryError(w, http.StatusInternalServerError, err)
		return
	}
	writeTreasuryJSON(w, http.StatusOK, accounts)
}

func (h *Handler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	identity, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		writeTreasuryError(w, http.StatusUnauthorized, shared.ErrUnauthorized)
		return
	}

	var payload struct {
		ReferenceCode string `json:"reference_code"`
		Currency      string `json:"currency"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeTreasuryError(w, http.StatusBadRequest, shared.ErrInvalidInput)
		return
	}

	batch, err := h.service.CreatePaymentBatch(r.Context(), identity.CompanyID, payload.ReferenceCode, payload.Currency, identity.UserID)
	if err != nil {
		writeTreasuryError(w, http.StatusBadRequest, err)
		return
	}
	writeTreasuryJSON(w, http.StatusCreated, batch)
}

func (h *Handler) AddBatchItem(w http.ResponseWriter, r *http.Request) {
	identity, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		writeTreasuryError(w, http.StatusUnauthorized, shared.ErrUnauthorized)
		return
	}
	batchID, ok := treasuryParamID(r, "id")
	if !ok {
		writeTreasuryError(w, http.StatusBadRequest, shared.ErrInvalidInput)
		return
	}

	var payload struct {
		SupplierID    int64   `json:"supplier_id"`
		BankAccountID int64   `json:"bank_account_id"`
		Amount        float64 `json:"amount"`
		APInvoiceID   int64   `json:"ap_invoice_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeTreasuryError(w, http.StatusBadRequest, shared.ErrInvalidInput)
		return
	}

	item, err := h.service.AddBatchItem(r.Context(), identity.CompanyID, batchID, payload.SupplierID, payload.BankAccountID, payload.Amount, payload.APInvoiceID)
	if err != nil {
		writeTreasuryError(w, http.StatusBadRequest, err)
		return
	}
	writeTreasuryJSON(w, http.StatusCreated, item)
}

func (h *Handler) RemoveBatchItem(w http.ResponseWriter, r *http.Request) {
	identity, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		writeTreasuryError(w, http.StatusUnauthorized, shared.ErrUnauthorized)
		return
	}
	batchID, batchOK := treasuryParamID(r, "id")
	itemID, itemOK := treasuryParamID(r, "item_id")
	if !batchOK || !itemOK {
		writeTreasuryError(w, http.StatusBadRequest, shared.ErrInvalidInput)
		return
	}
	if err := h.service.RemoveBatchItem(r.Context(), identity.CompanyID, batchID, itemID); err != nil {
		writeTreasuryError(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ApproveBatch(w http.ResponseWriter, r *http.Request) {
	identity, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		writeTreasuryError(w, http.StatusUnauthorized, shared.ErrUnauthorized)
		return
	}
	batchID, ok := treasuryParamID(r, "id")
	if !ok {
		writeTreasuryError(w, http.StatusBadRequest, shared.ErrInvalidInput)
		return
	}

	batch, err := h.service.ApproveBatch(r.Context(), identity.CompanyID, batchID, identity.UserID)
	if err != nil {
		writeTreasuryError(w, http.StatusForbidden, err)
		return
	}
	writeTreasuryJSON(w, http.StatusOK, batch)
}

func (h *Handler) ExportBatch(w http.ResponseWriter, r *http.Request) {
	identity, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		writeTreasuryError(w, http.StatusUnauthorized, shared.ErrUnauthorized)
		return
	}
	batchID, ok := treasuryParamID(r, "id")
	if !ok {
		writeTreasuryError(w, http.StatusBadRequest, shared.ErrInvalidInput)
		return
	}

	payload, err := h.service.ExportBatch(r.Context(), identity.CompanyID, batchID, identity.UserID, &CSVEncoder{})
	if err != nil {
		writeTreasuryError(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="batch_export.csv"`)
	_, _ = w.Write(payload)
}

func (h *Handler) SettleBatch(w http.ResponseWriter, r *http.Request) {
	identity, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		writeTreasuryError(w, http.StatusUnauthorized, shared.ErrUnauthorized)
		return
	}
	batchID, ok := treasuryParamID(r, "id")
	if !ok {
		writeTreasuryError(w, http.StatusBadRequest, shared.ErrInvalidInput)
		return
	}

	batch, err := h.service.SettleBatch(r.Context(), identity.CompanyID, batchID, identity.UserID)
	if err != nil {
		writeTreasuryError(w, http.StatusBadRequest, err)
		return
	}
	writeTreasuryJSON(w, http.StatusOK, batch)
}

func treasuryParamID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return id, err == nil && id > 0
}

func writeTreasuryJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeTreasuryError(w http.ResponseWriter, status int, err error) {
	shared.WriteErrorStatus(w, status, err)
}
