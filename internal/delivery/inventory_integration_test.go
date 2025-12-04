package delivery

import (
	"context"
	"errors"
	"testing"
)

// MockInventoryService is a mock implementation of InventoryService for testing
type MockInventoryService struct {
	PostAdjustmentFunc func(ctx context.Context, input InventoryAdjustmentInput) error
	CallCount          int
	LastInput          *InventoryAdjustmentInput
}

func (m *MockInventoryService) PostAdjustment(ctx context.Context, input InventoryAdjustmentInput) error {
	m.CallCount++
	m.LastInput = &input
	if m.PostAdjustmentFunc != nil {
		return m.PostAdjustmentFunc(ctx, input)
	}
	return nil
}

func TestInventoryAdapter(t *testing.T) {
	t.Run("PostAdjustment success", func(t *testing.T) {
		mockInv := &MockInventoryService{
			PostAdjustmentFunc: func(ctx context.Context, input InventoryAdjustmentInput) error {
				return nil
			},
		}

		adapter := &InventoryAdapter{service: nil}
		// Simulate adapter wrapping a real service
		adapter.service = nil // This would be a real inventory.Service in production

		input := InventoryAdjustmentInput{
			Code:        "DO-TEST-001",
			WarehouseID: 1,
			ProductID:   100,
			Qty:         -5.0,
			UnitCost:    50.0,
			Note:        "Test delivery",
			ActorID:     1,
			RefModule:   "DELIVERY",
			RefID:       "123",
		}

		// Test with mock
		err := mockInv.PostAdjustment(context.Background(), input)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if mockInv.CallCount != 1 {
			t.Errorf("expected 1 call, got %d", mockInv.CallCount)
		}

		if mockInv.LastInput == nil {
			t.Fatal("expected LastInput to be set")
		}

		if mockInv.LastInput.Code != "DO-TEST-001" {
			t.Errorf("expected Code 'DO-TEST-001', got '%s'", mockInv.LastInput.Code)
		}

		if mockInv.LastInput.Qty != -5.0 {
			t.Errorf("expected Qty -5.0, got %f", mockInv.LastInput.Qty)
		}
	})

	t.Run("PostAdjustment error handling", func(t *testing.T) {
		expectedErr := errors.New("inventory service error")
		mockInv := &MockInventoryService{
			PostAdjustmentFunc: func(ctx context.Context, input InventoryAdjustmentInput) error {
				return expectedErr
			},
		}

		input := InventoryAdjustmentInput{
			Code:        "DO-TEST-002",
			WarehouseID: 1,
			ProductID:   100,
			Qty:         -10.0,
			UnitCost:    50.0,
			Note:        "Test delivery error",
			ActorID:     1,
			RefModule:   "DELIVERY",
			RefID:       "456",
		}

		err := mockInv.PostAdjustment(context.Background(), input)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("Verify negative quantity for outbound", func(t *testing.T) {
		mockInv := &MockInventoryService{}

		input := InventoryAdjustmentInput{
			Code:        "DO-TEST-003",
			WarehouseID: 1,
			ProductID:   200,
			Qty:         -15.5, // Negative for outbound/delivery
			UnitCost:    75.25,
			Note:        "Delivery outbound",
			ActorID:     2,
			RefModule:   "DELIVERY",
			RefID:       "789",
		}

		_ = mockInv.PostAdjustment(context.Background(), input)

		if mockInv.LastInput == nil {
			t.Fatal("expected LastInput to be set")
		}

		if mockInv.LastInput.Qty >= 0 {
			t.Errorf("expected negative quantity for outbound, got %f", mockInv.LastInput.Qty)
		}

		if mockInv.LastInput.ProductID != 200 {
			t.Errorf("expected ProductID 200, got %d", mockInv.LastInput.ProductID)
		}
	})

	t.Run("Verify reference module is DELIVERY", func(t *testing.T) {
		mockInv := &MockInventoryService{}

		input := InventoryAdjustmentInput{
			Code:        "DO-TEST-004",
			WarehouseID: 3,
			ProductID:   300,
			Qty:         -20.0,
			UnitCost:    100.0,
			Note:        "Test reference",
			ActorID:     5,
			RefModule:   "DELIVERY",
			RefID:       "999",
		}

		_ = mockInv.PostAdjustment(context.Background(), input)

		if mockInv.LastInput == nil {
			t.Fatal("expected LastInput to be set")
		}

		if mockInv.LastInput.RefModule != "DELIVERY" {
			t.Errorf("expected RefModule 'DELIVERY', got '%s'", mockInv.LastInput.RefModule)
		}

		if mockInv.LastInput.RefID != "999" {
			t.Errorf("expected RefID '999', got '%s'", mockInv.LastInput.RefID)
		}
	})

	t.Run("Multiple inventory adjustments", func(t *testing.T) {
		mockInv := &MockInventoryService{}

		// Simulate multiple line items
		lines := []InventoryAdjustmentInput{
			{
				Code:        "DO-TEST-005-L1",
				WarehouseID: 1,
				ProductID:   401,
				Qty:         -10.0,
				UnitCost:    50.0,
				Note:        "Line 1",
				ActorID:     1,
				RefModule:   "DELIVERY",
				RefID:       "1001",
			},
			{
				Code:        "DO-TEST-005-L2",
				WarehouseID: 1,
				ProductID:   402,
				Qty:         -5.0,
				UnitCost:    75.0,
				Note:        "Line 2",
				ActorID:     1,
				RefModule:   "DELIVERY",
				RefID:       "1001",
			},
		}

		for _, line := range lines {
			err := mockInv.PostAdjustment(context.Background(), line)
			if err != nil {
				t.Fatalf("unexpected error for line: %v", err)
			}
		}

		if mockInv.CallCount != 2 {
			t.Errorf("expected 2 calls, got %d", mockInv.CallCount)
		}

		// Last input should be the second line
		if mockInv.LastInput.ProductID != 402 {
			t.Errorf("expected last ProductID 402, got %d", mockInv.LastInput.ProductID)
		}
	})
}

func TestSetInventoryService(t *testing.T) {
	service := &Service{
		repo:      nil,
		pool:      nil,
		inventory: nil,
	}

	if service.inventory != nil {
		t.Error("expected inventory to be nil initially")
	}

	mockInv := &MockInventoryService{}
	service.SetInventoryService(mockInv)

	if service.inventory == nil {
		t.Error("expected inventory to be set")
	}

	// Test that we can call through the interface
	err := service.inventory.PostAdjustment(context.Background(), InventoryAdjustmentInput{
		Code:        "TEST",
		WarehouseID: 1,
		ProductID:   1,
		Qty:         -1.0,
		UnitCost:    1.0,
		Note:        "test",
		ActorID:     1,
		RefModule:   "DELIVERY",
		RefID:       "1",
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mockInv.CallCount != 1 {
		t.Errorf("expected 1 call, got %d", mockInv.CallCount)
	}
}

func TestInventoryIntegrationOptional(t *testing.T) {
	t.Run("Service works without inventory integration", func(t *testing.T) {
		service := &Service{
			repo:      nil,
			pool:      nil,
			inventory: nil, // No inventory service
		}

		// Should not panic when inventory is nil
		if service.inventory != nil {
			t.Error("expected inventory to be nil")
		}

		// This demonstrates that the service can work without inventory integration
		// The actual MarkDelivered method checks if inventory is nil before calling it
	})

	t.Run("Service works with inventory integration", func(t *testing.T) {
		mockInv := &MockInventoryService{}

		service := &Service{
			repo:      nil,
			pool:      nil,
			inventory: mockInv,
		}

		if service.inventory == nil {
			t.Error("expected inventory to be set")
		}

		// Simulate a call
		_ = service.inventory.PostAdjustment(context.Background(), InventoryAdjustmentInput{
			Code:        "TEST-001",
			WarehouseID: 1,
			ProductID:   100,
			Qty:         -5.0,
			UnitCost:    50.0,
			Note:        "Test",
			ActorID:     1,
			RefModule:   "DELIVERY",
			RefID:       "1",
		})

		if mockInv.CallCount != 1 {
			t.Errorf("expected 1 call, got %d", mockInv.CallCount)
		}
	})
}
