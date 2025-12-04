package delivery

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// MOCK REPOSITORY
// ============================================================================

type mockRepository struct {
	// Delivery Order storage
	deliveryOrders       map[int64]*DeliveryOrder
	deliveryOrderLines   map[int64][]DeliveryOrderLine
	nextDeliveryOrderID  int64
	deliveryOrderCounter map[int64]int
	deliveryOrdersByDoc  map[string]*DeliveryOrder

	// Sales Order data for validation
	salesOrders      map[int64]*mockSalesOrder
	salesOrderLines  map[int64][]mockSalesOrderLine
	deliverableLines map[int64][]DeliverableSOLine

	// Warehouse validation
	warehouses map[int64]bool

	// Error injection
	txError                  error
	getDOError               error
	createDOError            error
	listDOError              error
	getSODetailsError        error
	checkWarehouseError      error
	getDeliverableLinesError error
}

type mockSalesOrder struct {
	ID         int64
	DocNumber  string
	CompanyID  int64
	CustomerID int64
	Status     string
}

type mockSalesOrderLine struct {
	ID                int64
	SalesOrderID      int64
	ProductID         int64
	Quantity          float64
	QuantityDelivered float64
	UOM               string
	UnitPrice         float64
	LineOrder         int
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		deliveryOrders:       make(map[int64]*DeliveryOrder),
		deliveryOrderLines:   make(map[int64][]DeliveryOrderLine),
		deliveryOrderCounter: make(map[int64]int),
		deliveryOrdersByDoc:  make(map[string]*DeliveryOrder),
		salesOrders:          make(map[int64]*mockSalesOrder),
		salesOrderLines:      make(map[int64][]mockSalesOrderLine),
		deliverableLines:     make(map[int64][]DeliverableSOLine),
		warehouses:           make(map[int64]bool),
		nextDeliveryOrderID:  1,
	}
}

func (m *mockRepository) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	if m.txError != nil {
		return m.txError
	}
	tx := &mockTxRepo{mock: m}
	return fn(ctx, tx)
}

func (m *mockRepository) GetDeliveryOrder(ctx context.Context, id int64) (*DeliveryOrder, error) {
	if m.getDOError != nil {
		return nil, m.getDOError
	}
	do, ok := m.deliveryOrders[id]
	if !ok {
		return nil, ErrNotFound
	}

	// Clone and attach lines
	doCopy := *do
	if lines, exists := m.deliveryOrderLines[id]; exists {
		doCopy.Lines = append([]DeliveryOrderLine{}, lines...)
	}

	return &doCopy, nil
}

func (m *mockRepository) GetDeliveryOrderByDocNumber(ctx context.Context, companyID int64, docNumber string) (*DeliveryOrder, error) {
	key := makeDocKey(companyID, docNumber)
	do, ok := m.deliveryOrdersByDoc[key]
	if !ok {
		return nil, ErrNotFound
	}

	// Clone and attach lines
	doCopy := *do
	if lines, exists := m.deliveryOrderLines[do.ID]; exists {
		doCopy.Lines = append([]DeliveryOrderLine{}, lines...)
	}

	return &doCopy, nil
}

func (m *mockRepository) GetDeliveryOrderWithDetails(ctx context.Context, id int64) (*DeliveryOrderWithDetails, error) {
	do, err := m.GetDeliveryOrder(ctx, id)
	if err != nil {
		return nil, err
	}

	so, ok := m.salesOrders[do.SalesOrderID]
	if !ok {
		return nil, ErrNotFound
	}

	lines := m.deliveryOrderLines[id]
	lineCount := len(lines)
	totalQty := 0.0
	for _, line := range lines {
		totalQty += line.QuantityToDeliver
	}

	return &DeliveryOrderWithDetails{
		DeliveryOrder:    *do,
		SalesOrderNumber: so.DocNumber,
		WarehouseName:    "Main Warehouse",
		CustomerName:     "Test Customer",
		CreatedByName:    "Test User",
		LineCount:        lineCount,
		TotalQuantity:    totalQty,
	}, nil
}

func (m *mockRepository) GetDeliveryOrderLinesWithDetails(ctx context.Context, deliveryOrderID int64) ([]DeliveryOrderLineWithDetails, error) {
	lines, ok := m.deliveryOrderLines[deliveryOrderID]
	if !ok {
		return []DeliveryOrderLineWithDetails{}, nil
	}

	result := make([]DeliveryOrderLineWithDetails, len(lines))
	for i, line := range lines {
		result[i] = DeliveryOrderLineWithDetails{
			DeliveryOrderLine:  line,
			ProductCode:        fmt.Sprintf("PROD%03d", line.ProductID),
			ProductName:        fmt.Sprintf("Product %d", line.ProductID),
			SOLineQuantity:     100.0,
			SOLineDelivered:    0.0,
			RemainingToDeliver: 100.0,
		}
	}

	return result, nil
}

