package banking

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type Handler struct {
	logger    *slog.Logger
	service   *Service
	templates *view.Engine
	csrf      *shared.CSRFManager
}

func NewHandler(logger *slog.Logger, service *Service, templates *view.Engine, csrf *shared.CSRFManager) *Handler {
	return &Handler{
		logger:    logger,
		service:   service,
		templates: templates,
		csrf:      csrf,
	}
}

func (h *Handler) MountRoutes(r chi.Router) {
	r.Get("/accounts", h.handleListAccounts)
	r.Post("/accounts", h.handleCreateAccount)
	r.Get("/accounts/{id}", h.handleShowAccount)
	r.Post("/accounts/{id}/transactions", h.handleCreateTransaction)
	r.Post("/transactions/{id}/reconcile", h.handleReconcileTransaction)
	r.Post("/transfer", h.handleTransferFunds)
}

func (h *Handler) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	companyID := int64(1) // Default for now

	accounts, err := h.service.ListBankAccounts(ctx, companyID)
	if err != nil {
		h.logger.Error("list bank accounts", slog.Any("error", err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/finance/banking/list.html", map[string]any{
		"Accounts": accounts,
	})
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
		h.redirectWithFlash(w, r, "/finance/banking/accounts", "error", "Failed to create account: "+err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/finance/banking/accounts", "success", "Bank account created")
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

	h.render(w, r, "pages/finance/banking/detail.html", map[string]any{
		"Account":      acct,
		"Transactions": txns,
	})
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

	sess := shared.SessionFromContext(r.Context())
	userID := getUserID(sess)

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
		h.redirectWithFlash(w, r, "/finance/banking/accounts/"+strconv.FormatInt(acctID, 10), "error", "Failed to create transaction: "+err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/finance/banking/accounts/"+strconv.FormatInt(acctID, 10), "success", "Transaction recorded")
}

func (h *Handler) handleReconcileTransaction(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid transaction ID", http.StatusBadRequest)
		return
	}

	if err := h.service.ReconcileTransaction(r.Context(), id); err != nil {
		h.logger.Error("reconcile transaction", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/finance/banking/accounts", "error", "Failed to reconcile")
		return
	}

	h.redirectWithFlash(w, r, "/finance/banking/accounts", "success", "Transaction reconciled")
}

func (h *Handler) handleTransferFunds(w http.ResponseWriter, r *http.Request) {
	fromID, _ := strconv.ParseInt(r.FormValue("from_account_id"), 10, 64)
	toID, _ := strconv.ParseInt(r.FormValue("to_account_id"), 10, 64)
	amount, _ := strconv.ParseFloat(r.FormValue("amount"), 64)
	dateStr := r.FormValue("date")
	date, _ := time.Parse("2006-01-02", dateStr)
	desc := r.FormValue("description")
	ref := r.FormValue("reference")

	sess := shared.SessionFromContext(r.Context())
	userID := getUserID(sess)
	periodID := int64(1) // TODO: Lookup period

	input := TransferInput{
		FromAccountID: fromID,
		ToAccountID:   toID,
		Amount:        amount,
		Date:          date,
		Description:   desc,
		Reference:     ref,
		PeriodID:      periodID,
		CreatedBy:     userID,
	}

	if err := h.service.TransferFunds(r.Context(), input); err != nil {
		h.logger.Error("transfer funds", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/finance/banking/accounts", "error", "Transfer failed: "+err.Error())
		return
	}

	h.redirectWithFlash(w, r, "/finance/banking/accounts", "success", "Funds transferred successfully")
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, template string, data map[string]any) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	viewData := view.TemplateData{
		Title:       "Banking",
		CSRFToken:   csrfToken,
		Flash:       flash,
		CurrentPath: r.URL.Path,
		Data:        data,
	}
	if err := h.templates.Render(w, template, viewData); err != nil {
		h.logger.Error("render template", slog.Any("error", err), slog.String("template", template))
	}
}

func (h *Handler) redirectWithFlash(w http.ResponseWriter, r *http.Request, location, kind, message string) {
	if sess := shared.SessionFromContext(r.Context()); sess != nil {
		sess.AddFlash(shared.FlashMessage{Kind: kind, Message: message})
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}

func getUserID(sess *shared.Session) int64 {
	if sess == nil || sess.User() == "" {
		return 0
	}
	id, _ := strconv.ParseInt(sess.User(), 10, 64)
	return id
}
