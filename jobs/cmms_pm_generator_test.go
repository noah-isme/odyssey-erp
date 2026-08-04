package jobs_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/odyssey-erp/odyssey-erp/jobs"
)

func TestHandleCMMSPMGeneratorScanTask(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	t.Run("success", func(t *testing.T) {
		called := false
		handler := jobs.HandleCMMSPMGeneratorScanTask(logger, func(ctx context.Context) error {
			called = true
			return nil
		})
		
		err := handler(context.Background(), &asynq.Task{})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !called {
			t.Fatal("expected generator function to be called")
		}
	})

	t.Run("failure propagates error", func(t *testing.T) {
		expectedErr := errors.New("simulated error")
		handler := jobs.HandleCMMSPMGeneratorScanTask(logger, func(ctx context.Context) error {
			return expectedErr
		})
		
		err := handler(context.Background(), &asynq.Task{})
		if err == nil {
			t.Fatal("expected error but got none")
		}
		if !errors.Is(err, expectedErr) && err.Error() != "generatorFunc failed: simulated error" {
			t.Fatalf("expected simulated error to be wrapped, got %v", err)
		}
	})
}
