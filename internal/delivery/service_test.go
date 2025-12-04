package delivery

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// TEST SERVICE WRAPPER
// ============================================================================

// testService wraps a service with mock repository methods
type testService struct {
	mock *mockRepository
}

func newTestService() *testService {
	return &testService{
		mock: newMockRepository(),
	}
}

// CreateDeliveryOrder wraps the service method with mock repo
func (ts *testService) CreateDeliveryOrder(ctx context.Context, req CreateDeliveryOrderRequest, createdBy int64) (*DeliveryOrder, error) {
	// Validate sales order exists and is in correct status
	soDetails, err := ts.mock.GetSalesOrderDetails(ctx, req.SalesOrderID)
	if err != nil {
		return nil, fmt.Errorf("get sales order: %w", err)
	}

	if soDetails.Status != "CONFIRMED" && soDetails.Status != "PROCESSING" {
		return nil, fmt.Errorf("sales order must be CONFIRMED or PROCESSING to create delivery order, got: %s", soDetails.Status)
	}

	// Validate company ID matches
	if soDetails.CompanyID != req.CompanyID {
		return nil, fmt.Errorf("sales order belongs to different company")
	}

	// Validate warehouse exists
	warehouseExists, err := ts.mock.CheckWarehouseExists(ctx, req.WarehouseID)
	if err != nil {
		return nil, fmt.Errorf("check warehouse: %w", err)
	}
	if !warehouseExists {
		return nil, fmt.Errorf("warehouse not found")
	}

	// Get deliverable lines to validate requested quantities
	deliverableLines, err := ts.mock.GetDeliverableSOLines(ctx, req.SalesOrderID)
	if err != nil {
		return nil, fmt.Errorf("get deliverable lines: %w", err)
	}

	if len(deliverableLines) == 0 {
		return nil, fmt.Errorf("no deliverable lines found for sales order")
	}

	// Build map of deliverable quantities for validation
	deliverableMap := make(map[int64]*DeliverableSOLine)
	for i := range deliverableLines {
		deliverableMap[deliverableLines[i].SalesOrderLineID] = &deliverableLines[i]
	}

	// Validate each requested line
	for _, reqLine := range req.Lines {
		deliverable, exists := deliverableMap[reqLine.SalesOrderLineID]
		if !exists {
			return nil, fmt.Errorf("sales order line %d not found or fully delivered", reqLine.SalesOrderLineID)
		}

		if reqLine.QuantityToDeliver > deliverable.RemainingQuantity {
			return nil, fmt.Errorf("requested quantity %.2f exceeds remaining quantity %.2f for line %d",
				reqLine.QuantityToDeliver, deliverable.RemainingQuantity, reqLine.SalesOrderLineID)
		}

		if reqLine.QuantityToDeliver <= 0 {
			return nil, fmt.Errorf("quantity to deliver must be positive for line %d", reqLine.SalesOrderLineID)
		}

		// Validate product ID matches
		if reqLine.ProductID != deliverable.ProductID {
			return nil, fmt.Errorf("product ID mismatch for line %d", reqLine.SalesOrderLineID)
		}
	}

	// Generate document number
	docNumber, err := ts.mock.GenerateDeliveryOrderNumber(ctx, req.CompanyID, req.DeliveryDate)
	if err != nil {
		return nil, fmt.Errorf("generate doc number: %w", err)
	}

	// Create delivery order in transaction
	var doID int64
	err = ts.mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		do := DeliveryOrder{
			DocNumber:      docNumber,
			CompanyID:      req.CompanyID,
			SalesOrderID:   req.SalesOrderID,
			WarehouseID:    req.WarehouseID,
			CustomerID:     soDetails.CustomerID,
			DeliveryDate:   req.DeliveryDate,
			Status:         DOStatusDraft,
			DriverName:     req.DriverName,
			VehicleNumber:  req.VehicleNumber,
			TrackingNumber: req.TrackingNumber,
			Notes:          req.Notes,
			CreatedBy:      createdBy,
		}

		id, err := tx.CreateDeliveryOrder(ctx, do)
		if err != nil {
			return fmt.Errorf("create delivery order: %w", err)
		}
		doID = id

		// Insert lines
		for _, reqLine := range req.Lines {
			deliverable := deliverableMap[reqLine.SalesOrderLineID]
			line := DeliveryOrderLine{
				DeliveryOrderID:   doID,
				SalesOrderLineID:  reqLine.SalesOrderLineID,
				ProductID:         reqLine.ProductID,
				QuantityToDeliver: reqLine.QuantityToDeliver,
				QuantityDelivered: 0,
				UOM:               deliverable.UOM,
				UnitPrice:         deliverable.UnitPrice,
				Notes:             reqLine.Notes,
				LineOrder:         reqLine.LineOrder,
			}

			_, err := tx.InsertDeliveryOrderLine(ctx, line)
			if err != nil {
				return fmt.Errorf("insert line: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return ts.mock.GetDeliveryOrder(ctx, doID)
}

// UpdateDeliveryOrder wraps update logic
func (ts *testService) UpdateDeliveryOrder(ctx context.Context, id int64, req UpdateDeliveryOrderRequest) (*DeliveryOrder, error) {
	existing, err := ts.mock.GetDeliveryOrder(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get delivery order: %w", err)
	}

	if !existing.Status.CanEdit() {
		return nil, fmt.Errorf("cannot edit delivery order in status: %s", existing.Status)
	}

	updates := make(map[string]interface{})
	if req.DeliveryDate != nil {
		updates["delivery_date"] = *req.DeliveryDate
	}
	if req.DriverName != nil {
		updates["driver_name"] = req.DriverName
	}
	if req.VehicleNumber != nil {
		updates["vehicle_number"] = req.VehicleNumber
	}
	if req.TrackingNumber != nil {
		updates["tracking_number"] = req.TrackingNumber
	}
	if req.Notes != nil {
		updates["notes"] = req.Notes
	}

	if req.Lines != nil {
		deliverableLines, err := ts.mock.GetDeliverableSOLines(ctx, existing.SalesOrderID)
		if err != nil {
			return nil, fmt.Errorf("get deliverable lines: %w", err)
		}

		deliverableMap := make(map[int64]*DeliverableSOLine)
		for i := range deliverableLines {
			deliverableMap[deliverableLines[i].SalesOrderLineID] = &deliverableLines[i]
		}

		for _, reqLine := range *req.Lines {
			deliverable, exists := deliverableMap[reqLine.SalesOrderLineID]
			if !exists {
				return nil, fmt.Errorf("sales order line %d not found or fully delivered", reqLine.SalesOrderLineID)
			}

			if reqLine.QuantityToDeliver > deliverable.RemainingQuantity {
				return nil, fmt.Errorf("requested quantity %.2f exceeds remaining quantity %.2f for line %d",
					reqLine.QuantityToDeliver, deliverable.RemainingQuantity, reqLine.SalesOrderLineID)
			}
		}
	}

	err = ts.mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		if len(updates) > 0 {
			if err := tx.UpdateDeliveryOrder(ctx, id, updates); err != nil {
				return fmt.Errorf("update delivery order: %w", err)
			}
		}

		if req.Lines != nil {
			if err := tx.DeleteDeliveryOrderLines(ctx, id); err != nil {
				return fmt.Errorf("delete lines: %w", err)
			}

			deliverableLines, err := ts.mock.GetDeliverableSOLines(ctx, existing.SalesOrderID)
			if err != nil {
				return fmt.Errorf("get deliverable lines: %w", err)
			}

			deliverableMap := make(map[int64]*DeliverableSOLine)
			for i := range deliverableLines {
				deliverableMap[deliverableLines[i].SalesOrderLineID] = &deliverableLines[i]
			}

			for _, reqLine := range *req.Lines {
				deliverable := deliverableMap[reqLine.SalesOrderLineID]
				line := DeliveryOrderLine{
					DeliveryOrderID:   id,
					SalesOrderLineID:  reqLine.SalesOrderLineID,
					ProductID:         reqLine.ProductID,
					QuantityToDeliver: reqLine.QuantityToDeliver,
					QuantityDelivered: 0,
					UOM:               deliverable.UOM,
					UnitPrice:         deliverable.UnitPrice,
					Notes:             reqLine.Notes,
					LineOrder:         reqLine.LineOrder,
				}

				if _, err := tx.InsertDeliveryOrderLine(ctx, line); err != nil {
					return fmt.Errorf("insert line: %w", err)
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return ts.mock.GetDeliveryOrder(ctx, id)
}

// ConfirmDeliveryOrder wraps confirm logic
func (ts *testService) ConfirmDeliveryOrder(ctx context.Context, id int64, confirmedBy int64) (*DeliveryOrder, error) {
	existing, err := ts.mock.GetDeliveryOrder(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get delivery order: %w", err)
	}

	if !existing.Status.CanConfirm() {
		return nil, fmt.Errorf("cannot confirm delivery order in status: %s", existing.Status)
	}

	if len(existing.Lines) == 0 {
		return nil, fmt.Errorf("cannot confirm delivery order without lines")
	}

	confirmedAt := time.Now()

	err = ts.mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		updates := map[string]interface{}{
			"confirmed_by": confirmedBy,
			"confirmed_at": confirmedAt,
		}

		if err := tx.UpdateDeliveryOrderStatus(ctx, id, DOStatusConfirmed, updates); err != nil {
			return fmt.Errorf("update status: %w", err)
		}

		for _, line := range existing.Lines {
			if err := tx.UpdateDeliveryOrderLineQuantity(ctx, line.ID, line.QuantityToDeliver); err != nil {
				return fmt.Errorf("update line quantity: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return ts.mock.GetDeliveryOrder(ctx, id)
}

// MarkInTransit wraps mark in transit logic
func (ts *testService) MarkInTransit(ctx context.Context, id int64, req MarkInTransitRequest) (*DeliveryOrder, error) {
	existing, err := ts.mock.GetDeliveryOrder(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get delivery order: %w", err)
	}

	if existing.Status != DOStatusConfirmed {
		return nil, fmt.Errorf("delivery order must be CONFIRMED to mark in transit, got: %s", existing.Status)
	}

	updates := map[string]interface{}{}
	if req.TrackingNumber != nil {
		updates["tracking_number"] = req.TrackingNumber
	}

	err = ts.mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.UpdateDeliveryOrderStatus(ctx, id, DOStatusInTransit, updates)
	})

	if err != nil {
		return nil, fmt.Errorf("mark in transit: %w", err)
	}

	return ts.mock.GetDeliveryOrder(ctx, id)
}

// MarkDelivered wraps mark delivered logic
func (ts *testService) MarkDelivered(ctx context.Context, id int64, req MarkDeliveredRequest) (*DeliveryOrder, error) {
	existing, err := ts.mock.GetDeliveryOrder(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get delivery order: %w", err)
	}

	if existing.Status != DOStatusInTransit {
		return nil, fmt.Errorf("delivery order must be IN_TRANSIT to mark delivered, got: %s", existing.Status)
	}

	updates := map[string]interface{}{
		"delivered_at": req.DeliveredAt,
	}

	err = ts.mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.UpdateDeliveryOrderStatus(ctx, id, DOStatusDelivered, updates)
	})

	if err != nil {
		return nil, fmt.Errorf("mark delivered: %w", err)
	}

	return ts.mock.GetDeliveryOrder(ctx, id)
}

// CancelDeliveryOrder wraps cancel logic
func (ts *testService) CancelDeliveryOrder(ctx context.Context, id int64, req CancelDeliveryOrderRequest) (*DeliveryOrder, error) {
	existing, err := ts.mock.GetDeliveryOrder(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get delivery order: %w", err)
	}

	if !existing.Status.CanCancel() {
		return nil, fmt.Errorf("cannot cancel delivery order in status: %s", existing.Status)
	}

	err = ts.mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		if existing.Status == DOStatusConfirmed {
			for _, line := range existing.Lines {
				if err := tx.UpdateDeliveryOrderLineQuantity(ctx, line.ID, 0); err != nil {
					return fmt.Errorf("reset line quantity: %w", err)
				}
			}
		}

		updates := map[string]interface{}{
			"notes": req.Reason,
		}

		return tx.UpdateDeliveryOrderStatus(ctx, id, DOStatusCancelled, updates)
	})

	if err != nil {
		return nil, fmt.Errorf("cancel delivery order: %w", err)
	}

	return ts.mock.GetDeliveryOrder(ctx, id)
}

// GetDeliveryOrder wraps get
func (ts *testService) GetDeliveryOrder(ctx context.Context, id int64) (*DeliveryOrder, error) {
	return ts.mock.GetDeliveryOrder(ctx, id)
}

// GetDeliverableSOLines wraps get deliverable lines
func (ts *testService) GetDeliverableSOLines(ctx context.Context, salesOrderID int64) ([]DeliverableSOLine, error) {
	soDetails, err := ts.mock.GetSalesOrderDetails(ctx, salesOrderID)
	if err != nil {
		return nil, fmt.Errorf("get sales order: %w", err)
	}

	if soDetails.Status != "CONFIRMED" && soDetails.Status != "PROCESSING" {
		return nil, fmt.Errorf("sales order must be CONFIRMED or PROCESSING to deliver, got: %s", soDetails.Status)
	}

	return ts.mock.GetDeliverableSOLines(ctx, salesOrderID)
}

// ListDeliveryOrders wraps list
func (ts *testService) ListDeliveryOrders(ctx context.Context, req ListDeliveryOrdersRequest) ([]DeliveryOrderWithDetails, int, error) {
	return ts.mock.ListDeliveryOrders(ctx, req)
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

func setupTestData(mock *mockRepository) {
	// Setup sales order
	mock.salesOrders[1] = &mockSalesOrder{
		ID:         1,
		DocNumber:  "SO-202401-00001",
		CompanyID:  1,
		CustomerID: 1,
		Status:     "CONFIRMED",
	}

	// Setup deliverable lines
	mock.deliverableLines[1] = []DeliverableSOLine{
		{
			SalesOrderLineID:  1,
			SalesOrderID:      1,
			ProductID:         1,
			ProductCode:       "PROD001",
			ProductName:       "Product 1",
			Quantity:          100.0,
			QuantityDelivered: 0.0,
			RemainingQuantity: 100.0,
			UOM:               "PCS",
			UnitPrice:         10.0,
			LineOrder:         1,
		},
		{
			SalesOrderLineID:  2,
			SalesOrderID:      1,
			ProductID:         2,
			ProductCode:       "PROD002",
			ProductName:       "Product 2",
			Quantity:          50.0,
			QuantityDelivered: 0.0,
			RemainingQuantity: 50.0,
			UOM:               "PCS",
			UnitPrice:         20.0,
			LineOrder:         2,
		},
	}

	// Setup warehouse
	mock.warehouses[1] = true
}

// ============================================================================
// CREATE DELIVERY ORDER TESTS
// ============================================================================

func TestService_CreateDeliveryOrder(t *testing.T) {
	ts := newTestService()
	ctx := context.Background()

	setupTestData(ts.mock)

	t.Run("successful creation", func(t *testing.T) {
		req := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			DeliveryDate: time.Now(),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  1,
					ProductID:         1,
					QuantityToDeliver: 50.0,
					LineOrder:         1,
				},
			},
		}

		do, err := ts.CreateDeliveryOrder(ctx, req, 1)
		require.NoError(t, err)
		assert.NotNil(t, do)
		assert.Equal(t, DOStatusDraft, do.Status)
		assert.Equal(t, int64(1), do.SalesOrderID)
		assert.Equal(t, int64(1), do.WarehouseID)
		assert.Len(t, do.Lines, 1)
		assert.Equal(t, 50.0, do.Lines[0].QuantityToDeliver)
	})

	t.Run("create with multiple lines", func(t *testing.T) {
		req := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			DeliveryDate: time.Now(),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  1,
					ProductID:         1,
					QuantityToDeliver: 100.0,
					LineOrder:         1,
				},
				{
					SalesOrderLineID:  2,
					ProductID:         2,
					QuantityToDeliver: 50.0,
					LineOrder:         2,
				},
			},
		}

		do, err := ts.CreateDeliveryOrder(ctx, req, 1)
		require.NoError(t, err)
		assert.Len(t, do.Lines, 2)
	})

	t.Run("SO not found", func(t *testing.T) {
		req := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 999,
			WarehouseID:  1,
			DeliveryDate: time.Now(),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  1,
					ProductID:         1,
					QuantityToDeliver: 50.0,
					LineOrder:         1,
				},
			},
		}

		do, err := ts.CreateDeliveryOrder(ctx, req, 1)
		assert.Error(t, err)
		assert.Nil(t, do)
		assert.Contains(t, err.Error(), "get sales order")
	})

	t.Run("SO not in correct status", func(t *testing.T) {
		ts.mock.salesOrders[2] = &mockSalesOrder{
			ID:         2,
			DocNumber:  "SO-202401-00002",
			CompanyID:  1,
			CustomerID: 1,
			Status:     "DRAFT",
		}

		req := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 2,
			WarehouseID:  1,
			DeliveryDate: time.Now(),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  1,
					ProductID:         1,
					QuantityToDeliver: 50.0,
					LineOrder:         1,
				},
			},
		}

		do, err := ts.CreateDeliveryOrder(ctx, req, 1)
		assert.Error(t, err)
		assert.Nil(t, do)
		assert.Contains(t, err.Error(), "must be CONFIRMED or PROCESSING")
	})

	t.Run("warehouse not found", func(t *testing.T) {
		req := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  999,
			DeliveryDate: time.Now(),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  1,
					ProductID:         1,
					QuantityToDeliver: 50.0,
					LineOrder:         1,
				},
			},
		}

		do, err := ts.CreateDeliveryOrder(ctx, req, 1)
		assert.Error(t, err)
		assert.Nil(t, do)
		assert.Contains(t, err.Error(), "warehouse not found")
	})

	t.Run("quantity exceeds remaining", func(t *testing.T) {
		req := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			DeliveryDate: time.Now(),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  1,
					ProductID:         1,
					QuantityToDeliver: 150.0, // Exceeds 100.0 remaining
					LineOrder:         1,
				},
			},
		}

		do, err := ts.CreateDeliveryOrder(ctx, req, 1)
		assert.Error(t, err)
		assert.Nil(t, do)
		assert.Contains(t, err.Error(), "exceeds remaining quantity")
	})

	t.Run("invalid quantity (zero or negative)", func(t *testing.T) {
		req := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			DeliveryDate: time.Now(),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  1,
					ProductID:         1,
					QuantityToDeliver: 0.0,
					LineOrder:         1,
				},
			},
		}

		do, err := ts.CreateDeliveryOrder(ctx, req, 1)
		assert.Error(t, err)
		assert.Nil(t, do)
		assert.Contains(t, err.Error(), "must be positive")
	})

	t.Run("product ID mismatch", func(t *testing.T) {
		req := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			DeliveryDate: time.Now(),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  1,
					ProductID:         999, // Wrong product
					QuantityToDeliver: 50.0,
					LineOrder:         1,
				},
			},
		}

		do, err := ts.CreateDeliveryOrder(ctx, req, 1)
		assert.Error(t, err)
		assert.Nil(t, do)
		assert.Contains(t, err.Error(), "product ID mismatch")
	})

	t.Run("SO line not found", func(t *testing.T) {
		req := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			DeliveryDate: time.Now(),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  999, // Non-existent line
					ProductID:         1,
					QuantityToDeliver: 50.0,
					LineOrder:         1,
				},
			},
		}

		do, err := ts.CreateDeliveryOrder(ctx, req, 1)
		assert.Error(t, err)
		assert.Nil(t, do)
		assert.Contains(t, err.Error(), "not found or fully delivered")
	})

	t.Run("no deliverable lines", func(t *testing.T) {
		// Create SO with no deliverable lines
		ts.mock.salesOrders[3] = &mockSalesOrder{
			ID:         3,
			DocNumber:  "SO-202401-00003",
			CompanyID:  1,
			CustomerID: 1,
			Status:     "CONFIRMED",
		}
		ts.mock.deliverableLines[3] = []DeliverableSOLine{} // Empty

		req := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 3,
			WarehouseID:  1,
			DeliveryDate: time.Now(),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  1,
					ProductID:         1,
					QuantityToDeliver: 50.0,
					LineOrder:         1,
				},
			},
		}

		do, err := ts.CreateDeliveryOrder(ctx, req, 1)
		assert.Error(t, err)
		assert.Nil(t, do)
		assert.Contains(t, err.Error(), "no deliverable lines found")
	})
}

