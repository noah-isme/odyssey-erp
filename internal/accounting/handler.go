package accounting

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/accounts"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/view"

)

// Handler wires finance ledger endpoints.
type Handler struct {
	logger         *slog.Logger
	templates      *view.Engine
	accountHandler *accounts.Handler
	journalHandler *journals.Handler
	// Future: ReportHandler
}

// NewHandler builds a Handler instance.
// Note: Dependencies like audit and guard should be injected here or created if simple.
// For now, assuming they are passed or we create separate constructors.
func NewHandler(logger *slog.Logger, db *pgxpool.Pool, templates *view.Engine, audit journals.AuditPort, guard journals.PeriodGuard) *Handler {
	// Repositories
	accountRepo := accounts.NewRepository(db)
	journalRepo := journals.NewRepository(db)
	// periodRepo := periods.NewRepository(db)
	// mappingRepo := mappings.NewRepository(db)

	// Services
	accountService := accounts.NewService(accountRepo)
	journalService := journals.NewService(journalRepo, audit, guard)

	// Handlers
	accountHandler := accounts.NewHandler(logger, accountService, templates)
	journalHandler := journals.NewHandler(logger, journalService, templates)

	return &Handler{
		logger:         logger,
		templates:      templates,
		accountHandler: accountHandler,
		journalHandler: journalHandler,
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

	// Legacy/Direct routes for now until Report module is fully separated
	r.Get("/gl", h.handleGeneralLedger)
	r.Get("/trial-balance", h.handleTrialBalance)
	r.Get("/pnl", h.handleProfitLoss)
	r.Get("/balance-sheet", h.handleBalanceSheet)

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
	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
}
