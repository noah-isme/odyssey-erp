package qms

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
		{"NCR", func() error { _, err := svc.CreateNCR(ctx, CreateNCRRequest{}); return err }},
		{"NCR update", func() error { _, err := svc.UpdateNCR(ctx, 1, UpdateNCRRequest{}); return err }},
		{"NCR status", func() error { _, err := svc.UpdateNCRStatus(ctx, 1, Status("UNKNOWN"), 1); return err }},
		{"disposition", func() error { _, err := svc.RecordDisposition(ctx, RecordDispositionRequest{}); return err }},
		{"CAPA", func() error { _, err := svc.CreateCAPA(ctx, CreateCAPARequest{}); return err }},
		{"CAPA update", func() error { _, err := svc.UpdateCAPA(ctx, 1, UpdateCAPARequest{}); return err }},
		{"CAPA status", func() error { _, err := svc.UpdateCAPAStatus(ctx, 1, Status("UNKNOWN"), 1); return err }},
		{"audit", func() error { _, err := svc.CreateAudit(ctx, CreateAuditRequest{}); return err }},
		{"finding", func() error { _, err := svc.AddFinding(ctx, CreateFindingRequest{}); return err }},
		{"supplier quality", func() error { _, err := svc.CreateSupplierQuality(ctx, CreateSupplierQualityRequest{}); return err }},
		{"supplier audit", func() error { _, err := svc.CreateSupplierAudit(ctx, CreateSupplierAuditRequest{}); return err }},
		{"quality objective", func() error { _, err := svc.CreateQualityObjective(ctx, CreateQualityObjectiveRequest{}); return err }},
		{"measurement", func() error { _, err := svc.RecordMeasurement(ctx, RecordMeasurementRequest{}); return err }},
		{"inspection", func() error { _, err := svc.CreateInspection(ctx, CreateInspectionRequest{}); return err }},
		{"inspection plan", func() error { _, err := svc.CreateInspectionPlan(ctx, CreateInspectionPlanRequest{}); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.HasPrefix(err.Error(), "qms:") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestNormaliseStatus(t *testing.T) {
	if got := NormaliseStatus(" passed "); got != Status("PASSED") {
		t.Fatalf("status=%q want PASSED", got)
	}
	if got := NormaliseStatus("unknown"); got != NCRStatusOpen {
		t.Fatalf("invalid status=%q want %q", got, NCRStatusOpen)
	}
}
