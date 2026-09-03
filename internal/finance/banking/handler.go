package banking

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
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
	r.Post("/accounts/{id}/import", h.handleImportStatement)
	r.Post("/transactions/{id}/reconcile", h.handleReconcileTransaction)
	r.Post("/transfer", h.handleTransferFunds)
}

func (h *Handler) handleImportStatement(w http.ResponseWriter, r *http.Request) {
	companyID, err := activeCompanyID(r)
	if err != nil {
		h.redirectWithFlash(w, r, "/", "error", "Pilih perusahaan aktif terlebih dahulu")
		return
	}
	accountID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || accountID <= 0 {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}
	account, err := h.service.repo.GetBankAccount(r.Context(), accountID)
	if err != nil || account.CompanyID != companyID {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		h.redirectWithFlash(w, r, "/finance/banking/accounts/"+strconv.FormatInt(accountID, 10), "error", "File statement maksimal 5 MB")
		return
	}
	file, header, err := r.FormFile("statement")
	if err != nil {
		h.redirectWithFlash(w, r, "/finance/banking/accounts/"+strconv.FormatInt(accountID, 10), "error", "Pilih file CSV atau OFX")
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, 5<<20))
	if err != nil {
		h.redirectWithFlash(w, r, "/finance/banking/accounts/"+strconv.FormatInt(accountID, 10), "error", "File statement tidak dapat dibaca")
		return
	}
	hash := sha256.Sum256(content)
	contentHash := hex.EncodeToString(hash[:])
	entries, err := parseStatement(header.Filename, content, account.Currency, account.ID)
	if err != nil {
		h.redirectWithFlash(w, r, "/finance/banking/accounts/"+strconv.FormatInt(accountID, 10), "error", shared.UserSafeMessage(err))
		return
	}
	result, err := h.service.ImportStatement(r.Context(), account, entries, header.Filename, contentHash)
	if err != nil {
		h.logger.Error("import bank statement", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/finance/banking/accounts/"+strconv.FormatInt(accountID, 10), "error", "Statement tidak dapat diimpor")
		return
	}
	h.redirectWithFlash(w, r, "/finance/banking/accounts/"+strconv.FormatInt(accountID, 10), "success", "Statement diimpor: "+strconv.Itoa(result.Imported)+", dilewati: "+strconv.Itoa(result.Skipped))
}

func (h *Handler) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	companyID, err := activeCompanyID(r)
	if err != nil {
		h.redirectWithFlash(w, r, "/", "error", "Pilih perusahaan aktif terlebih dahulu")
		return
	}

	accounts, err := h.service.ListBankAccountSummaries(ctx, companyID)
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
	companyID, err := activeCompanyID(r)
	if err != nil {
		h.redirectWithFlash(w, r, "/", "error", "Pilih perusahaan aktif terlebih dahulu")
		return
	}

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

	_, err = h.service.CreateBankAccount(r.Context(), input)
	if err != nil {
		h.logger.Error("create bank account", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/finance/banking/accounts", "error", shared.UserSafeMessage(err))
		return
	}

	h.redirectWithFlash(w, r, "/finance/banking/accounts", "success", "Bank account created")
}

func (h *Handler) handleShowAccount(w http.ResponseWriter, r *http.Request) {
	companyID, err := activeCompanyID(r)
	if err != nil {
		h.redirectWithFlash(w, r, "/", "error", "Pilih perusahaan aktif terlebih dahulu")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}

	acct, err := h.service.repo.GetBankAccount(r.Context(), id)
	if err != nil {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}
	if acct.CompanyID != companyID {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}

	txns, balance, err := h.service.ListBankTransactionSummaries(r.Context(), acct)
	if err != nil {
		h.logger.Error("list transactions", slog.Any("error", err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/finance/banking/detail.html", map[string]any{
		"Account":      acct,
		"Transactions": txns,
		"Balance":      balance,
	})
}

func (h *Handler) handleCreateTransaction(w http.ResponseWriter, r *http.Request) {
	companyID, err := activeCompanyID(r)
	if err != nil {
		h.redirectWithFlash(w, r, "/", "error", "Pilih perusahaan aktif terlebih dahulu")
		return
	}
	acctID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || acctID <= 0 {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}

	// Parse form
	dateStr := r.FormValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	amount, amountErr := strconv.ParseFloat(r.FormValue("amount"), 64)
	desc := r.FormValue("description")
	ref := r.FormValue("reference")
	contraID, _ := strconv.ParseInt(r.FormValue("contra_account_id"), 10, 64)

	if err != nil || amountErr != nil || amount == 0 {
		h.redirectWithFlash(w, r, "/finance/banking/accounts/"+strconv.FormatInt(acctID, 10), "error", "Tanggal dan jumlah transaksi tidak valid")
		return
	}
	account, err := h.service.repo.GetBankAccount(r.Context(), acctID)
	if err != nil || account.CompanyID != companyID {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}
	periodID, err := h.service.ResolveOpenPeriod(r.Context(), companyID, date)
	if err != nil {
		h.redirectWithFlash(w, r, "/finance/banking/accounts/"+strconv.FormatInt(acctID, 10), "error", "Tidak ada periode akuntansi terbuka untuk tanggal transaksi")
		return
	}

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

	_, err = h.service.CreateBankTransaction(r.Context(), input)
	if err != nil {
		h.logger.Error("create transaction", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/finance/banking/accounts/"+strconv.FormatInt(acctID, 10), "error", shared.UserSafeMessage(err))
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
	companyID, err := activeCompanyID(r)
	if err != nil {
		h.redirectWithFlash(w, r, "/", "error", "Pilih perusahaan aktif terlebih dahulu")
		return
	}
	fromID, _ := strconv.ParseInt(r.FormValue("from_account_id"), 10, 64)
	toID, _ := strconv.ParseInt(r.FormValue("to_account_id"), 10, 64)
	amount, _ := strconv.ParseFloat(r.FormValue("amount"), 64)
	dateStr := r.FormValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	desc := r.FormValue("description")
	ref := r.FormValue("reference")

	sess := shared.SessionFromContext(r.Context())
	userID := getUserID(sess)
	if err != nil || fromID <= 0 || toID <= 0 || amount <= 0 {
		h.redirectWithFlash(w, r, "/finance/banking/accounts", "error", "Data transfer tidak valid")
		return
	}
	fromAccount, err := h.service.repo.GetBankAccount(r.Context(), fromID)
	if err != nil || fromAccount.CompanyID != companyID {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}
	toAccount, err := h.service.repo.GetBankAccount(r.Context(), toID)
	if err != nil || toAccount.CompanyID != companyID {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}
	periodID, err := h.service.ResolveOpenPeriod(r.Context(), companyID, date)
	if err != nil {
		h.redirectWithFlash(w, r, "/finance/banking/accounts", "error", "Tidak ada periode akuntansi terbuka untuk tanggal transfer")
		return
	}

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
		h.redirectWithFlash(w, r, "/finance/banking/accounts", "error", shared.UserSafeMessage(err))
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

func activeCompanyID(r *http.Request) (int64, error) {
	sess := shared.SessionFromContext(r.Context())
	if sess == nil {
		return 0, strconv.ErrSyntax
	}
	companyID, err := strconv.ParseInt(sess.Get("company_id"), 10, 64)
	if err != nil || companyID <= 0 {
		return 0, strconv.ErrSyntax
	}
	return companyID, nil
}
