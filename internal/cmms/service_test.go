package cmms

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
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

func TestNextDueDate(t *testing.T) {
	day := func(s string) time.Time {
		t.Helper()
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	ptrDate := func(s string) *time.Time { d := day(s); return &d }
	ptrMeter := func(v float64) *float64 { return &v }

	tests := []struct {
		name      string
		sched     PreventiveMaintenanceSchedule
		asOf      string
		wantDate  string // "" = zero date
		wantMeter float64
		wantErr   bool
	}{
		{"daily", PreventiveMaintenanceSchedule{FrequencyType: "DAILY", FrequencyValue: 1, NextDueDate: ptrDate("2026-10-09")}, "2026-10-09", "2026-10-10", 0, false},
		{"every 3 days", PreventiveMaintenanceSchedule{FrequencyType: "DAILY", FrequencyValue: 3, NextDueDate: ptrDate("2026-10-09")}, "2026-10-09", "2026-10-12", 0, false},
		{"weekly", PreventiveMaintenanceSchedule{FrequencyType: "WEEKLY", FrequencyValue: 2, NextDueDate: ptrDate("2026-10-09")}, "2026-10-09", "2026-10-23", 0, false},
		{"monthly", PreventiveMaintenanceSchedule{FrequencyType: "MONTHLY", FrequencyValue: 1, NextDueDate: ptrDate("2026-10-09")}, "2026-10-09", "2026-11-09", 0, false},
		{"monthly clamps to month end", PreventiveMaintenanceSchedule{FrequencyType: "MONTHLY", FrequencyValue: 1, NextDueDate: ptrDate("2027-01-31")}, "2027-01-31", "2027-02-28", 0, false},
		{"quarterly", PreventiveMaintenanceSchedule{FrequencyType: "QUARTERLY", FrequencyValue: 1, NextDueDate: ptrDate("2026-10-09")}, "2026-10-09", "2027-01-09", 0, false},
		{"semi-annual", PreventiveMaintenanceSchedule{FrequencyType: "SEMI_ANNUAL", FrequencyValue: 1, NextDueDate: ptrDate("2026-10-09")}, "2026-10-09", "2027-04-09", 0, false},
		{"annual", PreventiveMaintenanceSchedule{FrequencyType: "ANNUAL", FrequencyValue: 1, NextDueDate: ptrDate("2026-10-09")}, "2026-10-09", "2027-10-09", 0, false},
		{"annual from leap day", PreventiveMaintenanceSchedule{FrequencyType: "ANNUAL", FrequencyValue: 1, NextDueDate: ptrDate("2028-02-29")}, "2028-02-29", "2029-02-28", 0, false},
		{"overdue skips to first occurrence after as-of", PreventiveMaintenanceSchedule{FrequencyType: "DAILY", FrequencyValue: 1, NextDueDate: ptrDate("2026-10-01")}, "2026-10-09", "2026-10-10", 0, false},
		{"overdue monthly", PreventiveMaintenanceSchedule{FrequencyType: "MONTHLY", FrequencyValue: 1, NextDueDate: ptrDate("2026-07-15")}, "2026-10-20", "2026-11-15", 0, false},
		{"meter based advances meter only", PreventiveMaintenanceSchedule{FrequencyType: "METER_BASED", FrequencyValue: 250, NextDueMeter: ptrMeter(1000)}, "2026-10-09", "", 1250, false},
		{"meter based without meter", PreventiveMaintenanceSchedule{FrequencyType: "METER_BASED", FrequencyValue: 250}, "2026-10-09", "", 0, true},
		{"calendar without date", PreventiveMaintenanceSchedule{FrequencyType: "DAILY", FrequencyValue: 1}, "2026-10-09", "", 0, true},
		{"zero value", PreventiveMaintenanceSchedule{FrequencyType: "DAILY", FrequencyValue: 0, NextDueDate: ptrDate("2026-10-09")}, "2026-10-09", "", 0, true},
		{"unknown type", PreventiveMaintenanceSchedule{FrequencyType: "YEARLY", FrequencyValue: 1, NextDueDate: ptrDate("2026-10-09")}, "2026-10-09", "", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDate, gotMeter, err := nextDueDate(tt.sched, day(tt.asOf))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got %s %v", gotDate.Format("2006-01-02"), gotMeter)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantDate == "" {
				if !gotDate.IsZero() {
					t.Fatalf("date=%s want zero", gotDate.Format("2006-01-02"))
				}
			} else if !gotDate.Equal(day(tt.wantDate)) {
				t.Fatalf("date=%s want %s", gotDate.Format("2006-01-02"), tt.wantDate)
			}
			if gotMeter != tt.wantMeter {
				t.Fatalf("meter=%v want %v", gotMeter, tt.wantMeter)
			}
		})
	}
}
