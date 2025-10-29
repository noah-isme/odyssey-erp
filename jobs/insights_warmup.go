package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/analytics"
	jobmetrics "github.com/odyssey-erp/odyssey-erp/internal/jobs"
)

var defaultJobMetrics = jobmetrics.NewMetrics(nil)

// InsightsWarmupJob pre-populates analytics caches for active finance scopes.
type InsightsWarmupJob struct {
	Analytics *analytics.Service
	Pool      *pgxpool.Pool
	Logger    *slog.Logger
	Metrics   *jobmetrics.Metrics
	clock     func() time.Time
}

// NewInsightsWarmupJob wires dependencies for the warmup handler.
func NewInsightsWarmupJob(analyticsSvc *analytics.Service, pool *pgxpool.Pool, logger *slog.Logger, metrics *jobmetrics.Metrics) *InsightsWarmupJob {
	return &InsightsWarmupJob{
		Analytics: analyticsSvc,
		Pool:      pool,
		Logger:    logger,
		Metrics:   metrics,
		clock: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// Handle processes analytics warmup tasks.
func (j *InsightsWarmupJob) Handle(ctx context.Context, t *asynq.Task) error {
	if j == nil {
		return errors.New("insights warmup: handler not configured")
	}
	var payload InsightsWarmupPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return asynq.SkipRetry
	}
	if payload.PeriodScope == "" {
		payload.PeriodScope = "active"
	}

	tracker := j.metrics().Track(TaskAnalyticsInsightsWarmup)
	var resultErr error
	defer func() {
		resultErr = tracker.End(resultErr)
	}()

	logger := j.logger().With(slog.String("period_scope", payload.PeriodScope))
	logger.Info("starting insights warmup")

	scopes, err := j.fetchScopes(ctx)
	if err != nil {
		resultErr = err
		logger.Error("load warmup scopes", slog.Any("error", err))
		return resultErr
	}
	if len(scopes) == 0 {
		logger.Info("no scopes discovered for warmup")
		return resultErr
	}

	now := j.now()
	warmed := 0
	for _, scope := range scopes {
		if err := j.warmScope(ctx, scope, now); err != nil {
			resultErr = err
			logger.Error("warm scope", slog.Int64("company_id", scope.CompanyID), slog.Int64("branch_id", scope.BranchValue()), slog.Any("error", err))
			return resultErr
		}
		warmed++
	}

	logger.Info("completed insights warmup", slog.Int("scopes", warmed), slog.Duration("duration", time.Since(now)))
	return resultErr
}

func (j *InsightsWarmupJob) warmScope(ctx context.Context, scope warmupScope, now time.Time) error {
	if j.Analytics == nil {
		return nil
	}
	// Tighten each scope execution with a timeout to avoid long-running jobs.
	scopeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	period := now.Format("2006-01")
	from := now.AddDate(0, -5, 0).Format("2006-01")

	if _, err := j.Analytics.GetKPISummary(scopeCtx, analytics.KPIFilter{
		Period:    period,
		CompanyID: scope.CompanyID,
		BranchID:  scope.BranchID,
		AsOf:      now,
	}); err != nil {
		return err
	}
	trendFilter := analytics.TrendFilter{From: from, To: period, CompanyID: scope.CompanyID, BranchID: scope.BranchID}
	if _, err := j.Analytics.GetPLTrend(scopeCtx, trendFilter); err != nil {
		return err
	}
	if _, err := j.Analytics.GetCashflowTrend(scopeCtx, trendFilter); err != nil {
		return err
	}
	agingFilter := analytics.AgingFilter{AsOf: now, CompanyID: scope.CompanyID, BranchID: scope.BranchID}
	if _, err := j.Analytics.GetARAging(scopeCtx, agingFilter); err != nil {
		return err
	}
	if _, err := j.Analytics.GetAPAging(scopeCtx, agingFilter); err != nil {
		return err
	}
	return nil
}

func (j *InsightsWarmupJob) fetchScopes(ctx context.Context) ([]warmupScope, error) {
	if j.Pool == nil {
		return nil, errors.New("insights warmup: pool not configured")
	}
	rows, err := j.Pool.Query(ctx, `SELECT DISTINCT company_id, branch_id FROM mv_pl_monthly WHERE company_id > 0 ORDER BY company_id, branch_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	scopes := make([]warmupScope, 0)
	for rows.Next() {
		var companyID int64
		var branchID int64
		if err := rows.Scan(&companyID, &branchID); err != nil {
			return nil, err
		}
		scope := warmupScope{CompanyID: companyID}
		if branchID > 0 {
			scope.BranchID = new(int64)
			*scope.BranchID = branchID
		}
		scopes = append(scopes, scope)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return scopes, nil
}

func (j *InsightsWarmupJob) logger() *slog.Logger {
	if j.Logger != nil {
		return j.Logger.With(slog.String("job", TaskAnalyticsInsightsWarmup))
	}
	return slog.Default().With(slog.String("job", TaskAnalyticsInsightsWarmup))
}

func (j *InsightsWarmupJob) metrics() *jobmetrics.Metrics {
	if j.Metrics != nil {
		return j.Metrics
	}
	return defaultJobMetrics
}

func (j *InsightsWarmupJob) now() time.Time {
	if j.clock != nil {
		return j.clock()
	}
	return time.Now().UTC()
}

type warmupScope struct {
	CompanyID int64
	BranchID  *int64
}

func (s warmupScope) BranchValue() int64 {
	if s.BranchID == nil {
		return 0
	}
	return *s.BranchID
}
