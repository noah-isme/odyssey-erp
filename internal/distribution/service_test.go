package distribution

import (
	"context"
	"strings"
	"testing"
)

func TestServiceRejectsInvalidPlanningInputsWithoutRepository(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
		want string
	}{
		{"planning horizon warehouse", func() error { _, err := svc.SetupPlanningHorizon(ctx, CreatePlanningHorizonInput{}); return err }, "warehouse ID is required"},
		{"planning rule name", func() error { _, err := svc.AddPlanningRule(ctx, CreatePlanningRuleInput{}); return err }, "rule name is required"},
		{"planning rule type", func() error {
			_, err := svc.AddPlanningRule(ctx, CreatePlanningRuleInput{RuleName: "capacity", RuleType: RuleType("INVALID")})
			return err
		}, "invalid rule type"},
		{"load origin", func() error { _, err := svc.CreateLoad(ctx, CreateLoadInput{}); return err }, "origin warehouse is required"},
		{"load destination", func() error { _, err := svc.CreateLoad(ctx, CreateLoadInput{OriginWarehouseID: 1}); return err }, "destination warehouse or city is required"},
		{"route load", func() error { _, err := svc.PlanDeliveryRoute(ctx, CreateRouteInput{}); return err }, "load ID is required"},
		{"transfer warehouses", func() error {
			_, err := svc.CreateTransferOrder(ctx, CreateTransferOrderInput{FromWarehouseID: 1, ToWarehouseID: 1})
			return err
		}, "from and to warehouses cannot be the same"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v want %q", err, tt.want)
			}
		})
	}
}
