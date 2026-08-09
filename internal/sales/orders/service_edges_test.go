package orders

import (
	"context"
	"testing"
)

func TestValidateFulfillmentLineCoversRequiredBounds(t *testing.T) {
	validWarehouse := int64(3)
	cases := []CreateSalesOrderLineReq{
		{ProductID: 1, FulfillmentWarehouseID: validWarehouse, Quantity: 1, UOM: "PCS", UnitPrice: 1},
		{ProductID: 0, FulfillmentWarehouseID: validWarehouse, Quantity: 1, UOM: "PCS", UnitPrice: 1},
		{ProductID: 1, FulfillmentWarehouseID: 0, Quantity: 1, UOM: "PCS", UnitPrice: 1},
		{ProductID: 1, FulfillmentWarehouseID: validWarehouse, Quantity: 0, UOM: "PCS", UnitPrice: 1},
		{ProductID: 1, FulfillmentWarehouseID: validWarehouse, Quantity: 1, UOM: "", UnitPrice: 1},
		{ProductID: 1, FulfillmentWarehouseID: validWarehouse, Quantity: 1, UOM: "PCS", UnitPrice: -1},
		{ProductID: 1, FulfillmentWarehouseID: validWarehouse, Quantity: 1, UOM: "PCS", UnitPrice: 1, DiscountPercent: 101},
		{ProductID: 1, FulfillmentWarehouseID: validWarehouse, Quantity: 1, UOM: "PCS", UnitPrice: 1, TaxPercent: -1},
	}
	if err := validateFulfillmentLine(cases[0]); err != nil {
		t.Fatalf("valid line rejected: %v", err)
	}
	for _, line := range cases[1:] {
		if err := validateFulfillmentLine(line); err == nil {
			t.Fatalf("invalid line accepted: %#v", line)
		}
	}
}

func TestCancelRejectsFinalOrderStatuses(t *testing.T) {
	for _, status := range []SalesOrderStatus{SalesOrderStatusCancelled, SalesOrderStatusCompleted} {
		repo := newMemoryRepo()
		repo.orders[1] = &SalesOrder{ID: 1, Status: status}
		_, err := NewService(repo, &mockCustomerRepo{}, &mockQuoteRepo{}).Cancel(context.Background(), 1, 2, "done")
		if err == nil {
			t.Fatalf("Cancel() accepted status %s", status)
		}
	}
}

func TestCreateSalesOrderRejectsInvalidLineAfterHeaderChecks(t *testing.T) {
	_, err := NewService(newMemoryRepo(), &mockCustomerRepo{}, &mockQuoteRepo{}).Create(context.Background(), CreateSalesOrderRequest{
		CompanyID: 1, CustomerID: 1,
		Lines: []CreateSalesOrderLineReq{{ProductID: 1, FulfillmentWarehouseID: 2, Quantity: 1, UOM: "PCS", UnitPrice: 1, DiscountPercent: 101}},
	}, 1)
	if err == nil {
		t.Fatal("Create() accepted an empty order date")
	}
}
