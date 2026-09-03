package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/odyssey-erp/odyssey-erp/internal/documents"
)

const TaskDocumentOCR = "documents:ocr"

type DocumentOCRPayload struct {
	JobID int64 `json:"job_id"`
}

func NewDocumentOCRTask(jobID int64) (*asynq.Task, error) {
	if jobID <= 0 {
		return nil, errors.New("document OCR job id required")
	}
	payload, err := json.Marshal(DocumentOCRPayload{JobID: jobID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskDocumentOCR, payload, asynq.Queue(QueueDefault), asynq.MaxRetry(5)), nil
}

func (c *Client) EnqueueDocumentOCR(ctx context.Context, jobID int64) (*asynq.TaskInfo, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("document OCR job client not configured")
	}
	task, err := NewDocumentOCRTask(jobID)
	if err != nil {
		return nil, err
	}
	info, err := c.client.EnqueueContext(ctx, task, asynq.Queue(QueueDefault), asynq.MaxRetry(5), asynq.TaskID("document-ocr:"+fmt.Sprint(jobID)))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil, nil
	}
	return info, err
}

func HandleDocumentOCR(service *documents.Service) asynq.HandlerFunc {
	return func(ctx context.Context, task *asynq.Task) error {
		if service == nil {
			return fmt.Errorf("document OCR service not configured: %w", asynq.SkipRetry)
		}
		var payload DocumentOCRPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil || payload.JobID <= 0 {
			return fmt.Errorf("invalid document OCR task: %w", asynq.SkipRetry)
		}
		return service.ProcessOCRJob(ctx, payload.JobID)
	}
}