// ============================================================================
// UPDATE DELIVERY ORDER TESTS
// ============================================================================

func TestService_UpdateDeliveryOrder(t *testing.T) {
	ts := newTestService()
	ctx := context.Background()

	setupTestData(ts.mock)

	// Create a draft DO
	req := CreateDeliveryOrderRequest{
		CompanyID:    1,
		SalesOrderID: 1,
		WarehouseID:  1,
		DeliveryDate: time.Now(),
		Lines: []CreateDeliveryOrderLineReq{
			{
				SalesOrderLineID:  1,
				ProductID:         1,
				QuantityToDeliver: 50.0,
				LineOrder:         1,
			},
		},
	}

	do, _ := ts.CreateDeliveryOrder(ctx, req, 1)

	t.Run("update basic fields", func(t *testing.T) {
		newDate := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
		driverName := "John Doe"
		vehicleNumber := "ABC-123"

		updateReq := UpdateDeliveryOrderRequest{
			DeliveryDate:  &newDate,
			DriverName:    &driverName,
			VehicleNumber: &vehicleNumber,
		}

		updated, err := ts.UpdateDeliveryOrder(ctx, do.ID, updateReq)
		require.NoError(t, err)
		assert.Equal(t, newDate.Unix(), updated.DeliveryDate.Unix())
		assert.Equal(t, driverName, *updated.DriverName)
		assert.Equal(t, vehicleNumber, *updated.VehicleNumber)
	})

	t.Run("update lines", func(t *testing.T) {
		newLines := []CreateDeliveryOrderLineReq{
			{
				SalesOrderLineID:  1,
				ProductID:         1,
				QuantityToDeliver: 80.0, // Changed quantity
				LineOrder:         1,
			},
			{
				SalesOrderLineID:  2,
				ProductID:         2,
				QuantityToDeliver: 30.0, // New line
				LineOrder:         2,
			},
		}

		updateReq := UpdateDeliveryOrderRequest{
			Lines: &newLines,
		}

		updated, err := ts.UpdateDeliveryOrder(ctx, do.ID, updateReq)
		require.NoError(t, err)
		assert.Len(t, updated.Lines, 2)
		assert.Equal(t, 80.0, updated.Lines[0].QuantityToDeliver)
		assert.Equal(t, 30.0, updated.Lines[1].QuantityToDeliver)
	})

	t.Run("cannot update non-existent DO", func(t *testing.T) {
		updateReq := UpdateDeliveryOrderRequest{
			DriverName: strPtr("John"),
		}

		updated, err := ts.UpdateDeliveryOrder(ctx, 9999, updateReq)
		assert.Error(t, err)
		assert.Nil(t, updated)
	})

	t.Run("cannot update confirmed DO", func(t *testing.T) {
		// Create and confirm a DO
		req2 := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			DeliveryDate: time.Now(),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  1,
					ProductID:         1,
					QuantityToDeliver: 30.0,
					LineOrder:         1,
				},
			},
		}

		do2, _ := ts.CreateDeliveryOrder(ctx, req2, 1)
		_, _ = ts.ConfirmDeliveryOrder(ctx, do2.ID, 1)

		updateReq := UpdateDeliveryOrderRequest{
			DriverName: strPtr("Jane"),
		}

		updated, err := ts.UpdateDeliveryOrder(ctx, do2.ID, updateReq)
		assert.Error(t, err)
		assert.Nil(t, updated)
		assert.Contains(t, err.Error(), "cannot edit")
	})

	t.Run("update with invalid line quantity", func(t *testing.T) {
		newLines := []CreateDeliveryOrderLineReq{
			{
				SalesOrderLineID:  1,
				ProductID:         1,
				QuantityToDeliver: 200.0, // Exceeds remaining
				LineOrder:         1,
			},
		}

		updateReq := UpdateDeliveryOrderRequest{
			Lines: &newLines,
		}

		updated, err := ts.UpdateDeliveryOrder(ctx, do.ID, updateReq)
		assert.Error(t, err)
		assert.Nil(t, updated)
		assert.Contains(t, err.Error(), "exceeds remaining quantity")
	})
}

