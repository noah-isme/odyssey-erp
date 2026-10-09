package main

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/mappings"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/periods"
	"github.com/odyssey-erp/odyssey-erp/internal/ap"
	closepkg "github.com/odyssey-erp/odyssey-erp/internal/close"
	"github.com/odyssey-erp/odyssey-erp/internal/integration"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// newWorkerAPStack builds the AP service and orchestrator behind the
// ap:invoice_process task. On a MATCHED run the orchestrator posts the
// invoice, so the service needs the same ledger hooks, audit logger and tax
// capture as cmd/odyssey; without them the invoice is posted with no journal
// and no audit row. GRN/PO lookups (the procurement service) are not used on
// this path.
func newWorkerAPStack(pool *pgxpool.Pool, taxService ap.TaxServicePort) (*ap.Service, *ap.Orchestrator) {
	auditLogger := shared.NewAuditLogger(pool)
	journalService := journals.NewService(journals.NewRepository(pool), auditLogger, closepkg.NewService(closepkg.NewRepository(pool)))
	hooks := integration.NewHooks(journalService, periods.NewRepository(pool), mappings.NewRepository(pool))

	apRepo := ap.NewRepository(pool)
	apService := ap.NewService(apRepo, nil)
	apService.SetIntegrationHandler(hooks)
	apService.SetAuditLogger(auditLogger)
	apService.SetTaxService(taxService)
	return apService, ap.NewOrchestrator(ap.NewMatchingService(apRepo), ap.NewExceptionService(apRepo), apService, apRepo)
}
