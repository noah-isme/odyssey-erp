package orders

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type returnRepoFake struct {
	delivery         *DeliveryOrder
	returnedQuantity float64
	hasCreditNote    bool
	returned         *ReturnDeliveryOrder
}

func (r *returnRepoFake) GetByID(context.Context, int64) (*DeliveryOrder, error) {
	return r.delivery, nil
}
func (r *returnRepoFake) GetByDocNumber(context.Context, int64, string) (*DeliveryOrder, error) {
	return nil, nil
}
func (r *returnRepoFake) GetWithDetails(context.Context, int64) (*WithDetails, error) {
	return nil, nil
}
func (r *returnRepoFake) GetLinesWithDetails(context.Context, int64) ([]LineWithDetails, error) {
	return nil, nil
}
func (r *returnRepoFake) List(context.Context, ListRequest) ([]WithDetails, int, error) {
	return nil, 0, nil
}
func (r *returnRepoFake) GetDeliverableSOLines(context.Context, int64) ([]DeliverableSOLine, error) {
	return nil, nil
}
func (r *returnRepoFake) GetReturnByID(context.Context, int64) (*ReturnDeliveryOrder, error) {
	return r.returned, nil
}
func (r *returnRepoFake) ListReturns(context.Context, ListReturnRequest) ([]ReturnDeliveryOrderWithDetails, int, error) {
	return nil, 0, nil
}
func (r *returnRepoFake) GetReturnedQuantity(context.Context, int64) (float64, error) {
	return r.returnedQuantity, nil
}
func (r *returnRepoFake) HasCreditNoteForReturn(context.Context, int64) (bool, error) {
	return r.hasCreditNote, nil
}
func (r *returnRepoFake) GenerateDocNumber(context.Context, int64, time.Time) (string, error) {
	return "DO", nil
}
func (r *returnRepoFake) GetSalesOrderDetails(context.Context, int64) (*SalesOrderInfo, error) {
	return nil, nil
}
func (r *returnRepoFake) CheckWarehouseExists(context.Context, int64) (bool, error) { return true, nil }
func (r *returnRepoFake) GenerateReturnDocNumber(context.Context, int64, time.Time) (string, error) {
	return "RDO-1", nil
}
func (r *returnRepoFake) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	return fn(ctx, &returnTxFake{repo: r})
}

type returnTxFake struct{ repo *returnRepoFake }

func (*returnTxFake) CreateDeliveryOrder(context.Context, DeliveryOrder) (int64, error) {
	return 0, nil
}
func (*returnTxFake) InsertLine(context.Context, Line) (int64, error) { return 0, nil }
func (*returnTxFake) UpdateDeliveryOrder(context.Context, int64, map[string]interface{}) error {
	return nil
}
func (*returnTxFake) UpdateStatus(context.Context, int64, Status, map[string]interface{}) error {
	return nil
}
func (*returnTxFake) DeleteLines(context.Context, int64) error                 { return nil }
func (*returnTxFake) UpdateLineQuantity(context.Context, int64, float64) error { return nil }
func (t *returnTxFake) CreateReturnDeliveryOrder(_ context.Context, returned ReturnDeliveryOrder) (int64, error) {
	returned.ID = 1
	t.repo.returned = &returned
	return 1, nil
}
func (t *returnTxFake) InsertReturnLine(_ context.Context, line ReturnLine) (int64, error) {
	line.ID = 1
	t.repo.returned.Lines = append(t.repo.returned.Lines, line)
	return 1, nil
}
func (t *returnTxFake) UpdateReturnStatus(_ context.Context, _ int64, status ReturnStatus, _ map[string]interface{}) error {
	t.repo.returned.Status = status
	return nil
}
func (*returnTxFake) DeleteReturnLines(context.Context, int64) error { return nil }

type inventoryFake struct{ restocked []InventoryItem }

func (f *inventoryFake) Reduce(context.Context, []InventoryItem) error { return nil }
func (f *inventoryFake) Restock(_ context.Context, items []InventoryItem) error {
	f.restocked = append(f.restocked, items...)
	return nil
}
func (f *inventoryFake) CheckAvailability(context.Context, int64, int64) (float64, error) {
	return 0, nil
}