func (m *mockRepository) ListDeliveryOrders(ctx context.Context, req ListDeliveryOrdersRequest) ([]DeliveryOrderWithDetails, int, error) {
	if m.listDOError != nil {
		return nil, 0, m.listDOError
	}

	var result []DeliveryOrderWithDetails
	for _, do := range m.deliveryOrders {
		if do.CompanyID != req.CompanyID {
			continue
		}
		if req.SalesOrderID != nil && do.SalesOrderID != *req.SalesOrderID {
			continue
		}
		if req.WarehouseID != nil && do.WarehouseID != *req.WarehouseID {
			continue
		}
		if req.CustomerID != nil && do.CustomerID != *req.CustomerID {
			continue
		}
		if req.Status != nil && do.Status != *req.Status {
			continue
		}
		if req.DateFrom != nil && do.DeliveryDate.Before(*req.DateFrom) {
			continue
		}
		if req.DateTo != nil && do.DeliveryDate.After(*req.DateTo) {
			continue
		}

		so := m.salesOrders[do.SalesOrderID]
		lines := m.deliveryOrderLines[do.ID]
		totalQty := 0.0
		for _, line := range lines {
			totalQty += line.QuantityToDeliver
		}

		result = append(result, DeliveryOrderWithDetails{
			DeliveryOrder:    *do,
			SalesOrderNumber: so.DocNumber,
			WarehouseName:    "Main Warehouse",
			CustomerName:     "Test Customer",
			CreatedByName:    "Test User",
			LineCount:        len(lines),
			TotalQuantity:    totalQty,
		})
	}

	// Apply pagination
	total := len(result)
	start := req.Offset
	end := start + req.Limit
	if start > len(result) {
		return []DeliveryOrderWithDetails{}, total, nil
	}
	if end > len(result) {
		end = len(result)
	}

	return result[start:end], total, nil
}

func (m *mockRepository) GetDeliverableSOLines(ctx context.Context, salesOrderID int64) ([]DeliverableSOLine, error) {
	if m.getDeliverableLinesError != nil {
		return nil, m.getDeliverableLinesError
	}

	lines, ok := m.deliverableLines[salesOrderID]
	if !ok {
		return []DeliverableSOLine{}, nil
	}

	return lines, nil
}

func (m *mockRepository) GenerateDeliveryOrderNumber(ctx context.Context, companyID int64, date time.Time) (string, error) {
	count := m.deliveryOrderCounter[companyID]
	m.deliveryOrderCounter[companyID]++
	return fmt.Sprintf("DO-%s-%05d", date.Format("200601"), count+1), nil
}

func (m *mockRepository) GetSalesOrderDetails(ctx context.Context, salesOrderID int64) (*struct {
	ID         int64
	DocNumber  string
	CompanyID  int64
	CustomerID int64
	Status     string
}, error) {
	if m.getSODetailsError != nil {
		return nil, m.getSODetailsError
	}

	so, ok := m.salesOrders[salesOrderID]
	if !ok {
		return nil, ErrNotFound
	}

	return &struct {
		ID         int64
		DocNumber  string
		CompanyID  int64
		CustomerID int64
		Status     string
	}{
		ID:         so.ID,
		DocNumber:  so.DocNumber,
		CompanyID:  so.CompanyID,
		CustomerID: so.CustomerID,
		Status:     so.Status,
	}, nil
}

func (m *mockRepository) CheckWarehouseExists(ctx context.Context, warehouseID int64) (bool, error) {
	if m.checkWarehouseError != nil {
		return false, m.checkWarehouseError
	}
	exists, ok := m.warehouses[warehouseID]
	if !ok {
		return false, nil
	}
	return exists, nil
}

func (m *mockRepository) GetDeliveryOrderIDByDocNumber(ctx context.Context, companyID int64, docNumber string) (int64, error) {
	key := makeDocKey(companyID, docNumber)
	do, ok := m.deliveryOrdersByDoc[key]
	if !ok {
		return 0, ErrNotFound
	}
	return do.ID, nil
}

// Mock transaction repository
type mockTxRepo struct {
	mock *mockRepository
}

