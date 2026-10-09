package cmms

import (
	"context"
	"errors"
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
		{"invalid status", func() error { _, err := svc.UpdateWorkOrderStatus(ctx, 1, 1, Status("UNKNOWN"), 1); return err }},
		{"IoT reading", func() error { _, err := svc.RecordIoTReading(ctx, IoTReading{}); return err }},
		{"predictive model", func() error { _, err := svc.CreatePredictiveModel(ctx, PredictiveModel{}); return err }},
		{"predictive alerts", func() error { _, err := svc.EvaluatePredictiveAlertsBatch(ctx, 0); return err }},
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

func TestWorkOrderTransitions(t *testing.T) {
	all := []Status{
		WorkOrderStatusDraft, WorkOrderStatusRequested, WorkOrderStatusPlanned, WorkOrderStatusScheduled,
		WorkOrderStatusInProgress, WorkOrderStatusOnHold, WorkOrderStatusCompleted,
		WorkOrderStatusCancelled, WorkOrderStatusClosed,
	}
	tests := []struct {
		from, to Status
		ok       bool
	}{
		// complete: only from IN_PROGRESS or ON_HOLD.
		{WorkOrderStatusDraft, WorkOrderStatusCompleted, false},
		{WorkOrderStatusRequested, WorkOrderStatusCompleted, false},
		{WorkOrderStatusPlanned, WorkOrderStatusCompleted, false},
		{WorkOrderStatusScheduled, WorkOrderStatusCompleted, false},
		{WorkOrderStatusInProgress, WorkOrderStatusCompleted, true},
		{WorkOrderStatusOnHold, WorkOrderStatusCompleted, true},
		// close: only from COMPLETED.
		{WorkOrderStatusDraft, WorkOrderStatusClosed, false},
		{WorkOrderStatusInProgress, WorkOrderStatusClosed, false},
		{WorkOrderStatusOnHold, WorkOrderStatusClosed, false},
		{WorkOrderStatusCompleted, WorkOrderStatusClosed, true},
		// a generated PM work order (REQUESTED) can be started like DRAFT.
		{WorkOrderStatusRequested, WorkOrderStatusInProgress, true},
		{WorkOrderStatusDraft, WorkOrderStatusInProgress, true},
		{WorkOrderStatusDraft, WorkOrderStatusPlanned, true},
		{WorkOrderStatusDraft, WorkOrderStatusOnHold, false},
		{WorkOrderStatusPlanned, WorkOrderStatusScheduled, true},
		{WorkOrderStatusPlanned, WorkOrderStatusOnHold, true},
		{WorkOrderStatusScheduled, WorkOrderStatusPlanned, false},
		{WorkOrderStatusInProgress, WorkOrderStatusOnHold, true},
		{WorkOrderStatusInProgress, WorkOrderStatusDraft, false},
		{WorkOrderStatusOnHold, WorkOrderStatusInProgress, true},
		{WorkOrderStatusCompleted, WorkOrderStatusInProgress, false},
		{WorkOrderStatusCompleted, WorkOrderStatusCancelled, false},
		// idempotent no-op.
		{WorkOrderStatusInProgress, WorkOrderStatusInProgress, true},
		{WorkOrderStatusCompleted, WorkOrderStatusCompleted, true},
		{WorkOrderStatusClosed, WorkOrderStatusClosed, true},
		// unknown statuses.
		{Status("UNKNOWN"), WorkOrderStatusInProgress, false},
		{WorkOrderStatusDraft, Status("UNKNOWN"), false},
	}
	// CLOSED and CANCELLED are terminal.
	for _, terminal := range []Status{WorkOrderStatusClosed, WorkOrderStatusCancelled} {
		for _, to := range all {
			if to != terminal {
				tests = append(tests, struct {
					from, to Status
					ok       bool
				}{terminal, to, false})
			}
		}
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			err := validateWorkOrderStatusTransition(tt.from, tt.to)
			if tt.ok && err != nil {
				t.Fatalf("want allowed, got %v", err)
			}
			if !tt.ok && !errors.Is(err, ErrInvalidStatus) {
				t.Fatalf("want ErrInvalidStatus, got %v", err)
			}
		})
	}

	// The generic status action refuses the targets owned by complete and
	// close before touching the repository.
	svc := NewService(nil)
	for _, to := range []Status{WorkOrderStatusCompleted, WorkOrderStatusClosed} {
		t.Run("status action refuses "+string(to), func(t *testing.T) {
			if _, err := svc.UpdateWorkOrderStatus(context.Background(), 1, 1, to, 1); !errors.Is(err, ErrInvalidStatus) {
				t.Fatalf("want ErrInvalidStatus, got %v", err)
			}
		})
	}
}
