package reporting

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

type Runner struct {
	q    *sqlc.Queries
	pool *pgxpool.Pool
}

func NewRunner(q *sqlc.Queries, pool *pgxpool.Pool) *Runner {
	return &Runner{
		q:    q,
		pool: pool,
	}
}

func (r *Runner) RunQuery(ctx context.Context, companyID, datasetID, actorID uuid.UUID, plan *CompiledPlan) error {
	// 1. Create Run Metadata
	run, err := r.q.CreateReportRun(ctx, sqlc.CreateReportRunParams{
		CompanyID:         pgtype.UUID{Bytes: companyID, Valid: true},
		DatasetID:         pgtype.UUID{Bytes: datasetID, Valid: true},
		ActorID:           pgtype.UUID{Bytes: actorID, Valid: true},
		Status:            "RUNNING",
		QueryCostEstimate: pgtype.Int4{Int32: int32(plan.CostLimit), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to create run metadata: %w", err)
	}

	start := time.Now()

	// 2. Execute Query with quotas
	// Enforce a hard timeout limit using context
	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	rows, execErr := r.pool.Query(execCtx, plan.SQL, plan.Args...)

	// Collect stats
	execDuration := time.Since(start)

	var rowCount int32
	if execErr == nil {
		for rows.Next() {
			rowCount++
		}
		rows.Close()
		execErr = rows.Err()
	}

	// 3. Update Run Metadata
	status := "COMPLETED"
	var errMsg string
	if execErr != nil {
		status = "FAILED"
		errMsg = execErr.Error()
	}

	updateErr := r.q.UpdateReportRunStatus(ctx, sqlc.UpdateReportRunStatusParams{
		ID:              run.ID,
		Status:          status,
		RowCount:        pgtype.Int4{Int32: rowCount, Valid: true},
		ErrorMessage:    pgtype.Text{String: errMsg, Valid: errMsg != ""},
		ExecutedSql:     pgtype.Text{String: plan.SQL, Valid: true},
		ExecutionTimeMs: pgtype.Int4{Int32: int32(execDuration.Milliseconds()), Valid: true},
		StartedAt:       pgtype.Timestamptz{Time: start, Valid: true},
		CompletedAt:     pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})

	if updateErr != nil {
		return fmt.Errorf("failed to update run status: %w (exec err: %v)", updateErr, execErr)
	}

	if execErr != nil {
		return fmt.Errorf("query execution failed: %w", execErr)
	}

	return nil
}