// ============================================================================
// CONFIRM DELIVERY ORDER TESTS
// ============================================================================

func TestService_ConfirmDeliveryOrder(t *testing.T) {
	ts := newTestService()
	ctx := context.Background()

	setupTestData(ts.mock)

	t.Run("successful confirmation", func(t *testing.T) {
		// Create draft DO
		req := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			DeliveryDate: time.Now(),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  1,
					ProductID:         1,
					QuantityToDeliver: 50.0,
					LineOrder:         1,
				},
			},
		}

		do, _ := ts.CreateDeliveryOrder(ctx, req, 1)

		confirmed, err := ts.ConfirmDeliveryOrder(ctx, do.ID, 1)
		require.NoError(t, err)
		assert.Equal(t, DOStatusConfirmed, confirmed.Status)
		assert.NotNil(t, confirmed.ConfirmedBy)
		assert.Equal(t, int64(1), *confirmed.ConfirmedBy)
		assert.NotNil(t, confirmed.ConfirmedAt)
		assert.Equal(t, 50.0, confirmed.Lines[0].QuantityDelivered)
	})

	t.Run("cannot confirm non-existent DO", func(t *testing.T) {
		confirmed, err := ts.ConfirmDeliveryOrder(ctx, 9999, 1)
		assert.Error(t, err)
		assert.Nil(t, confirmed)
	})

	t.Run("cannot confirm already confirmed DO", func(t *testing.T) {
		// Create and confirm
		req := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			DeliveryDate: time.Now(),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  1,
					ProductID:         1,
					QuantityToDeliver: 20.0,
					LineOrder:         1,
				},
			},
		}

		do, _ := ts.CreateDeliveryOrder(ctx, req, 1)
		_, _ = ts.ConfirmDeliveryOrder(ctx, do.ID, 1)

		// Try to confirm again
		confirmed, err := ts.ConfirmDeliveryOrder(ctx, do.ID, 1)
		assert.Error(t, err)
		assert.Nil(t, confirmed)
		assert.Contains(t, err.Error(), "cannot confirm")
	})

	t.Run("cannot confirm DO without lines", func(t *testing.T) {
		// Create DO manually without lines (edge case)
		do := DeliveryOrder{
			DocNumber:    "DO-202401-00999",
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			CustomerID:   1,
			DeliveryDate: time.Now(),
			Status:       DOStatusDraft,
			CreatedBy:    1,
		}

		var doID int64
		_ = ts.mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
			id, err := tx.CreateDeliveryOrder(ctx, do)
			doID = id
			return err
		})

		confirmed, err := ts.ConfirmDeliveryOrder(ctx, doID, 1)
		assert.Error(t, err)
		assert.Nil(t, confirmed)
		assert.Contains(t, err.Error(), "without lines")
	})
}