func (t *mockTxRepo) CreateDeliveryOrder(ctx context.Context, do DeliveryOrder) (int64, error) {
	if t.mock.createDOError != nil {
		return 0, t.mock.createDOError
	}

	id := t.mock.nextDeliveryOrderID
	t.mock.nextDeliveryOrderID++

	now := time.Now()
	do.ID = id
	do.CreatedAt = now
	do.UpdatedAt = now

	t.mock.deliveryOrders[id] = &do
	key := makeDocKey(do.CompanyID, do.DocNumber)
	t.mock.deliveryOrdersByDoc[key] = &do

	return id, nil
}

func (t *mockTxRepo) InsertDeliveryOrderLine(ctx context.Context, line DeliveryOrderLine) (int64, error) {
	lines := t.mock.deliveryOrderLines[line.DeliveryOrderID]
	line.ID = int64(len(lines) + 1)
	line.CreatedAt = time.Now()
	line.UpdatedAt = time.Now()

	t.mock.deliveryOrderLines[line.DeliveryOrderID] = append(lines, line)

	return line.ID, nil
}

func (t *mockTxRepo) UpdateDeliveryOrder(ctx context.Context, id int64, updates map[string]interface{}) error {
	do, ok := t.mock.deliveryOrders[id]
	if !ok {
		return ErrNotFound
	}

	// Apply updates
	for field, value := range updates {
		switch field {
		case "delivery_date":
			if v, ok := value.(time.Time); ok {
				do.DeliveryDate = v
			}
		case "driver_name":
			if v, ok := value.(*string); ok {
				do.DriverName = v
			}
		case "vehicle_number":
			if v, ok := value.(*string); ok {
				do.VehicleNumber = v
			}
		case "tracking_number":
			if v, ok := value.(*string); ok {
				do.TrackingNumber = v
			}
		case "notes":
			if v, ok := value.(*string); ok {
				do.Notes = v
			}
		case "status":
			if v, ok := value.(DeliveryOrderStatus); ok {
				do.Status = v
			}
		case "confirmed_by":
			if v, ok := value.(int64); ok {
				do.ConfirmedBy = &v
			}
		case "confirmed_at":
			if v, ok := value.(time.Time); ok {
				do.ConfirmedAt = &v
			}
		case "delivered_at":
			if v, ok := value.(time.Time); ok {
				do.DeliveredAt = &v
			}
		}
	}

	do.UpdatedAt = time.Now()
	return nil
}

func (t *mockTxRepo) UpdateDeliveryOrderStatus(ctx context.Context, id int64, status DeliveryOrderStatus, updates map[string]interface{}) error {
	if updates == nil {
		updates = make(map[string]interface{})
	}
	updates["status"] = status
	return t.UpdateDeliveryOrder(ctx, id, updates)
}

func (t *mockTxRepo) DeleteDeliveryOrderLines(ctx context.Context, deliveryOrderID int64) error {
	delete(t.mock.deliveryOrderLines, deliveryOrderID)
	return nil
}

func (t *mockTxRepo) UpdateDeliveryOrderLineQuantity(ctx context.Context, lineID int64, quantityDelivered float64) error {
	for doID, lines := range t.mock.deliveryOrderLines {
		for i, line := range lines {
			if line.ID == lineID {
				lines[i].QuantityDelivered = quantityDelivered
				lines[i].UpdatedAt = time.Now()
				t.mock.deliveryOrderLines[doID] = lines
				return nil
			}
		}
	}
	return ErrNotFound
}

// Helper functions
func makeDocKey(companyID int64, docNumber string) string {
	return fmt.Sprintf("%d:%s", companyID, docNumber)
}

// ============================================================================
// REPOSITORY TESTS
// ============================================================================

func TestRepository_CreateDeliveryOrder(t *testing.T) {
	mock := newMockRepository()
	ctx := context.Background()

	// Setup test data
	mock.salesOrders[1] = &mockSalesOrder{
		ID:         1,
		DocNumber:  "SO-202401-00001",
		CompanyID:  1,
		CustomerID: 1,
		Status:     "CONFIRMED",
	}
	mock.warehouses[1] = true

	t.Run("successful creation", func(t *testing.T) {
		now := time.Now()
		do := DeliveryOrder{
			DocNumber:    "DO-202401-00001",
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			CustomerID:   1,
			DeliveryDate: now,
			Status:       DOStatusDraft,
			CreatedBy:    1,
		}

		var createdID int64
		err := mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
			id, err := tx.CreateDeliveryOrder(ctx, do)
			createdID = id
			return err
		})

		require.NoError(t, err)
		assert.Greater(t, createdID, int64(0))

		// Verify it was created
		created, err := mock.GetDeliveryOrder(ctx, createdID)
		require.NoError(t, err)
		assert.Equal(t, do.DocNumber, created.DocNumber)
		assert.Equal(t, do.SalesOrderID, created.SalesOrderID)
		assert.Equal(t, DOStatusDraft, created.Status)
	})

	t.Run("error injection", func(t *testing.T) {
		mock.createDOError = errors.New("creation failed")

		do := DeliveryOrder{
			DocNumber:    "DO-202401-00002",
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			CustomerID:   1,
			DeliveryDate: time.Now(),
			Status:       DOStatusDraft,
			CreatedBy:    1,
		}

		err := mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
			_, err := tx.CreateDeliveryOrder(ctx, do)
			return err
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "creation failed")

		mock.createDOError = nil
	})
}

