package jobs

import (
	"context"
	"fmt"
	"log/slog"
	
	"github.com/hibiken/asynq"
)

const TypeCMMSPMGeneratorScan = "cmms:pm_generator_scan"

// CMMSPMGeneratorService defines the interface required by the PM generator job.
type CMMSPMGeneratorService interface {
	// GenerateAllPMWorkOrders generates PM work orders and returns the number generated
	// Actually it returns a slice of WorkOrders, but we don't need them in the job, just executing it is enough.
	// But we need the exact signature to mock or accept the real cmms.Service.
	// We'll define a simpler interface if we don't want to import cmms models, 
	// or we can just import cmms if we need to.
}

// Since we want to avoid circular dependencies (jobs importing cmms, cmms importing jobs),
// we will just define a closure that captures the service dependency inside cmd/worker/main.go,
// or we can define a simple interface here.
// Actually, jobs doesn't import cmms models right now? Let's check. Wait, jobs imports `fxdomain` (which is `internal/fx`).
// So we can just define a simple function wrapper instead of an interface.

// HandleCMMSPMGeneratorScanTask runs the PM generator across all active companies.
func HandleCMMSPMGeneratorScanTask(logger *slog.Logger, generatorFunc func(context.Context) error) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, _ *asynq.Task) error {
		logger.Info("starting cmms pm schedule scan")

		if err := generatorFunc(ctx); err != nil {
			logger.Error("cmms pm schedule scan failed", slog.Any("error", err))
			return fmt.Errorf("generatorFunc failed: %w", err)
		}

		logger.Info("cmms pm schedule scan completed")
		return nil
	}
}