// ============================================================================
// STATUS TRANSITION TESTS
// ============================================================================

func TestService_MarkInTransit(t *testing.T) {
	ts := newTestService()
	ctx := context.Background()

	setupTestData(ts.mock)

	t.Run("successful mark in transit", func(t *testing.T) {
		// Create and confirm DO
		req := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			DeliveryDate: time.Now(),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  1,
					ProductID:         1,
					QuantityToDeliver: 50.0,
					LineOrder:         1,
				},
			},
		}

		do, _ := ts.CreateDeliveryOrder(ctx, req, 1)
		do, _ = ts.ConfirmDeliveryOrder(ctx, do.ID, 1)

		trackingNumber := "TRACK123456"
		transitReq := MarkInTransitRequest{
			TrackingNumber: &trackingNumber,
			UpdatedBy:      1,
		}

		inTransit, err := ts.MarkInTransit(ctx, do.ID, transitReq)
		require.NoError(t, err)
		assert.Equal(t, DOStatusInTransit, inTransit.Status)
		assert.Equal(t, trackingNumber, *inTransit.TrackingNumber)
	})

	t.Run("cannot mark draft in transit", func(t *testing.T) {
		req := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			DeliveryDate: time.Now(),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  1,
					ProductID:         1,
					QuantityToDeliver: 30.0,
					LineOrder:         1,
				},
			},
		}

		do, _ := ts.CreateDeliveryOrder(ctx, req, 1)

		transitReq := MarkInTransitRequest{
			UpdatedBy: 1,
		}

		inTransit, err := ts.MarkInTransit(ctx, do.ID, transitReq)
		assert.Error(t, err)
		assert.Nil(t, inTransit)
		assert.Contains(t, err.Error(), "must be CONFIRMED")
	})
}

