package distribution

import (
	"context"
	"strings"
	"testing"
	"time"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
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

func TestServiceCompletesDistributionLoadLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := newFakeDistributionRepository()
	shipments := newFakeShipmentGateway()
	inventory := &fakeInventoryGateway{}
	svc := NewServiceWithDependencies(repo, Dependencies{Shipments: shipments, Inventory: inventory})

	repo.loads[1] = &Load{
		ID:                 1,
		CompanyID:          7,
		LoadNumber:         "LOAD-1",
		Status:             LoadStatusDraft,
		OriginWarehouseID:  10,
		DestinationCity:    "Jakarta",
		DestinationCountry: "ID",
		CreatedBy:          42,
	}
	repo.nextID = 2

	shipmentID, err := svc.CreateShipmentForLoad(ctx, 1, ShipmentCreateInput{}, []ShipmentLineInput{{ProductID: 99, Quantity: 3}})
	if err != nil {
		t.Fatalf("create shipment: %v", err)
	}
	if err := assertNoError(svc.MarkLoadReady(ctx, 1)); err != nil {
		t.Fatal(err)
	}
	vehicleID, driverID := int64(21), int64(22)
	if err := svc.DispatchLoad(ctx, 1, &vehicleID, &driverID, nil, nil); err != nil {
		t.Fatalf("dispatch load: %v", err)
	}
	if err := svc.DeliverLoad(ctx, 1, 42, time.Date(2026, time.August, 9, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("deliver load: %v", err)
	}

	if got := repo.loads[1].Status; got != LoadStatusDelivered {
		t.Fatalf("load status=%s want %s", got, LoadStatusDelivered)
	}
	if got := shipments.statuses[shipmentID]; got != "DELIVERED" {
		t.Fatalf("shipment status=%s want DELIVERED", got)
	}
	if len(inventory.adjustments) != 1 {
		t.Fatalf("inventory adjustments=%d want 1", len(inventory.adjustments))
	}
	adjustment := inventory.adjustments[0]
	if adjustment.Quantity != -3 || adjustment.WarehouseID != 10 || adjustment.ProductID != 99 {
		t.Fatalf("unexpected inventory adjustment: %+v", adjustment)
	}
	if adjustment.RefModule != "DISTRIBUTION" {
		t.Fatalf("ref module=%q", adjustment.RefModule)
	}
}

func assertNoError(_ []string, err error) error {
	return err
}

func TestValidateLoadAgainstRulesReportsCapacityViolation(t *testing.T) {
	repo := newFakeDistributionRepository()
	repo.loads[1] = &Load{ID: 1, OriginWarehouseID: 10, Status: LoadStatusDraft}
	repo.items[1] = []*LoadItem{{ID: 2, LoadID: 1, ProductID: 5, Quantity: accountingmoney.Must("3", 4)}}
	maxItems := 2
	repo.rules[3] = &PlanningRule{ID: 3, WarehouseID: 10, RuleName: "small truck", RuleType: RuleTypeCapacity, MaxItemsPerLoad: &maxItems, IsActive: true}
	svc := NewServiceWithDependencies(repo, Dependencies{})

	valid, violations, err := svc.ValidateLoadAgainstRules(context.Background(), 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if valid || len(violations) != 1 || !strings.Contains(violations[0], "exceeds maximum") {
		t.Fatalf("valid=%v violations=%v", valid, violations)
	}
}
