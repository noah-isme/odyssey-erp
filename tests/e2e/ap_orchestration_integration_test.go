//go:build integration

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/ap"
)

func TestAPOrchestrationIntegration(t *testing.T) {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN is required for AP Orchestration suite")
	}
	applyAllMigrations(t, dsn)

	ctx := context.Background()
	p, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer p.Close()

	// 1. Setup Services
	repo := ap.NewRepository(p)
	// Passing nils for unused modules in orchestration context
	apSvc := ap.NewService(repo, nil)
	matchSvc := ap.NewMatchingService(repo)
	excSvc := ap.NewExceptionService(repo)
	orchestrator := ap.NewOrchestrator(matchSvc, excSvc, apSvc)

	// 2. Setup Test Data (PO and GRN)
	// For simplicity, we just create an unbacked invoice in draft
	// that will trigger MISSING_MAPPING because no PO/GRN matching policy exists
	inv, err := apSvc.CreateAPInvoice(ctx, ap.CreateAPInvoiceInput{
		SupplierID: 1, // Assume 1 exists from seed
		DueDate:    time.Now().AddDate(0, 1, 0),
		Lines: []ap.CreateAPInvoiceLineInput{
			{
				Description: "IT Services",
				Quantity:    1.0,
				UnitPrice:   1500.00,
			},
		},
		CreatedBy: 1,
	})
	require.NoError(t, err)

	// 3. Process the Invoice via Orchestrator
	err = orchestrator.ProcessInvoice(ctx, inv.ID, 1)
	require.NoError(t, err)

	// 4. Verify exception was created (MISSING_MAPPING) since no policy exists
	excs, err := repo.ListAPExceptions(ctx, "", 0, inv.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, excs, 1)
	require.Equal(t, "MISSING_MAPPING", excs[0].ExceptionType)

	// Verify Invoice is still DRAFT because it failed matching
	invCheck, err := repo.GetAPInvoice(ctx, inv.ID)
	require.NoError(t, err)
	require.Equal(t, ap.APStatusDraft, invCheck.Status)
}