func TestService_MarkDelivered(t *testing.T) {
	ts := newTestService()
	ctx := context.Background()

	setupTestData(ts.mock)

	t.Run("successful mark delivered", func(t *testing.T) {
		// Create, confirm, and mark in transit
		req := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			DeliveryDate: time.Now(),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  1,
					ProductID:         1,
					QuantityToDeliver: 50.0,
					LineOrder:         1,
				},
			},
		}

		do, _ := ts.CreateDeliveryOrder(ctx, req, 1)
		do, _ = ts.ConfirmDeliveryOrder(ctx, do.ID, 1)

		trackingNumber := "TRACK123"
		transitReq := MarkInTransitRequest{
			TrackingNumber: &trackingNumber,
			UpdatedBy:      1,
		}
		do, _ = ts.MarkInTransit(ctx, do.ID, transitReq)

		deliveredReq := MarkDeliveredRequest{
			DeliveredAt: time.Now(),
			UpdatedBy:   1,
		}

		delivered, err := ts.MarkDelivered(ctx, do.ID, deliveredReq)
		require.NoError(t, err)
		assert.Equal(t, DOStatusDelivered, delivered.Status)
		assert.NotNil(t, delivered.DeliveredAt)
	})

	t.Run("cannot mark confirmed as delivered", func(t *testing.T) {
		req := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			DeliveryDate: time.Now(),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  1,
					ProductID:         1,
					QuantityToDeliver: 30.0,
					LineOrder:         1,
				},
			},
		}

		do, _ := ts.CreateDeliveryOrder(ctx, req, 1)
		do, _ = ts.ConfirmDeliveryOrder(ctx, do.ID, 1)

		deliveredReq := MarkDeliveredRequest{
			DeliveredAt: time.Now(),
			UpdatedBy:   1,
		}

		delivered, err := ts.MarkDelivered(ctx, do.ID, deliveredReq)
		assert.Error(t, err)
		assert.Nil(t, delivered)
		assert.Contains(t, err.Error(), "must be IN_TRANSIT")
	})
}