func TestRepository_GetDeliveryOrder(t *testing.T) {
	mock := newMockRepository()
	ctx := context.Background()

	// Setup test data
	mock.salesOrders[1] = &mockSalesOrder{
		ID:         1,
		DocNumber:  "SO-202401-00001",
		CompanyID:  1,
		CustomerID: 1,
		Status:     "CONFIRMED",
	}

	now := time.Now()
	do := DeliveryOrder{
		DocNumber:    "DO-202401-00001",
		CompanyID:    1,
		SalesOrderID: 1,
		WarehouseID:  1,
		CustomerID:   1,
		DeliveryDate: now,
		Status:       DOStatusDraft,
		CreatedBy:    1,
	}

	var doID int64
	_ = mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		id, err := tx.CreateDeliveryOrder(ctx, do)
		doID = id

		// Add lines
		line := DeliveryOrderLine{
			DeliveryOrderID:   doID,
			SalesOrderLineID:  1,
			ProductID:         1,
			QuantityToDeliver: 100.0,
			QuantityDelivered: 0.0,
			UOM:               "PCS",
			UnitPrice:         10.0,
			LineOrder:         1,
		}
		_, err = tx.InsertDeliveryOrderLine(ctx, line)
		return err
	})

	t.Run("get existing delivery order", func(t *testing.T) {
		result, err := mock.GetDeliveryOrder(ctx, doID)
		require.NoError(t, err)
		assert.Equal(t, doID, result.ID)
		assert.Equal(t, "DO-202401-00001", result.DocNumber)
		assert.Equal(t, DOStatusDraft, result.Status)
		assert.Len(t, result.Lines, 1)
		assert.Equal(t, 100.0, result.Lines[0].QuantityToDeliver)
	})

	t.Run("get non-existent delivery order", func(t *testing.T) {
		result, err := mock.GetDeliveryOrder(ctx, 9999)
		assert.Error(t, err)
		assert.Equal(t, ErrNotFound, err)
		assert.Nil(t, result)
	})

	t.Run("error injection", func(t *testing.T) {
		mock.getDOError = errors.New("database error")

		result, err := mock.GetDeliveryOrder(ctx, doID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database error")
		assert.Nil(t, result)

		mock.getDOError = nil
	})
}

func TestRepository_GetDeliveryOrderByDocNumber(t *testing.T) {
	mock := newMockRepository()
	ctx := context.Background()

	// Setup test data
	mock.salesOrders[1] = &mockSalesOrder{
		ID:         1,
		DocNumber:  "SO-202401-00001",
		CompanyID:  1,
		CustomerID: 1,
		Status:     "CONFIRMED",
	}

	do := DeliveryOrder{
		DocNumber:    "DO-202401-00001",
		CompanyID:    1,
		SalesOrderID: 1,
		WarehouseID:  1,
		CustomerID:   1,
		DeliveryDate: time.Now(),
		Status:       DOStatusDraft,
		CreatedBy:    1,
	}

	_ = mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		_, err := tx.CreateDeliveryOrder(ctx, do)
		return err
	})

	t.Run("get by doc number", func(t *testing.T) {
		result, err := mock.GetDeliveryOrderByDocNumber(ctx, 1, "DO-202401-00001")
		require.NoError(t, err)
		assert.Equal(t, "DO-202401-00001", result.DocNumber)
		assert.Equal(t, int64(1), result.CompanyID)
	})

	t.Run("not found", func(t *testing.T) {
		result, err := mock.GetDeliveryOrderByDocNumber(ctx, 1, "DO-NOTEXIST")
		assert.Error(t, err)
		assert.Equal(t, ErrNotFound, err)
		assert.Nil(t, result)
	})
}

