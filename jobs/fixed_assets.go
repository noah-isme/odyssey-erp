package jobs

import (
	"context"
	"github.com/hibiken/asynq"
	"github.com/odyssey-erp/odyssey-erp/internal/fixedassets"
	"time"
)

const TaskFixedAssetDepreciation = "fixed_assets:depreciate"

func HandleFixedAssetDepreciation(service *fixedassets.Service) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, _ *asynq.Task) error {
		_, err := service.RunMonthlyDepreciation(ctx, time.Now().UTC())
		return err
	}
}
