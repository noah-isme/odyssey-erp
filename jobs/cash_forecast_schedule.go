package jobs

import (
	"context"
	"errors"
	"fmt"

	"github.com/hibiken/asynq"
)

// CashForecastScheduleCompany is the minimal company identity needed by the
// nightly scanner. It deliberately carries no provider or credential data.
type CashForecastScheduleCompany struct {
	ID int64
}

// CashForecastScheduleScenario is a company-owned forecast scenario returned
// by the scheduler store.
type CashForecastScheduleScenario struct {
	ID        int64
	CompanyID int64
}

// CashForecastScheduleStore enumerates the persisted scope for nightly
// refreshes. Implementations must return scenarios filtered by company ID and
// the enabled flag must come from the same company-scoped settings row.
type CashForecastScheduleStore interface {
	ListCompanies(context.Context) ([]CashForecastScheduleCompany, error)
	CashForecastEnabled(context.Context, int64) (bool, error)
	ListForecastScenarios(context.Context, int64) ([]CashForecastScheduleScenario, error)
}

// CashForecastRefreshEnqueuer submits one already-scoped scenario refresh.
type CashForecastRefreshEnqueuer func(context.Context, int64, int64) error

// HandleCashForecastRefreshScanTask expands the nightly scan into one task
// per enabled company/scenario pair. Invalid scope data is non-retryable so a
// malformed row cannot be retried indefinitely or broaden into a wildcard.
func HandleCashForecastRefreshScanTask(store CashForecastScheduleStore, enqueue CashForecastRefreshEnqueuer) asynq.HandlerFunc {
	return func(ctx context.Context, task *asynq.Task) error {
		if store == nil {
			return fmt.Errorf("cash forecast refresh scan: store not configured: %w", asynq.SkipRetry)
		}
		if enqueue == nil {
			return fmt.Errorf("cash forecast refresh scan: enqueuer not configured: %w", asynq.SkipRetry)
		}
		if task == nil {
			return fmt.Errorf("cash forecast refresh scan: task not configured: %w", asynq.SkipRetry)
		}

		companies, err := store.ListCompanies(ctx)
		if err != nil {
			return err
		}
		seenCompanies := make(map[int64]struct{}, len(companies))
		for _, company := range companies {
			if company.ID <= 0 {
				return fmt.Errorf("cash forecast refresh scan: invalid company ID: %w", asynq.SkipRetry)
			}
			if _, duplicate := seenCompanies[company.ID]; duplicate {
				continue
			}
			seenCompanies[company.ID] = struct{}{}

			enabled, err := store.CashForecastEnabled(ctx, company.ID)
			if err != nil {
				return fmt.Errorf("cash forecast refresh scan: company %d settings: %w", company.ID, err)
			}
			if !enabled {
				continue
			}

			scenarios, err := store.ListForecastScenarios(ctx, company.ID)
			if err != nil {
				return fmt.Errorf("cash forecast refresh scan: company %d scenarios: %w", company.ID, err)
			}
			seenScenarios := make(map[int64]struct{}, len(scenarios))
			for _, scenario := range scenarios {
				if scenario.ID <= 0 || scenario.CompanyID != company.ID {
					return fmt.Errorf("cash forecast refresh scan: scenario %d is outside company %d scope: %w", scenario.ID, company.ID, asynq.SkipRetry)
				}
				if _, duplicate := seenScenarios[scenario.ID]; duplicate {
					continue
				}
				seenScenarios[scenario.ID] = struct{}{}
				if err := enqueue(ctx, company.ID, scenario.ID); err != nil {
					if errors.Is(err, asynq.ErrTaskIDConflict) {
						continue
					}
					return fmt.Errorf("cash forecast refresh scan: enqueue company %d scenario %d: %w", company.ID, scenario.ID, err)
				}
			}
		}
		return nil
	}
}
