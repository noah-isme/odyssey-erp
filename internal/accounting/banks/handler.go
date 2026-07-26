package banks

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
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
		http.Error(w, "Failed to perform transfer: "+err.Error(), http.StatusInternalServerError)
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
		http.Error(w, "Failed to confirm statement: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/accounting/banks/statements/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (h *Handler) listStatements(w http.ResponseWriter, r *http.Request) {
	// For MVP, assume bank account ID 1
	var accountID int64 = 1

	statements, err := h.service.queries.ListBankStatements(r.Context(), sqlc.ListBankStatementsParams{
		BankAccountID: accountID,
		Limit:         50,
		Offset:        0,
	})
	if err != nil {
		h.logger.Error("list statements", slog.Any("error", err))
		http.Error(w, "Failed to load statements", http.StatusInternalServerError)
		return
	}

	data := view.TemplateData{
		CSRFToken: h.csrfToken(r),
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
	defer func() {
		if err := file.Close(); err != nil {
			h.logger.Warn("close statement upload", slog.Any("error", err))
		}
	}()

	// Hardcode account ID and date for MVP, ideally comes from form
	var accountID int64 = 1
	dateStr := r.FormValue("statement_date")
	statementDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		statementDate = time.Now()
	}

	lines, err := ParseCSV(file)
	if err != nil {
		h.logger.Error("parse csv", slog.Any("error", err))
		http.Error(w, "Invalid CSV format: "+err.Error(), http.StatusBadRequest)
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

	stmt, err := h.service.queries.GetBankStatement(r.Context(), id)
	if err != nil {
		h.logger.Error("get statement", slog.Any("error", err))
		http.Error(w, "Statement not found", http.StatusNotFound)
		return
	}

	lines, err := h.service.queries.ListBankStatementLines(r.Context(), id)
	if err != nil {
		h.logger.Error("list lines", slog.Any("error", err))
		http.Error(w, "Failed to load statement lines", http.StatusInternalServerError)
		return
	}

	data := view.TemplateData{
		CSRFToken: h.csrfToken(r),
		Data: map[string]any{
			"Statement": stmt,
			"Lines":     lines,
		},
	}
	if err := h.renderer.Render(w, "pages/accounting/bank_reconciliation.html", data); err != nil {
		h.logger.Error("render bank reconciliation", slog.Any("error", err))
	}
}