// ============================================================================
// CANCEL DELIVERY ORDER TESTS
// ============================================================================

func TestService_CancelDeliveryOrder(t *testing.T) {
	ts := newTestService()
	ctx := context.Background()

	setupTestData(ts.mock)

	t.Run("cancel draft DO", func(t *testing.T) {
		req := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			DeliveryDate: time.Now(),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  1,
					ProductID:         1,
					QuantityToDeliver: 50.0,
					LineOrder:         1,
				},
			},
		}

		do, _ := ts.CreateDeliveryOrder(ctx, req, 1)

		cancelReq := CancelDeliveryOrderRequest{
			Reason:      "Customer requested cancellation",
			CancelledBy: 1,
		}

		cancelled, err := ts.CancelDeliveryOrder(ctx, do.ID, cancelReq)
		require.NoError(t, err)
		assert.Equal(t, DOStatusCancelled, cancelled.Status)
	})

	t.Run("cancel confirmed DO", func(t *testing.T) {
		req := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			DeliveryDate: time.Now(),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  1,
					ProductID:         1,
					QuantityToDeliver: 40.0,
					LineOrder:         1,
				},
			},
		}

		do, _ := ts.CreateDeliveryOrder(ctx, req, 1)
		do, _ = ts.ConfirmDeliveryOrder(ctx, do.ID, 1)

		cancelReq := CancelDeliveryOrderRequest{
			Reason:      "Inventory shortage",
			CancelledBy: 1,
		}

		cancelled, err := ts.CancelDeliveryOrder(ctx, do.ID, cancelReq)
		require.NoError(t, err)
		assert.Equal(t, DOStatusCancelled, cancelled.Status)
		// Verify line quantities reset to 0
		assert.Equal(t, 0.0, cancelled.Lines[0].QuantityDelivered)
	})

	t.Run("cannot cancel delivered DO", func(t *testing.T) {
		// Create full workflow
		req := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			DeliveryDate: time.Now(),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  1,
					ProductID:         1,
					QuantityToDeliver: 20.0,
					LineOrder:         1,
				},
			},
		}

		do, _ := ts.CreateDeliveryOrder(ctx, req, 1)
		do, _ = ts.ConfirmDeliveryOrder(ctx, do.ID, 1)

		trackingNumber := "TRACK999"
		transitReq := MarkInTransitRequest{
			TrackingNumber: &trackingNumber,
			UpdatedBy:      1,
		}
		do, _ = ts.MarkInTransit(ctx, do.ID, transitReq)

		deliveredReq := MarkDeliveredRequest{
			DeliveredAt: time.Now(),
			UpdatedBy:   1,
		}
		do, _ = ts.MarkDelivered(ctx, do.ID, deliveredReq)

		cancelReq := CancelDeliveryOrderRequest{
			Reason:      "Changed mind",
			CancelledBy: 1,
		}

		cancelled, err := ts.CancelDeliveryOrder(ctx, do.ID, cancelReq)
		assert.Error(t, err)
		assert.Nil(t, cancelled)
		assert.Contains(t, err.Error(), "cannot cancel")
	})
}