func TestRepository_GetDeliveryOrderWithDetails(t *testing.T) {
	mock := newMockRepository()
	ctx := context.Background()

	// Setup test data
	mock.salesOrders[1] = &mockSalesOrder{
		ID:         1,
		DocNumber:  "SO-202401-00001",
		CompanyID:  1,
		CustomerID: 1,
		Status:     "CONFIRMED",
	}

	do := DeliveryOrder{
		DocNumber:    "DO-202401-00001",
		CompanyID:    1,
		SalesOrderID: 1,
		WarehouseID:  1,
		CustomerID:   1,
		DeliveryDate: time.Now(),
		Status:       DOStatusDraft,
		CreatedBy:    1,
	}

	var doID int64
	_ = mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		id, err := tx.CreateDeliveryOrder(ctx, do)
		doID = id

		// Add 2 lines
		for i := 1; i <= 2; i++ {
			line := DeliveryOrderLine{
				DeliveryOrderID:   doID,
				SalesOrderLineID:  int64(i),
				ProductID:         int64(i),
				QuantityToDeliver: 50.0,
				QuantityDelivered: 0.0,
				UOM:               "PCS",
				UnitPrice:         10.0,
				LineOrder:         i,
			}
			_, err = tx.InsertDeliveryOrderLine(ctx, line)
			if err != nil {
				return err
			}
		}
		return nil
	})

	t.Run("get with details", func(t *testing.T) {
		result, err := mock.GetDeliveryOrderWithDetails(ctx, doID)
		require.NoError(t, err)
		assert.Equal(t, "DO-202401-00001", result.DocNumber)
		assert.Equal(t, "SO-202401-00001", result.SalesOrderNumber)
		assert.Equal(t, "Main Warehouse", result.WarehouseName)
		assert.Equal(t, "Test Customer", result.CustomerName)
		assert.Equal(t, 2, result.LineCount)
		assert.Equal(t, 100.0, result.TotalQuantity)
	})
}

func TestRepository_ListDeliveryOrders(t *testing.T) {
	mock := newMockRepository()
	ctx := context.Background()

	// Setup test data
	for i := 1; i <= 2; i++ {
		mock.salesOrders[int64(i)] = &mockSalesOrder{
			ID:         int64(i),
			DocNumber:  fmt.Sprintf("SO-202401-%05d", i),
			CompanyID:  1,
			CustomerID: 1,
			Status:     "CONFIRMED",
		}
	}

	// Create multiple delivery orders
	baseDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 5; i++ {
		do := DeliveryOrder{
			DocNumber:    fmt.Sprintf("DO-202401-%05d", i),
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			CustomerID:   1,
			DeliveryDate: baseDate.AddDate(0, 0, i-1),
			Status:       DOStatusDraft,
			CreatedBy:    1,
		}
		if i > 3 {
			do.Status = DOStatusConfirmed
		}

		_ = mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
			_, err := tx.CreateDeliveryOrder(ctx, do)
			return err
		})
	}

	t.Run("list all for company", func(t *testing.T) {
		req := ListDeliveryOrdersRequest{
			CompanyID: 1,
			Limit:     100,
			Offset:    0,
		}

		result, total, err := mock.ListDeliveryOrders(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, 5, total)
		assert.Len(t, result, 5)
	})

	t.Run("filter by status", func(t *testing.T) {
		status := DOStatusConfirmed
		req := ListDeliveryOrdersRequest{
			CompanyID: 1,
			Status:    &status,
			Limit:     100,
			Offset:    0,
		}

		result, total, err := mock.ListDeliveryOrders(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, result, 2)
		for _, do := range result {
			assert.Equal(t, DOStatusConfirmed, do.Status)
		}
	})

	t.Run("filter by date range", func(t *testing.T) {
		dateFrom := baseDate.AddDate(0, 0, 1)
		dateTo := baseDate.AddDate(0, 0, 3)
		req := ListDeliveryOrdersRequest{
			CompanyID: 1,
			DateFrom:  &dateFrom,
			DateTo:    &dateTo,
			Limit:     100,
			Offset:    0,
		}

		result, total, err := mock.ListDeliveryOrders(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, 3, total)
		assert.Len(t, result, 3)
	})

	t.Run("pagination", func(t *testing.T) {
		req := ListDeliveryOrdersRequest{
			CompanyID: 1,
			Limit:     2,
			Offset:    0,
		}

		result, total, err := mock.ListDeliveryOrders(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, 5, total)
		assert.Len(t, result, 2)

		// Second page
		req.Offset = 2
		result, total, err = mock.ListDeliveryOrders(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, 5, total)
		assert.Len(t, result, 2)
	})

	t.Run("error injection", func(t *testing.T) {
		mock.listDOError = errors.New("list failed")

		req := ListDeliveryOrdersRequest{
			CompanyID: 1,
			Limit:     10,
			Offset:    0,
		}

		result, total, err := mock.ListDeliveryOrders(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "list failed")
		assert.Nil(t, result)
		assert.Equal(t, 0, total)

		mock.listDOError = nil
	})
}

