package main

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/jobs"
)

// newHRJobChangeApplier builds the employees service the daily
// hr:apply_job_changes job runs. It only needs the pool and the audit logger.
func newHRJobChangeApplier(pool *pgxpool.Pool) jobs.HRJobChangeApplier {
	return employees.NewService(pool, shared.NewAuditLogger(pool))
}
