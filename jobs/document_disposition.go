package jobs

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/odyssey-erp/odyssey-erp/internal/documents"
)

const TaskDocumentDisposition = "documents:disposition"

// HandleDocumentDisposition creates a background job handler for document dispositions.
// This worker executes approved document disposition requests (deleting files and archiving records).
func HandleDocumentDisposition(service *documents.Service) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, _ *asynq.Task) error {
		if service == nil {
			return fmt.Errorf("document disposition service not configured: %w", asynq.SkipRetry)
		}
		return service.ProcessRetentionAndDispositions(ctx)
	}
}
