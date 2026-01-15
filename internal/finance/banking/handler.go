package banking

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type Handler struct {
	logger    *slog.Logger
	service   *Service
	templates *view.Engine
}

func NewHandler(logger *slog.Logger, service *Service, templates *view.Engine) *Handler {
	return &Handler{
		logger:    logger,
		service:   service,
		templates: templates,
	}
}

func (h *Handler) MountRoutes(r chi.Router) {
	r.Get("/accounts", h.handleListAccounts)
	r.Post("/accounts", h.handleCreateAccount)
	r.Get("/accounts/{id}", h.handleShowAccount)
	r.Post("/accounts/{id}/transactions", h.handleCreateTransaction)
}

func (h *Handler) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	// TODO: Get company ID from session context
	companyID := int64(1)

	accounts, err := h.service.ListBankAccounts(r.Context(), companyID)
	if err != nil {
		h.logger.Error("list bank accounts", slog.Any("error", err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := view.TemplateData{
		Data: map[string]any{
			"Accounts": accounts,
		},
	}
	if err := h.templates.Render(w, "pages/finance/banking/list.html", data); err != nil {
		h.logger.Error("render list", slog.Any("error", err))
	}
}

func (h *Handler) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	// TODO: Get company ID from session
	companyID := int64(1)

	name := r.FormValue("name")
	acctNum := r.FormValue("account_number")
	currency := r.FormValue("currency")
	glID, _ := strconv.ParseInt(r.FormValue("gl_account_id"), 10, 64)
	initialBal, _ := strconv.ParseFloat(r.FormValue("initial_balance"), 64)

	input := CreateAccountInput{
		CompanyID:      companyID,
		Name:           name,
		AccountNumber:  acctNum,
		Currency:       currency,
		GLAccountID:    glID,
		InitialBalance: initialBal,
	}

	_, err := h.service.CreateBankAccount(r.Context(), input)
	if err != nil {
		h.logger.Error("create bank account", slog.Any("error", err))
		// render with error... for now simple error
		http.Error(w, "Failed to create account: "+err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/finance/banking/accounts", http.StatusSeeOther)
}

func (h *Handler) handleShowAccount(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	acct, err := h.service.repo.GetBankAccount(r.Context(), id)
	if err != nil {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}

	txns, err := h.service.ListBankTransactions(r.Context(), id)
	if err != nil {
		h.logger.Error("list transactions", slog.Any("error", err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := view.TemplateData{
		Data: map[string]any{
			"Account":      acct,
			"Transactions": txns,
		},
	}
	if err := h.templates.Render(w, "pages/finance/banking/detail.html", data); err != nil {
		h.logger.Error("render detail", slog.Any("error", err))
	}
}

func (h *Handler) handleCreateTransaction(w http.ResponseWriter, r *http.Request) {
	acctID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	// Parse form
	dateStr := r.FormValue("date")
	date, _ := time.Parse("2006-01-02", dateStr)
	amount, _ := strconv.ParseFloat(r.FormValue("amount"), 64)
	desc := r.FormValue("description")
	ref := r.FormValue("reference")
	contraID, _ := strconv.ParseInt(r.FormValue("contra_account_id"), 10, 64)

	// Determine PeriodID from Date (Need a helper service for this really)
	// For now hardcode or assume period is looked up inside service if we move logic there?
	// But service expects PeriodID.
	// Let's rely on a helper if available, or just pass 0 and fail validation for now until we wire PeriodFinder.
	periodID := int64(0) // TODO: Lookup period

	// User ID from session
	userID := int64(1)
	if sess := shared.SessionFromContext(r.Context()); sess != nil {
		// assuming user ID is in session values or derived
	}

	input := CreateTransactionInput{
		BankAccountID:   acctID,
		Date:            date,
		Amount:          amount,
		Description:     desc,
		Reference:       ref,
		ContraAccountID: contraID,
		PeriodID:        periodID,
		CreatedBy:       userID,
	}

	_, err := h.service.CreateBankTransaction(r.Context(), input)
	if err != nil {
		h.logger.Error("create transaction", slog.Any("error", err))
		// redirect back with error flash
		http.Redirect(w, r, "/finance/banking/accounts/"+strconv.FormatInt(acctID, 10), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/finance/banking/accounts/"+strconv.FormatInt(acctID, 10), http.StatusSeeOther)
}