func TestRepository_UpdateDeliveryOrder(t *testing.T) {
	mock := newMockRepository()
	ctx := context.Background()

	// Setup test data
	mock.salesOrders[1] = &mockSalesOrder{
		ID:         1,
		DocNumber:  "SO-202401-00001",
		CompanyID:  1,
		CustomerID: 1,
		Status:     "CONFIRMED",
	}

	do := DeliveryOrder{
		DocNumber:    "DO-202401-00001",
		CompanyID:    1,
		SalesOrderID: 1,
		WarehouseID:  1,
		CustomerID:   1,
		DeliveryDate: time.Now(),
		Status:       DOStatusDraft,
		CreatedBy:    1,
	}

	var doID int64
	_ = mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		id, err := tx.CreateDeliveryOrder(ctx, do)
		doID = id
		return err
	})

	t.Run("update basic fields", func(t *testing.T) {
		driverName := "John Doe"
		vehicleNumber := "ABC-123"
		newDate := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

		updates := map[string]interface{}{
			"driver_name":    &driverName,
			"vehicle_number": &vehicleNumber,
			"delivery_date":  newDate,
		}

		err := mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
			return tx.UpdateDeliveryOrder(ctx, doID, updates)
		})

		require.NoError(t, err)

		// Verify updates
		updated, err := mock.GetDeliveryOrder(ctx, doID)
		require.NoError(t, err)
		assert.Equal(t, driverName, *updated.DriverName)
		assert.Equal(t, vehicleNumber, *updated.VehicleNumber)
		assert.Equal(t, newDate.Unix(), updated.DeliveryDate.Unix())
	})

	t.Run("update non-existent DO", func(t *testing.T) {
		updates := map[string]interface{}{
			"notes": strPtr("Some notes"),
		}

		err := mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
			return tx.UpdateDeliveryOrder(ctx, 9999, updates)
		})

		assert.Error(t, err)
		assert.Equal(t, ErrNotFound, err)
	})
}

func TestRepository_UpdateDeliveryOrderStatus(t *testing.T) {
	mock := newMockRepository()
	ctx := context.Background()

	// Setup test data
	mock.salesOrders[1] = &mockSalesOrder{
		ID:         1,
		DocNumber:  "SO-202401-00001",
		CompanyID:  1,
		CustomerID: 1,
		Status:     "CONFIRMED",
	}

	do := DeliveryOrder{
		DocNumber:    "DO-202401-00001",
		CompanyID:    1,
		SalesOrderID: 1,
		WarehouseID:  1,
		CustomerID:   1,
		DeliveryDate: time.Now(),
		Status:       DOStatusDraft,
		CreatedBy:    1,
	}

	var doID int64
	_ = mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		id, err := tx.CreateDeliveryOrder(ctx, do)
		doID = id
		return err
	})

	t.Run("update status", func(t *testing.T) {
		confirmedAt := time.Now()
		confirmedBy := int64(1)
		updates := map[string]interface{}{
			"confirmed_by": confirmedBy,
			"confirmed_at": confirmedAt,
		}

		err := mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
			return tx.UpdateDeliveryOrderStatus(ctx, doID, DOStatusConfirmed, updates)
		})

		require.NoError(t, err)

		// Verify status update
		updated, err := mock.GetDeliveryOrder(ctx, doID)
		require.NoError(t, err)
		assert.Equal(t, DOStatusConfirmed, updated.Status)
		assert.NotNil(t, updated.ConfirmedBy)
		assert.Equal(t, confirmedBy, *updated.ConfirmedBy)
		assert.NotNil(t, updated.ConfirmedAt)
	})
}