func TestCreateReturnRejectsCumulativeQuantityAboveDelivered(t *testing.T) {
	repo := &returnRepoFake{returnedQuantity: 4, delivery: &DeliveryOrder{ID: 10, CompanyID: 1, CustomerID: 2, Status: StatusDelivered, Lines: []Line{{ID: 7, ProductID: 9, QuantityDelivered: 5}}}}
	service := NewService(repo)

	_, err := service.CreateReturnDeliveryOrder(context.Background(), CreateReturnRequest{CompanyID: 1, OriginalDeliveryOrderID: 10, ReturnDate: time.Now(), Lines: []CreateReturnLineReq{{DeliveryOrderLineID: 7, ProductID: 9, QuantityReturned: 2}}}, 3)
	require.ErrorContains(t, err, "cumulative returned quantity")
}

func TestConfirmReturnRestocksSelectedWarehouse(t *testing.T) {
	warehouseID := int64(8)
	repo := &returnRepoFake{returned: &ReturnDeliveryOrder{ID: 1, Number: "RDO-1", WarehouseID: 4, Status: ReturnStatusDraft, Lines: []ReturnLine{{ProductID: 9, QuantityReturned: 2, UnitPrice: 30, RestockWarehouseID: &warehouseID}}}}
	stock := &inventoryFake{}
	service := NewService(repo)
	service.SetInventory(stock)

	returned, err := service.ConfirmReturnDeliveryOrder(context.Background(), 1, 3)
	require.NoError(t, err)
	require.Equal(t, ReturnStatusConfirmed, returned.Status)
	require.Len(t, stock.restocked, 1)
	require.Equal(t, int64(8), stock.restocked[0].WarehouseID)
	require.Equal(t, 2.0, stock.restocked[0].Quantity)
}

func TestConfirmReturnReversesOnCancel(t *testing.T) {
	repo := &returnRepoFake{returned: &ReturnDeliveryOrder{ID: 1, Number: "RDO-2", WarehouseID: 4, Status: ReturnStatusConfirmed, Lines: []ReturnLine{{ProductID: 9, QuantityReturned: 2, UnitPrice: 30}}}}
	stock := &inventoryFake{}
	service := NewService(repo)
	service.SetInventory(stock)

	returned, err := service.CancelReturnDeliveryOrder(context.Background(), 1, 3, "customer rejected")
	require.NoError(t, err)
	require.Equal(t, ReturnStatusCancelled, returned.Status)
	require.Len(t, stock.restocked, 1)
	require.Equal(t, -2.0, stock.restocked[0].Quantity)
	require.Equal(t, "RDO-RDO-2-X", stock.restocked[0].Code)
}

func TestCreateReturnRejectsDuplicateLinesBeyondDelivered(t *testing.T) {
	repo := &returnRepoFake{
		delivery: &DeliveryOrder{
			ID:          10,
			CompanyID:   1,
			CustomerID:  2,
			Status:      StatusDelivered,
			WarehouseID: 4,
			Lines:       []Line{{ID: 7, ProductID: 9, QuantityDelivered: 5}},
		},
	}
	service := NewService(repo)

	_, err := service.CreateReturnDeliveryOrder(context.Background(), CreateReturnRequest{
		CompanyID:               1,
		OriginalDeliveryOrderID: 10,
		ReturnDate:              time.Now(),
		Lines: []CreateReturnLineReq{
			{DeliveryOrderLineID: 7, ProductID: 9, QuantityReturned: 3},
			{DeliveryOrderLineID: 7, ProductID: 9, QuantityReturned: 3},
		},
	}, 3)
	require.ErrorContains(t, err, "quantity returned exceeds delivered quantity")
}

func TestConfirmReturnRejectsInvalidStatus(t *testing.T) {
	repo := &returnRepoFake{returned: &ReturnDeliveryOrder{ID: 1, Number: "RDO-3", WarehouseID: 4, Status: ReturnStatusCancelled, Lines: []ReturnLine{{ProductID: 9, QuantityReturned: 2, UnitPrice: 30}}}}
	service := NewService(repo)
	_, err := service.ConfirmReturnDeliveryOrder(context.Background(), 1, 3)
	require.Error(t, err)
	require.ErrorContains(t, err, "cannot confirm return delivery order")
}
