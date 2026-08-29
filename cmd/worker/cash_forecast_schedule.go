package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
	"github.com/odyssey-erp/odyssey-erp/jobs"
)

// cashForecastScheduleStore adapts the generated company/scenario queries and
// the company-scoped automation settings to the narrow jobs contract. Keeping
// this adapter in worker composition avoids exposing generated SQL types to the
// task handler.
type cashForecastScheduleStore struct {
	queries  *sqlc.Queries
	settings automation.SettingsStore
}

func newCashForecastScheduleStore(pool *pgxpool.Pool) *cashForecastScheduleStore {
	if pool == nil {
		return nil
	}
	return &cashForecastScheduleStore{
		queries:  sqlc.New(pool),
		settings: automation.NewRepository(pool),
	}
}

func (s *cashForecastScheduleStore) ListCompanies(ctx context.Context) ([]jobs.CashForecastScheduleCompany, error) {
	if s == nil || s.queries == nil {
		return nil, errors.New("cash forecast schedule: company query is not configured")
	}
	rows, err := s.queries.ListCompanies(ctx)
	if err != nil {
		return nil, err
	}
	companies := make([]jobs.CashForecastScheduleCompany, 0, len(rows))
	for _, row := range rows {
		companies = append(companies, jobs.CashForecastScheduleCompany{ID: row.ID})
	}
	return companies, nil
}

func (s *cashForecastScheduleStore) CashForecastEnabled(ctx context.Context, companyID int64) (bool, error) {
	if s == nil || s.settings == nil {
		return false, errors.New("cash forecast schedule: settings store is not configured")
	}
	if companyID <= 0 {
		return false, fmt.Errorf("cash forecast schedule: company ID is required")
	}
	settings, err := s.settings.Settings(ctx, companyID)
	if err != nil {
		return false, err
	}
	if settings.CompanyID != companyID {
		return false, fmt.Errorf("cash forecast schedule: settings company mismatch")
	}
	return settings.CashForecastEnabled, nil
}

func (s *cashForecastScheduleStore) ListForecastScenarios(ctx context.Context, companyID int64) ([]jobs.CashForecastScheduleScenario, error) {
	if s == nil || s.queries == nil {
		return nil, errors.New("cash forecast schedule: scenario query is not configured")
	}
	if companyID <= 0 {
		return nil, fmt.Errorf("cash forecast schedule: company ID is required")
	}
	rows, err := s.queries.ListForecastScenarios(ctx, companyID)
	if err != nil {
		return nil, err
	}
	scenarios := make([]jobs.CashForecastScheduleScenario, 0, len(rows))
	for _, row := range rows {
		scenarios = append(scenarios, jobs.CashForecastScheduleScenario{ID: row.ID, CompanyID: row.CompanyID})
	}
	return scenarios, nil
}
