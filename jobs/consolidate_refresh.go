package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/hibiken/asynq"

	consolhttp "github.com/odyssey-erp/odyssey-erp/internal/consol/http"
	jobmetrics "github.com/odyssey-erp/odyssey-erp/internal/jobs"
)

const (
	// TaskConsolidateRefresh schedules the consolidation refresh routine.
	TaskConsolidateRefresh = "consol:refresh"
)

// ConsolidateRefreshPayload configures the scope of the consolidation refresh job.
type ConsolidateRefreshPayload struct {
	GroupID string `json:"group_id"`
	Period  string `json:"period"`
}

// ConsolidationService describes the behaviour required to rebuild materialised balances.
type ConsolidationService interface {
	RebuildConsolidation(ctx context.Context, groupID int64, period string) error
}

// ConsolidationRepository provides helper lookups for the job runtime.
type ConsolidationRepository interface {
	ListGroupIDs(ctx context.Context) ([]int64, error)
	ActiveConsolidationPeriod(ctx context.Context) (string, error)
}

// ConsolidateRefreshJob coordinates the refresh workflow.
type ConsolidateRefreshJob struct {
	Service ConsolidationService
	Repo    ConsolidationRepository
	Logger  *slog.Logger
	Metrics *jobmetrics.Metrics
	clock   func() time.Time
}

// NewConsolidateRefreshJob constructs the job handler.
func NewConsolidateRefreshJob(service ConsolidationService, repo ConsolidationRepository, logger *slog.Logger, metrics *jobmetrics.Metrics) *ConsolidateRefreshJob {
	return &ConsolidateRefreshJob{
		Service: service,
		Repo:    repo,
		Logger:  logger,
		Metrics: metrics,
		clock: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// NewConsolidateRefreshTask creates an Asynq task for refreshing consolidated balances.
func NewConsolidateRefreshTask(groupID, period string) (*asynq.Task, error) {
	if groupID == "" {
		groupID = "all"
	}
	if period == "" {
		period = "active"
	}
	payload := ConsolidateRefreshPayload{GroupID: groupID, Period: period}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskConsolidateRefresh, body, asynq.Queue(QueueDefault)), nil
}

// Handle executes the consolidate refresh job.
func (j *ConsolidateRefreshJob) Handle(ctx context.Context, task *asynq.Task) error {
	if j == nil || j.Service == nil || j.Repo == nil {
		return errors.New("consolidate refresh: dependencies not configured")
	}
	var payload ConsolidateRefreshPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return asynq.SkipRetry
	}
	if payload.GroupID == "" {
		payload.GroupID = "all"
	}
	if payload.Period == "" {
		payload.Period = "active"
	}

	tracker := j.metrics().Track(TaskConsolidateRefresh)
	var resultErr error
	defer func() {
		resultErr = tracker.End(resultErr)
	}()

	period, err := j.resolvePeriod(ctx, payload.Period)
	if err != nil {
		resultErr = err
		j.log().Error("resolve period", slog.String("period", payload.Period), slog.Any("error", err))
		return resultErr
	}

	groupIDs, err := j.resolveGroups(ctx, payload.GroupID)
	if err != nil {
		resultErr = err
		j.log().Error("resolve groups", slog.String("group", payload.GroupID), slog.Any("error", err))
		return resultErr
	}
	if len(groupIDs) == 0 {
		j.log().Info("no consolidation groups discovered", slog.String("period", period))
		return resultErr
	}

	start := j.now()
	refreshed := 0
	for _, groupID := range groupIDs {
		if err := j.Service.RebuildConsolidation(ctx, groupID, period); err != nil {
			resultErr = err
			j.log().Error("rebuild consolidation", slog.Int64("group_id", groupID), slog.String("period", period), slog.Any("error", err))
			return resultErr
		}
		refreshed++
	}

	consolhttp.BustConsolViewCache()

	j.log().Info("refreshed consolidation balances", slog.String("period", period), slog.Int("groups", refreshed), slog.Duration("duration", time.Since(start)))
	return resultErr
}

func (j *ConsolidateRefreshJob) resolvePeriod(ctx context.Context, period string) (string, error) {
	if period != "" && period != "active" {
		return period, nil
	}
	if j.Repo == nil {
		return "", fmt.Errorf("consolidate refresh: repository not configured")
	}
	code, err := j.Repo.ActiveConsolidationPeriod(ctx)
	if err != nil {
		return "", err
	}
	if code == "" {
		return "", fmt.Errorf("no active consolidation period")
	}
	return code, nil
}

func (j *ConsolidateRefreshJob) resolveGroups(ctx context.Context, group string) ([]int64, error) {
	if group == "" || group == "all" {
		return j.Repo.ListGroupIDs(ctx)
	}
	id, err := strconv.ParseInt(group, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid group id %s", group)
	}
	if id <= 0 {
		return nil, fmt.Errorf("group id must be positive")
	}
	return []int64{id}, nil
}

func (j *ConsolidateRefreshJob) metrics() *jobmetrics.Metrics {
	if j != nil && j.Metrics != nil {
		return j.Metrics
	}
	return defaultJobMetrics
}

func (j *ConsolidateRefreshJob) log() *slog.Logger {
	if j != nil && j.Logger != nil {
		return j.Logger.With(slog.String("job", TaskConsolidateRefresh))
	}
	return slog.Default().With(slog.String("job", TaskConsolidateRefresh))
}

func (j *ConsolidateRefreshJob) now() time.Time {
	if j != nil && j.clock != nil {
		return j.clock()
	}
	return time.Now().UTC()
}

// WithClock overrides the internal clock for deterministic tests.
func (j *ConsolidateRefreshJob) WithClock(clock func() time.Time) {
	if j != nil && clock != nil {
		j.clock = clock
	}
}
