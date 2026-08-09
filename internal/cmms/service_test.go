package cmms

import (
	"context"
	"strings"
	"testing"
)

func TestServiceRejectsInvalidRequestsWithoutRepository(t *testing.T) {
	svc := NewService(nil)
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"work order", func() error { _, err := svc.CreateWorkOrder(ctx, CreateWorkOrderRequest{}); return err }},
		{"task", func() error { _, err := svc.AddTask(ctx, CreateTaskRequest{}); return err }},
		{"asset", func() error { _, err := svc.CreateAsset(ctx, CreateAssetRequest{}); return err }},
		{"location", func() error { _, err := svc.CreateLocation(ctx, CreateLocationRequest{}); return err }},
		{"PM schedule", func() error { _, err := svc.CreatePMSchedule(ctx, CreatePMScheduleRequest{}); return err }},
		{"meter reading", func() error { _, err := svc.RecordMeterReading(ctx, CreateMeterReadingRequest{}); return err }},
		{"spare part", func() error { _, err := svc.CreateSparePart(ctx, CreateSparePartRequest{}); return err }},
		{"work order spare part", func() error { _, err := svc.AddSparePartToWorkOrder(ctx, AddSparePartRequest{}); return err }},
		{"invalid status", func() error { _, err := svc.UpdateWorkOrderStatus(ctx, 1, Status("UNKNOWN"), 1); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.HasPrefix(err.Error(), "cmms:") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestNormaliseWorkOrderValues(t *testing.T) {
	if got := NormaliseStatus(" in_progress "); got != WorkOrderStatusInProgress {
		t.Fatalf("status=%q want %q", got, WorkOrderStatusInProgress)
	}
	if got := NormaliseStatus("unknown"); got != WorkOrderStatusDraft {
		t.Fatalf("invalid status=%q want %q", got, WorkOrderStatusDraft)
	}
	if got := NormalisePriority(" critical "); got != PriorityCritical {
		t.Fatalf("priority=%q want %q", got, PriorityCritical)
	}
	if got := NormalisePriority("unknown"); got != PriorityMedium {
		t.Fatalf("invalid priority=%q want %q", got, PriorityMedium)
	}
}
