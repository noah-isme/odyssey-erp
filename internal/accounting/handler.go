package accounting

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/accounts"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/banks"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/reports"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// Handler wires finance ledger endpoints.
type Handler struct {
	logger         *slog.Logger
	templates      *view.Engine
	accountHandler *accounts.Handler
	accountService *accounts.Service
	journalHandler *journals.Handler
	banksHandler   *banks.Handler
	budgets        *sqlc.Queries
}

// NewHandler builds a Handler instance.
func NewHandler(logger *slog.Logger, db *pgxpool.Pool, templates *view.Engine, audit journals.AuditPort, guard journals.PeriodGuard) *Handler {
	// Repositories
	accountRepo := accounts.NewRepository(db)
	journalRepo := journals.NewRepository(db)

	// Services
	accountService := accounts.NewService(accountRepo)
	journalService := journals.NewService(journalRepo, audit, guard)
	bankService := banks.NewService(db)

	// Handlers
	accountHandler := accounts.NewHandler(logger, accountService, templates)
	journalHandler := journals.NewHandler(logger, journalService, templates)
	banksHandler := banks.NewHandler(logger, bankService, templates)

	return &Handler{
		logger:         logger,
		templates:      templates,
		accountHandler: accountHandler,
		accountService: accountService,
		journalHandler: journalHandler,
		banksHandler:   banksHandler,
		budgets:        sqlc.New(db),
	}
}

// MountRoutes registers HTTP routes for the ledger module.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Route("/coa", func(r chi.Router) {
		h.accountHandler.MountRoutes(r)
	})
	r.Route("/journals", func(r chi.Router) {
		h.journalHandler.MountRoutes(r)
	})
	r.Route("/banks", func(r chi.Router) {
		h.banksHandler.MountRoutes(r)
	})
	r.Get("/gl", h.handleGeneralLedger)
	r.Get("/trial-balance", h.handleTrialBalance)
	r.Get("/pnl", h.handleProfitLoss)
	r.Get("/balance-sheet", h.handleBalanceSheet)
	r.Get("/cash-flow", h.handleCashFlow)
	r.Get("/budget", h.handleBudget)

	r.Get("/finance/reports/trial-balance/pdf", h.handleNotImplemented)
	r.Get("/finance/reports/pl/pdf", h.handleNotImplemented)
	r.Get("/finance/reports/bs/pdf", h.handleNotImplemented)
}

func (h *Handler) handleGeneralLedger(w http.ResponseWriter, r *http.Request) {
	// Proxy to Account Service for List (MVP behavior)
	h.accountHandler.List(w, r)
}

func (h *Handler) handleTrialBalance(w http.ResponseWriter, r *http.Request) {
	h.accountHandler.List(w, r)
}

func (h *Handler) handleBalanceSheet(w http.ResponseWriter, r *http.Request) {
	h.accountHandler.List(w, r)
}

func (h *Handler) handleProfitLoss(w http.ResponseWriter, r *http.Request) {
	h.accountHandler.List(w, r)
}

func (h *Handler) handleNotImplemented(w http.ResponseWriter, _ *http.Request) {
	h.logger.Info("ledger handler invoked", slog.String("path", "finance"))
	shared.WriteHTTPError(w, http.StatusNotImplemented, "")
}

func (h *Handler) handleCashFlow(w http.ResponseWriter, r *http.Request) {
	balances, err := h.accountService.ListBalances(r.Context())
	if err != nil {
		h.logger.Error("list balances for cash flow", slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "Gagal memuat data laporan")
		return
	}

	cf := reports.BuildCashFlow(balances)
	viewData := view.TemplateData{
		Title: "Cash Flow",
		Data: map[string]any{
			"Report": cf,
		},
	}
	if err := h.templates.Render(w, "pages/finance/cashflow.html", viewData); err != nil {
		h.logger.Error("render cash flow", slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
	}
}

func (h *Handler) handleBudget(w http.ResponseWriter, r *http.Request) {
	year, month, err := budgetPeriod(r, time.Now())
	if err != nil {
		shared.WriteHTTPError(w, http.StatusBadRequest, "Periode harus berformat YYYY-MM")
		return
	}
	balances, err := h.accountService.ListBalancesForPeriod(r.Context(), year, month)
	if err != nil {
		h.logger.Error("list balances for budget", slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "Gagal memuat data laporan")
		return
	}

	rows, err := h.budgets.ListBudgetsByPeriod(r.Context(), sqlc.ListBudgetsByPeriodParams{PeriodYear: int32(year), PeriodMonth: int32(month)})
	if err != nil {
		h.logger.Error("list budgets", slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "Gagal memuat data laporan")
		return
	}
	budgetData := make(reports.BudgetData, len(rows))
	for _, row := range rows {
		amount, err := row.Amount.Float64Value()
		if err != nil || !amount.Valid {
			h.logger.Error("invalid budget amount", slog.Int64("budget_id", row.ID))
			shared.WriteHTTPError(w, http.StatusInternalServerError, "Gagal memuat data laporan")
			return
		}
		budgetData[row.AccountID] = amount.Float64
	}
	bva := reports.BuildBudgetVsActual(balances, budgetData)

	viewData := view.TemplateData{
		Title: "Budget vs Actual",
		Data: map[string]any{
			"Report":      bva,
			"Period":      time.Date(year, month, 1, 0, 0, 0, 0, time.UTC).Format("January 2006"),
			"PeriodValue": time.Date(year, month, 1, 0, 0, 0, 0, time.UTC).Format("2006-01"),
		},
	}
	if err := h.templates.Render(w, "pages/finance/budget.html", viewData); err != nil {
		h.logger.Error("render budget", slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
	}
}

func budgetPeriod(r *http.Request, now time.Time) (int, time.Month, error) {
	value := r.URL.Query().Get("period")
	if value == "" {
		return now.Year(), now.Month(), nil
	}
	parsed, err := time.Parse("2006-01", value)
	if err != nil {
		return 0, 0, err
	}
	return parsed.Year(), parsed.Month(), nil
}