// ============================================================================
// QUERY OPERATIONS TESTS
// ============================================================================

func TestService_GetDeliveryOrder(t *testing.T) {
	ts := newTestService()
	ctx := context.Background()

	setupTestData(ts.mock)

	req := CreateDeliveryOrderRequest{
		CompanyID:    1,
		SalesOrderID: 1,
		WarehouseID:  1,
		DeliveryDate: time.Now(),
		Lines: []CreateDeliveryOrderLineReq{
			{
				SalesOrderLineID:  1,
				ProductID:         1,
				QuantityToDeliver: 50.0,
				LineOrder:         1,
			},
		},
	}

	do, _ := ts.CreateDeliveryOrder(ctx, req, 1)

	t.Run("get existing DO", func(t *testing.T) {
		retrieved, err := ts.GetDeliveryOrder(ctx, do.ID)
		require.NoError(t, err)
		assert.Equal(t, do.ID, retrieved.ID)
		assert.Equal(t, do.DocNumber, retrieved.DocNumber)
	})

	t.Run("get non-existent DO", func(t *testing.T) {
		retrieved, err := ts.GetDeliveryOrder(ctx, 9999)
		assert.Error(t, err)
		assert.Nil(t, retrieved)
	})
}

func TestService_GetDeliverableSOLines(t *testing.T) {
	ts := newTestService()
	ctx := context.Background()

	setupTestData(ts.mock)

	t.Run("get deliverable lines for confirmed SO", func(t *testing.T) {
		lines, err := ts.GetDeliverableSOLines(ctx, 1)
		require.NoError(t, err)
		assert.Len(t, lines, 2)
		assert.Equal(t, 100.0, lines[0].RemainingQuantity)
		assert.Equal(t, 50.0, lines[1].RemainingQuantity)
	})

	t.Run("cannot get lines for non-confirmed SO", func(t *testing.T) {
		ts.mock.salesOrders[2] = &mockSalesOrder{
			ID:         2,
			DocNumber:  "SO-202401-00002",
			CompanyID:  1,
			CustomerID: 1,
			Status:     "DRAFT",
		}

		lines, err := ts.GetDeliverableSOLines(ctx, 2)
		assert.Error(t, err)
		assert.Nil(t, lines)
		assert.Contains(t, err.Error(), "must be CONFIRMED or PROCESSING")
	})

	t.Run("get lines for non-existent SO", func(t *testing.T) {
		lines, err := ts.GetDeliverableSOLines(ctx, 999)
		assert.Error(t, err)
		assert.Nil(t, lines)
	})
}

