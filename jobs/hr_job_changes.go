package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
)

// TaskHRApplyJobChanges mirrors effective-dated HR job changes that have come
// due onto hr_employees.
const TaskHRApplyJobChanges = "hr:apply_job_changes"

// HRJobChangeApplier applies the job records effective on asOf. It returns the
// number of employees updated and must be idempotent.
type HRJobChangeApplier interface {
	ApplyDueJobChanges(ctx context.Context, asOf time.Time) (int, error)
}

// hrJobChangeClock supplies "today" for the job; tests replace it.
var hrJobChangeClock = time.Now

// HandleHRApplyJobChanges applies job changes due as of the current calendar
// date in location (Asia/Jakarta when nil, matching the FX daily-rates job and
// the employee directory). Errors are returned so asynq retries; the
// operation is idempotent.
func HandleHRApplyJobChanges(applier HRJobChangeApplier, location *time.Location) asynq.HandlerFunc {
	return func(ctx context.Context, _ *asynq.Task) error {
		if applier == nil {
			return fmt.Errorf("HR job change applier not configured: %w", asynq.SkipRetry)
		}
		loc := location
		if loc == nil {
			loc, _ = time.LoadLocation("Asia/Jakarta")
		}
		now := hrJobChangeClock()
		if loc != nil {
			now = now.In(loc)
		}
		n, err := applier.ApplyDueJobChanges(ctx, now)
		if n > 0 {
			slog.InfoContext(ctx, "applied due HR job changes", slog.Int("employees", n))
		}
		return err
	}
}