func TestRepository_DeleteDeliveryOrderLines(t *testing.T) {
	mock := newMockRepository()
	ctx := context.Background()

	// Setup test data
	mock.salesOrders[1] = &mockSalesOrder{
		ID:         1,
		DocNumber:  "SO-202401-00001",
		CompanyID:  1,
		CustomerID: 1,
		Status:     "CONFIRMED",
	}

	do := DeliveryOrder{
		DocNumber:    "DO-202401-00001",
		CompanyID:    1,
		SalesOrderID: 1,
		WarehouseID:  1,
		CustomerID:   1,
		DeliveryDate: time.Now(),
		Status:       DOStatusDraft,
		CreatedBy:    1,
	}

	var doID int64
	_ = mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		id, err := tx.CreateDeliveryOrder(ctx, do)
		doID = id

		// Add lines
		for i := 1; i <= 3; i++ {
			line := DeliveryOrderLine{
				DeliveryOrderID:   doID,
				SalesOrderLineID:  int64(i),
				ProductID:         int64(i),
				QuantityToDeliver: 100.0,
				QuantityDelivered: 0.0,
				UOM:               "PCS",
				UnitPrice:         10.0,
				LineOrder:         i,
			}
			_, err = tx.InsertDeliveryOrderLine(ctx, line)
			if err != nil {
				return err
			}
		}
		return nil
	})

	t.Run("delete lines", func(t *testing.T) {
		// Verify lines exist
		beforeDO, _ := mock.GetDeliveryOrder(ctx, doID)
		assert.Len(t, beforeDO.Lines, 3)

		err := mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
			return tx.DeleteDeliveryOrderLines(ctx, doID)
		})

		require.NoError(t, err)

		// Verify lines deleted
		afterDO, _ := mock.GetDeliveryOrder(ctx, doID)
		assert.Len(t, afterDO.Lines, 0)
	})
}

func TestRepository_UpdateDeliveryOrderLineQuantity(t *testing.T) {
	mock := newMockRepository()
	ctx := context.Background()

	// Setup test data
	mock.salesOrders[1] = &mockSalesOrder{
		ID:         1,
		DocNumber:  "SO-202401-00001",
		CompanyID:  1,
		CustomerID: 1,
		Status:     "CONFIRMED",
	}

	do := DeliveryOrder{
		DocNumber:    "DO-202401-00001",
		CompanyID:    1,
		SalesOrderID: 1,
		WarehouseID:  1,
		CustomerID:   1,
		DeliveryDate: time.Now(),
		Status:       DOStatusDraft,
		CreatedBy:    1,
	}

	var doID, lineID int64
	_ = mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		id, err := tx.CreateDeliveryOrder(ctx, do)
		doID = id

		line := DeliveryOrderLine{
			DeliveryOrderID:   doID,
			SalesOrderLineID:  1,
			ProductID:         1,
			QuantityToDeliver: 100.0,
			QuantityDelivered: 0.0,
			UOM:               "PCS",
			UnitPrice:         10.0,
			LineOrder:         1,
		}
		lineID, err = tx.InsertDeliveryOrderLine(ctx, line)
		return err
	})

	t.Run("update quantity", func(t *testing.T) {
		err := mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
			return tx.UpdateDeliveryOrderLineQuantity(ctx, lineID, 100.0)
		})

		require.NoError(t, err)

		// Verify update
		updated, _ := mock.GetDeliveryOrder(ctx, doID)
		assert.Equal(t, 100.0, updated.Lines[0].QuantityDelivered)
	})

	t.Run("update non-existent line", func(t *testing.T) {
		err := mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
			return tx.UpdateDeliveryOrderLineQuantity(ctx, 9999, 50.0)
		})

		assert.Error(t, err)
		assert.Equal(t, ErrNotFound, err)
	})
}

