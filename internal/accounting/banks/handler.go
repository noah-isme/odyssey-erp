package banks

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type Renderer interface {
	Render(w http.ResponseWriter, name string, data view.TemplateData) error
}

type SessionManager interface {
	Put(ctx context.Context, key string, val any)
}

type Handler struct {
	logger   *slog.Logger
	service  *Service
	renderer Renderer
	csrf     *shared.CSRFManager
}

func NewHandler(logger *slog.Logger, service *Service, renderer Renderer, csrf *shared.CSRFManager) *Handler {
	return &Handler{
		logger:   logger,
		service:  service,
		renderer: renderer,
		csrf:     csrf,
	}
}

// csrfToken issues the token the statement import and reconciliation forms
// post back. Without it those forms render an empty field and every submit is
// rejected by the CSRF middleware.
func (h *Handler) csrfToken(r *http.Request) string {
	if h.csrf == nil {
		return ""
	}
	token, _ := h.csrf.EnsureToken(r.Context(), shared.SessionFromContext(r.Context()))
	return token
}

func (h *Handler) MountRoutes(r chi.Router) {
	r.Get("/statements", h.listStatements)
	r.Post("/statements/import", h.importStatement)
	r.Get("/statements/{id}", h.viewStatement)
	r.Post("/statements/{id}/confirm", h.confirmStatement)
	r.Post("/transfer", h.transferFunds)
}

func (h *Handler) transferFunds(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	fromID, _ := strconv.ParseInt(r.FormValue("from_account_id"), 10, 64)
	toID, _ := strconv.ParseInt(r.FormValue("to_account_id"), 10, 64)
	amount, _ := strconv.ParseFloat(r.FormValue("amount"), 64)
	dateStr := r.FormValue("transfer_date")
	transferDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		transferDate = time.Now()
	}

	req := TransferRequest{
		FromBankAccountID: fromID,
		ToBankAccountID:   toID,
		Amount:            amount,
		TransferDate:      transferDate,
		Reference:         r.FormValue("reference"),
		Notes:             r.FormValue("notes"),
		CreatedBy:         1, // Hardcoded for MVP
	}

	err = h.service.PerformTransfer(r.Context(), req)
	if err != nil {
		h.logger.Error("perform transfer", slog.Any("error", err))
		shared.WriteErrorStatus(w, http.StatusInternalServerError, err)
		return
	}

	// Redirect to bank accounts list (assuming it exists, otherwise dashboard)
	http.Redirect(w, r, "/finance/banking/accounts", http.StatusSeeOther)
}

func (h *Handler) confirmStatement(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid statement ID", http.StatusBadRequest)
		return
	}

	err = h.service.ConfirmStatement(r.Context(), id)
	if err != nil {
		h.logger.Error("confirm statement", slog.Any("error", err))
		shared.WriteErrorStatus(w, http.StatusInternalServerError, err)
		return
	}

	http.Redirect(w, r, "/accounting/banks/statements/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (h *Handler) listStatements(w http.ResponseWriter, r *http.Request) {
	sess := shared.SessionFromContext(r.Context())
	if sess == nil {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	companyID, err := strconv.ParseInt(sess.Get("company_id"), 10, 64)
	if err != nil || companyID <= 0 {
		http.Error(w, "Active company is required", http.StatusBadRequest)
		return
	}
	var statements []BankStatement
	accountID, err := h.service.BankAccountIDForCompany(r.Context(), companyID)
	if err == nil {
		statements, _ = h.service.ListStatements(r.Context(), accountID, 50, 0)
	}

	data := view.TemplateData{
		Title:       "Bank Statements",
		CurrentPath: "/accounting/banks/statements",
		CSRFToken:   h.csrfToken(r),
		Data: map[string]any{
			"Statements": statements,
		},
	}
	if err := h.renderer.Render(w, "pages/accounting/bank_statements.html", data); err != nil {
		h.logger.Error("render bank statements", slog.Any("error", err))
	}
}

func (h *Handler) importStatement(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 << 20) // 10MB
	if err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("statement_csv")
	if err != nil {
		http.Error(w, "CSV file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	sess := shared.SessionFromContext(r.Context())
	companyID, _ := strconv.ParseInt(sess.Get("company_id"), 10, 64)
	accountID, err := h.service.BankAccountIDForCompany(r.Context(), companyID)
	if err != nil {
		h.logger.Error("find bank account", slog.Any("error", err), slog.Int64("company_id", companyID))
		shared.WriteErrorStatus(w, http.StatusBadRequest, err)
		return
	}

	dateStr := r.FormValue("statement_date")
	statementDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		statementDate = time.Now()
	}

	lines, err := ParseCSV(file)
	if err != nil {
		h.logger.Error("parse csv", slog.Any("error", err))
		shared.WriteErrorStatus(w, http.StatusBadRequest, err)
		return
	}

	stmtID, err := h.service.ImportStatement(r.Context(), accountID, statementDate, lines)
	if err != nil {
		h.logger.Error("import statement", slog.Any("error", err))
		http.Error(w, "Failed to import statement", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/accounting/banks/statements/"+strconv.FormatInt(stmtID, 10), http.StatusSeeOther)
}

func (h *Handler) viewStatement(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid statement ID", http.StatusBadRequest)
		return
	}

	stmt, err := h.service.GetStatement(r.Context(), id)
	if err != nil {
		h.logger.Error("get statement", slog.Any("error", err))
		shared.WriteErrorStatus(w, http.StatusNotFound, err)
		return
	}

	lines, err := h.service.ListStatementLines(r.Context(), id)
	if err != nil {
		h.logger.Error("list lines", slog.Any("error", err))
		shared.WriteErrorStatus(w, http.StatusInternalServerError, err)
		return
	}

	data := view.TemplateData{
		Title:       "Bank Reconciliation",
		CurrentPath: "/accounting/banks/statements",
		CSRFToken:   h.csrfToken(r),
		Data: map[string]any{
			"Statement": stmt,
			"Lines":     lines,
		},
	}
	if err := h.renderer.Render(w, "pages/accounting/bank_reconciliation.html", data); err != nil {
		h.logger.Error("render bank reconciliation", slog.Any("error", err))
	}
}
