package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"

	jobmetrics "github.com/odyssey-erp/odyssey-erp/internal/jobs"
)

// AnomalyScanJob inspects finance aggregates looking for significant deltas.
type AnomalyScanJob struct {
	Pool    *pgxpool.Pool
	Logger  *slog.Logger
	Metrics *jobmetrics.Metrics
	clock   func() time.Time
}

// NewAnomalyScanJob initialises the anomaly scan handler.
func NewAnomalyScanJob(pool *pgxpool.Pool, logger *slog.Logger, metrics *jobmetrics.Metrics) *AnomalyScanJob {
	return &AnomalyScanJob{
		Pool:    pool,
		Logger:  logger,
		Metrics: metrics,
		clock: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// Handle executes the anomaly scan logic.
func (j *AnomalyScanJob) Handle(ctx context.Context, t *asynq.Task) error {
	if j == nil {
		return errors.New("anomaly scan: handler not configured")
	}
	var payload AnomalyScanPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return asynq.SkipRetry
	}
	if payload.WindowMonths <= 0 {
		payload.WindowMonths = 12
	}
	if payload.Z <= 0 {
		payload.Z = 2.5
	}

	start := j.now()
	tracker := j.metrics().Track(TaskAnalyticsAnomalyScan)
	var resultErr error
	defer func() {
		resultErr = tracker.End(resultErr)
	}()

	logger := j.logger().With(
		slog.Int("window_months", payload.WindowMonths),
		slog.Float64("z_threshold", payload.Z),
	)
	logger.Info("starting anomaly scan")

	scopes, anomalies, err := j.scan(ctx, payload, start)
	if err != nil {
		resultErr = err
		logger.Error("scan failed", slog.Any("error", err))
		return resultErr
	}

	for _, a := range anomalies {
		logger.Warn("finance anomaly detected",
			slog.Int64("company_id", a.CompanyID),
			slog.Int64("branch_id", a.BranchID),
			slog.String("period", a.Period),
			slog.String("severity", a.Severity),
			slog.Float64("z_score", a.ZScore),
			slog.Float64("delta", a.Delta),
		)
		j.metrics().AddAnomalies(a.Severity, a.CompanyID, a.BranchID, 1)
	}

	logger.Info("completed anomaly scan",
		slog.Int("scopes", scopes),
		slog.Int("anomalies", len(anomalies)),
		slog.Duration("duration", time.Since(start)),
	)
	return resultErr
}

func (j *AnomalyScanJob) scan(ctx context.Context, payload AnomalyScanPayload, now time.Time) (int, []scanAnomaly, error) {
	if j.Pool == nil {
		return 0, nil, errors.New("anomaly scan: pool not configured")
	}
	from := now.AddDate(0, -payload.WindowMonths+1, 0).Format("2006-01")
	rows, err := j.Pool.Query(ctx, `SELECT company_id, branch_id, period, net::double precision FROM mv_pl_monthly WHERE company_id > 0 AND period >= $1 ORDER BY company_id, branch_id, period`, from)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()

	series := make(map[string]*timeSeries)
	for rows.Next() {
		var companyID int64
		var branchID int64
		var period string
		var net float64
		if err := rows.Scan(&companyID, &branchID, &period, &net); err != nil {
			return 0, nil, err
		}
		key := fmt.Sprintf("%d:%d", companyID, branchID)
		entry, ok := series[key]
		if !ok {
			entry = &timeSeries{CompanyID: companyID, BranchID: branchID}
			series[key] = entry
		}
		entry.Periods = append(entry.Periods, period)
		entry.Values = append(entry.Values, net)
	}
	if err := rows.Err(); err != nil {
		return 0, nil, err
	}

	anomalies := make([]scanAnomaly, 0)
	for _, entry := range series {
		if len(entry.Values) < 3 {
			continue
		}
		mean := average(entry.Values)
		stddev := std(entry.Values, mean)
		if stddev == 0 {
			continue
		}
		last := entry.Values[len(entry.Values)-1]
		zscore := math.Abs((last - mean) / stddev)
		severity := ""
		switch {
		case zscore >= payload.Z:
			severity = "HIGH"
		case zscore >= payload.Z*0.6:
			severity = "MEDIUM"
		default:
			continue
		}
		anomalies = append(anomalies, scanAnomaly{
			CompanyID: entry.CompanyID,
			BranchID:  entry.BranchID,
			Period:    entry.Periods[len(entry.Periods)-1],
			Severity:  severity,
			ZScore:    zscore,
			Delta:     last - mean,
		})
	}

	return len(series), anomalies, nil
}

func (j *AnomalyScanJob) logger() *slog.Logger {
	if j.Logger != nil {
		return j.Logger.With(slog.String("job", TaskAnalyticsAnomalyScan))
	}
	return slog.Default().With(slog.String("job", TaskAnalyticsAnomalyScan))
}

func (j *AnomalyScanJob) metrics() *jobmetrics.Metrics {
	if j.Metrics != nil {
		return j.Metrics
	}
	return defaultJobMetrics
}

func (j *AnomalyScanJob) now() time.Time {
	if j.clock != nil {
		return j.clock()
	}
	return time.Now().UTC()
}

type timeSeries struct {
	CompanyID int64
	BranchID  int64
	Periods   []string
	Values    []float64
}

type scanAnomaly struct {
	CompanyID int64
	BranchID  int64
	Period    string
	Severity  string
	ZScore    float64
	Delta     float64
}

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func std(values []float64, mean float64) float64 {
	if len(values) < 2 {
		return 0
	}
	var variance float64
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(values) - 1)
	return math.Sqrt(variance)
}