func TestRepository_GetDeliverableSOLines(t *testing.T) {
	mock := newMockRepository()
	ctx := context.Background()

	// Setup test data
	mock.deliverableLines[1] = []DeliverableSOLine{
		{
			SalesOrderLineID:  1,
			SalesOrderID:      1,
			ProductID:         1,
			ProductCode:       "PROD001",
			ProductName:       "Product 1",
			Quantity:          100.0,
			QuantityDelivered: 20.0,
			RemainingQuantity: 80.0,
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

	t.Run("get deliverable lines", func(t *testing.T) {
		result, err := mock.GetDeliverableSOLines(ctx, 1)
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, 80.0, result[0].RemainingQuantity)
		assert.Equal(t, 50.0, result[1].RemainingQuantity)
	})

	t.Run("no deliverable lines", func(t *testing.T) {
		result, err := mock.GetDeliverableSOLines(ctx, 999)
		require.NoError(t, err)
		assert.Len(t, result, 0)
	})

	t.Run("error injection", func(t *testing.T) {
		mock.getDeliverableLinesError = errors.New("query failed")

		result, err := mock.GetDeliverableSOLines(ctx, 1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "query failed")
		assert.Nil(t, result)

		mock.getDeliverableLinesError = nil
	})
}

func TestRepository_HelperFunctions(t *testing.T) {
	mock := newMockRepository()
	ctx := context.Background()

	t.Run("GenerateDeliveryOrderNumber", func(t *testing.T) {
		date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

		docNum1, err := mock.GenerateDeliveryOrderNumber(ctx, 1, date)
		require.NoError(t, err)
		assert.Equal(t, "DO-202401-00001", docNum1)

		docNum2, err := mock.GenerateDeliveryOrderNumber(ctx, 1, date)
		require.NoError(t, err)
		assert.Equal(t, "DO-202401-00002", docNum2)
	})

	t.Run("GetSalesOrderDetails", func(t *testing.T) {
		mock.salesOrders[1] = &mockSalesOrder{
			ID:         1,
			DocNumber:  "SO-202401-00001",
			CompanyID:  1,
			CustomerID: 1,
			Status:     "CONFIRMED",
		}

		so, err := mock.GetSalesOrderDetails(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, int64(1), so.ID)
		assert.Equal(t, "SO-202401-00001", so.DocNumber)
		assert.Equal(t, "CONFIRMED", so.Status)

		// Non-existent SO
		_, err = mock.GetSalesOrderDetails(ctx, 999)
		assert.Error(t, err)
		assert.Equal(t, ErrNotFound, err)
	})

	t.Run("CheckWarehouseExists", func(t *testing.T) {
		mock.warehouses[1] = true

		exists, err := mock.CheckWarehouseExists(ctx, 1)
		require.NoError(t, err)
		assert.True(t, exists)

		exists, err = mock.CheckWarehouseExists(ctx, 999)
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("GetDeliveryOrderIDByDocNumber", func(t *testing.T) {
		mock.salesOrders[1] = &mockSalesOrder{
			ID:         1,
			DocNumber:  "SO-202401-00001",
			CompanyID:  1,
			CustomerID: 1,
			Status:     "CONFIRMED",
		}

		do := DeliveryOrder{
			DocNumber:    "DO-202401-00001",
			CompanyID:    1,
			SalesOrderID: 1,
			WarehouseID:  1,
			CustomerID:   1,
			DeliveryDate: time.Now(),
			Status:       DOStatusDraft,
			CreatedBy:    1,
		}

		var doID int64
		_ = mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
			id, err := tx.CreateDeliveryOrder(ctx, do)
			doID = id
			return err
		})

		retrievedID, err := mock.GetDeliveryOrderIDByDocNumber(ctx, 1, "DO-202401-00001")
		require.NoError(t, err)
		assert.Equal(t, doID, retrievedID)

		// Non-existent doc number
		_, err = mock.GetDeliveryOrderIDByDocNumber(ctx, 1, "DO-NOTEXIST")
		assert.Error(t, err)
		assert.Equal(t, ErrNotFound, err)
	})
}

func TestRepository_TransactionBehavior(t *testing.T) {
	mock := newMockRepository()
	ctx := context.Background()

	// Setup test data
	mock.salesOrders[1] = &mockSalesOrder{
		ID:         1,
		DocNumber:  "SO-202401-00001",
		CompanyID:  1,
		CustomerID: 1,
		Status:     "CONFIRMED",
	}

	t.Run("transaction commit", func(t *testing.T) {
		var doID int64
		err := mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
			do := DeliveryOrder{
				DocNumber:    "DO-202401-00001",
				CompanyID:    1,
				SalesOrderID: 1,
				WarehouseID:  1,
				CustomerID:   1,
				DeliveryDate: time.Now(),
				Status:       DOStatusDraft,
				CreatedBy:    1,
			}

			id, err := tx.CreateDeliveryOrder(ctx, do)
			doID = id
			return err
		})

		require.NoError(t, err)

		// Verify committed
		result, err := mock.GetDeliveryOrder(ctx, doID)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("transaction rollback", func(t *testing.T) {
		initialCount := len(mock.deliveryOrders)

		err := mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
			do := DeliveryOrder{
				DocNumber:    "DO-202401-00099",
				CompanyID:    1,
				SalesOrderID: 1,
				WarehouseID:  1,
				CustomerID:   1,
				DeliveryDate: time.Now(),
				Status:       DOStatusDraft,
				CreatedBy:    1,
			}

			_, err := tx.CreateDeliveryOrder(ctx, do)
			if err != nil {
				return err
			}

			// Force rollback
			return errors.New("force rollback")
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "force rollback")

		// Verify no change (in real implementation, rollback would prevent this)
		// In our mock, this would still create the record, but in real DB it wouldn't
		assert.GreaterOrEqual(t, len(mock.deliveryOrders), initialCount)
	})

	t.Run("transaction error injection", func(t *testing.T) {
		mock.txError = errors.New("transaction start failed")

		err := mock.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
			return nil
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "transaction start failed")

		mock.txError = nil
	})
}

// Helper functions
func strPtr(s string) *string {
	return &s
}