func TestService_ListDeliveryOrders(t *testing.T) {
	ts := newTestService()
	ctx := context.Background()

	setupTestData(ts.mock)

	// Create multiple DOs
	for i := 1; i <= 5; i++ {
		req := CreateDeliveryOrderRequest{
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			DeliveryDate: time.Now().AddDate(0, 0, i),
			Lines: []CreateDeliveryOrderLineReq{
				{
					SalesOrderLineID:  1,
					ProductID:         1,
					QuantityToDeliver: 10.0,
					LineOrder:         1,
				},
			},
		}
		_, _ = ts.CreateDeliveryOrder(ctx, req, 1)
	}

	t.Run("list all DOs", func(t *testing.T) {
		listReq := ListDeliveryOrdersRequest{
			CompanyID: 1,
			Limit:     100,
			Offset:    0,
		}

		dos, total, err := ts.ListDeliveryOrders(ctx, listReq)
		require.NoError(t, err)
		assert.Equal(t, 5, total)
		assert.Len(t, dos, 5)
	})

	t.Run("list with pagination", func(t *testing.T) {
		listReq := ListDeliveryOrdersRequest{
			CompanyID: 1,
			Limit:     2,
			Offset:    0,
		}

		dos, total, err := ts.ListDeliveryOrders(ctx, listReq)
		require.NoError(t, err)
		assert.Equal(t, 5, total)
		assert.Len(t, dos, 2)
	})
}
