package reporting

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type ReportRunRepository interface {
	CreateReportRun(ctx context.Context, input ReportRunCreateInput) (uuid.UUID, error)
	UpdateReportRunStatus(ctx context.Context, update ReportRunStatusUpdate) error
}

type QueryExecutor interface {
	CountRows(ctx context.Context, query string, args ...any) (int, error)
}

type Runner struct {
	repo     ReportRunRepository
	executor QueryExecutor
}

func NewRunner(repo ReportRunRepository, executor QueryExecutor) *Runner {
	return &Runner{
		repo:     repo,
		executor: executor,
	}
}

func (r *Runner) RunQuery(ctx context.Context, companyID, datasetID, actorID uuid.UUID, plan *CompiledPlan) error {
	// 1. Create Run Metadata
	runID, err := r.repo.CreateReportRun(ctx, ReportRunCreateInput{
		CompanyID:         companyID,
		DatasetID:         datasetID,
		ActorID:           actorID,
		Status:            "RUNNING",
		QueryCostEstimate: plan.CostLimit,
	})
	if err != nil {
		return fmt.Errorf("failed to create run metadata: %w", err)
	}

	start := time.Now()

	// 2. Execute Query with quotas
	// Enforce a hard timeout limit using context
	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	rowCount, execErr := r.executor.CountRows(execCtx, plan.SQL, plan.Args...)
	execDuration := time.Since(start)

	// 3. Update Run Metadata
	status := "COMPLETED"
	var errMsg string
	if execErr != nil {
		status = "FAILED"
		errMsg = execErr.Error()
	}

	updateErr := r.repo.UpdateReportRunStatus(ctx, ReportRunStatusUpdate{
		ID:              runID,
		Status:          status,
		RowCount:        rowCount,
		ErrorMessage:    errMsg,
		ExecutedSQL:     plan.SQL,
		ExecutionTimeMS: int(execDuration.Milliseconds()),
		StartedAt:       start,
		CompletedAt:     time.Now(),
	})

	if updateErr != nil {
		return fmt.Errorf("failed to update run status: %w (exec err: %v)", updateErr, execErr)
	}

	if execErr != nil {
		return fmt.Errorf("query execution failed: %w", execErr)
	}

	return nil
}
