package main

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
	"github.com/odyssey-erp/odyssey-erp/jobs"
)

// bankFeedScheduleStore keeps the scheduler's due query in the worker
// composition layer. The query joins the company-owned feature setting,
// connection state, consent, and latest sync run before returning IDs to the
// generic scan handler.
type bankFeedScheduleStore struct {
	queries *sqlc.Queries
}

func newBankFeedScheduleStore(pool *pgxpool.Pool) *bankFeedScheduleStore {
	if pool == nil {
		return nil
	}
	return &bankFeedScheduleStore{queries: sqlc.New(pool)}
}

func (s *bankFeedScheduleStore) ListDueBankFeedConnections(ctx context.Context) ([]jobs.BankFeedScheduleConnection, error) {
	if s == nil || s.queries == nil {
		return nil, errors.New("bank feed schedule: store is not configured")
	}
	rows, err := s.queries.ListDueBankFeedConnections(ctx)
	if err != nil {
		return nil, err
	}
	connections := make([]jobs.BankFeedScheduleConnection, 0, len(rows))
	for _, row := range rows {
		connections = append(connections, jobs.BankFeedScheduleConnection{
			ID:        row.ID,
			CompanyID: row.CompanyID,
		})
	}
	return connections, nil
}
