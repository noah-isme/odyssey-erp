package jobs

import (
	"context"
	"errors"
	"fmt"

	"github.com/hibiken/asynq"
)

// BankFeedScheduleConnection is the minimum identity returned by the
// company-scoped due-connection query. Keeping the company ID in the scan
// result lets the worker reject a malformed or cross-tenant projection before
// it can enqueue work.
type BankFeedScheduleConnection struct {
	ID        int64
	CompanyID int64
}

// BankFeedScheduleStore returns only active, consent-valid connections whose
// company setting and latest sync state make them due. Implementations must
// apply the company/settings join in their query; the task carries no
// wildcard that could be expanded by a provider adapter.
type BankFeedScheduleStore interface {
	ListDueBankFeedConnections(context.Context) ([]BankFeedScheduleConnection, error)
}

// BankFeedsSyncEnqueuer submits one already-scoped connection sync.
type BankFeedsSyncEnqueuer func(context.Context, int64) error

// HandleBankFeedsSyncScanTask expands a scheduled scan into stable,
// connection-scoped sync tasks. Duplicate rows are harmless, while conflicting
// company identities for one connection fail closed because they indicate a
// broken tenant projection.
func HandleBankFeedsSyncScanTask(store BankFeedScheduleStore, enqueue BankFeedsSyncEnqueuer) asynq.HandlerFunc {
	return func(ctx context.Context, task *asynq.Task) error {
		if store == nil {
			return fmt.Errorf("bank feed sync scan: store not configured: %w", asynq.SkipRetry)
		}
		if enqueue == nil {
			return fmt.Errorf("bank feed sync scan: enqueuer not configured: %w", asynq.SkipRetry)
		}
		if task == nil {
			return fmt.Errorf("bank feed sync scan: task not configured: %w", asynq.SkipRetry)
		}

		connections, err := store.ListDueBankFeedConnections(ctx)
		if err != nil {
			return err
		}
		seen := make(map[int64]int64, len(connections))
		due := make([]BankFeedScheduleConnection, 0, len(connections))
		for _, connection := range connections {
			if connection.ID <= 0 || connection.CompanyID <= 0 {
				return fmt.Errorf("bank feed sync scan: invalid connection scope (%d,%d): %w", connection.ID, connection.CompanyID, asynq.SkipRetry)
			}
			if previousCompany, duplicate := seen[connection.ID]; duplicate {
				if previousCompany != connection.CompanyID {
					return fmt.Errorf("bank feed sync scan: connection %d has conflicting companies %d and %d: %w", connection.ID, previousCompany, connection.CompanyID, asynq.SkipRetry)
				}
				continue
			}
			seen[connection.ID] = connection.CompanyID
			due = append(due, connection)
		}

		for _, connection := range due {
			if err := enqueue(ctx, connection.ID); err != nil {
				if errors.Is(err, asynq.ErrTaskIDConflict) {
					continue
				}
				return fmt.Errorf("bank feed sync scan: enqueue connection %d: %w", connection.ID, err)
			}
		}
		return nil
	}
}
