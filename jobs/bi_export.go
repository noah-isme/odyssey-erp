package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/odyssey-erp/odyssey-erp/internal/analytics"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

const TaskBIExport = "analytics:bi_export"

type BIExportPayload struct {
	CompanyID int64  `json:"company_id"`
	Period    string `json:"period"`
	Provider  string `json:"provider"`
}

func NewBIExportTask(companyID int64, period, provider string) (*asynq.Task, error) {
	payload, err := json.Marshal(BIExportPayload{
		CompanyID: companyID,
		Period:    period,
		Provider:  provider,
	})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskBIExport, payload), nil
}

type BIExportJob struct {
	analyticsService *analytics.Service
	connectorService *connectors.Service
}

func NewBIExportJob(analyticsSvc *analytics.Service, connectorSvc *connectors.Service) *BIExportJob {
	return &BIExportJob{
		analyticsService: analyticsSvc,
		connectorService: connectorSvc,
	}
}

// Handle processes the background job
func (j *BIExportJob) Handle(ctx context.Context, task *asynq.Task) error {
	var payload BIExportPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("invalid bi_export payload: %w", asynq.SkipRetry)
	}

	content, err := j.analyticsService.GenerateBIExport(ctx, payload.CompanyID, payload.Period, nil)
	if err != nil {
		return fmt.Errorf("bi export generation failed: %w", err)
	}

	objectKey := fmt.Sprintf("exports/%d/kpi_%s_%s.csv", payload.CompanyID, payload.Period, time.Now().Format("20060102150405"))
	correlationID := uuid.New().String()

	if err := j.connectorService.EnqueueBIExport(ctx, payload.CompanyID, payload.Provider, objectKey, content, "text/csv", correlationID); err != nil {
		return fmt.Errorf("failed to enqueue bi export: %w", err)
	}

	return nil
}
