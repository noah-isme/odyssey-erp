package ap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOrchestrator_ProcessInvoice(t *testing.T) {
	ctx := context.Background()
	repo := &memoryAPRepo{
		invoices: make(map[int64]APInvoice),
		lines:    make(map[int64][]APInvoiceLine),
	}
	repo.invoices[1] = APInvoice{
		ID:        1,
		Status:    APStatusDraft,
		CreatedBy: 123,
	}
	// Setup services
	ms := NewMatchingService(repo)
	es := NewExceptionService(repo)
	as := NewService(repo, nil)
	orchestrator := NewOrchestrator(ms, es, as)

	err := orchestrator.ProcessInvoice(ctx, 1, 123)
	require.NoError(t, err) // It should succeed because matching is mocked to return MATCHED
}
