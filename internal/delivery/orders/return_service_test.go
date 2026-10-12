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

// WarehouseBelongsToCompany: warehouses 1-99 belong to company 1; 0 and others to
// no company the tests use.
func (r *returnRepoFake) WarehouseBelongsToCompany(_ context.Context, warehouseID, companyID int64) (bool, error) {
	return companyID == 1 && warehouseID > 0 && warehouseID < 100, nil
}
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
	repo := &returnRepoFake{returned: &ReturnDeliveryOrder{ID: 1, CompanyID: 1, Number: "RDO-1", WarehouseID: 4, Status: ReturnStatusDraft, Lines: []ReturnLine{{ProductID: 9, QuantityReturned: 2, UnitPrice: 30, RestockWarehouseID: &warehouseID}}}}
	stock := &inventoryFake{}
	service := NewService(repo)
	service.SetInventory(stock)

	returned, err := service.ConfirmReturnDeliveryOrder(context.Background(), 1, 1, 3)
	require.NoError(t, err)
	require.Equal(t, ReturnStatusConfirmed, returned.Status)
	require.Len(t, stock.restocked, 1)
	require.Equal(t, int64(8), stock.restocked[0].WarehouseID)
	require.Equal(t, 2.0, stock.restocked[0].Quantity)
}

func TestConfirmReturnReversesOnCancel(t *testing.T) {
	repo := &returnRepoFake{returned: &ReturnDeliveryOrder{ID: 1, CompanyID: 1, Number: "RDO-2", WarehouseID: 4, Status: ReturnStatusConfirmed, Lines: []ReturnLine{{ProductID: 9, QuantityReturned: 2, UnitPrice: 30}}}}
	stock := &inventoryFake{}
	service := NewService(repo)
	service.SetInventory(stock)

	returned, err := service.CancelReturnDeliveryOrder(context.Background(), 1, 1, 3, "customer rejected")
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
	repo := &returnRepoFake{returned: &ReturnDeliveryOrder{ID: 1, CompanyID: 1, Number: "RDO-3", WarehouseID: 4, Status: ReturnStatusCancelled, Lines: []ReturnLine{{ProductID: 9, QuantityReturned: 2, UnitPrice: 30}}}}
	service := NewService(repo)
	_, err := service.ConfirmReturnDeliveryOrder(context.Background(), 1, 1, 3)
	require.Error(t, err)
	require.ErrorContains(t, err, "cannot confirm return delivery order")
}

// TestReturnActionsHideOtherCompany: confirming or cancelling a return of
// another company, or with no company, answers ErrNotFound and moves no
// stock (FIND-036).
func TestReturnActionsHideOtherCompany(t *testing.T) {
	for _, companyID := range []int64{2, 0} {
		repo := &returnRepoFake{returned: &ReturnDeliveryOrder{ID: 1, CompanyID: 1, Number: "RDO-4", WarehouseID: 4, Status: ReturnStatusConfirmed, Lines: []ReturnLine{{ProductID: 9, QuantityReturned: 2, UnitPrice: 30}}}}
		stock := &inventoryFake{}
		service := NewService(repo)
		service.SetInventory(stock)

		_, err := service.CancelReturnDeliveryOrder(context.Background(), 1, companyID, 3, "reason")
		require.ErrorIs(t, err, ErrNotFound, "company %d", companyID)
		repo.returned.Status = ReturnStatusDraft
		_, err = service.ConfirmReturnDeliveryOrder(context.Background(), 1, companyID, 3)
		require.ErrorIs(t, err, ErrNotFound, "company %d", companyID)
		_, err = service.GetReturnByID(context.Background(), 1, companyID)
		require.ErrorIs(t, err, ErrNotFound, "company %d", companyID)
		require.Empty(t, stock.restocked, "company %d", companyID)
	}
}

// TestOrderActionsHideOtherCompany: every by-id delivery order action of
// another company, or with no company, answers ErrNotFound and moves no
// stock (FIND-036).
func TestOrderActionsHideOtherCompany(t *testing.T) {
	for _, companyID := range []int64{2, 0} {
		order := &DeliveryOrder{ID: 10, CompanyID: 1, Status: StatusDraft, WarehouseID: 4, Lines: []Line{{ID: 7, ProductID: 9, QuantityToDeliver: 1}}}
		repo := &returnRepoFake{delivery: order}
		stock := &inventoryFake{}
		service := NewService(repo)
		service.SetInventory(stock)
		ctx := context.Background()

		_, err := service.GetByID(ctx, 10, companyID)
		require.ErrorIs(t, err, ErrNotFound)
		_, err = service.Update(ctx, 10, companyID, UpdateRequest{})
		require.ErrorIs(t, err, ErrNotFound)
		_, err = service.Confirm(ctx, 10, companyID, 3)
		require.ErrorIs(t, err, ErrNotFound)
		order.Status = StatusConfirmed
		_, err = service.MarkInTransit(ctx, 10, companyID, MarkInTransitRequest{})
		require.ErrorIs(t, err, ErrNotFound)
		_, err = service.Cancel(ctx, 10, companyID, CancelRequest{Reason: "x"})
		require.ErrorIs(t, err, ErrNotFound)
		order.Status = StatusInTransit
		_, err = service.MarkDelivered(ctx, 10, companyID, MarkDeliveredRequest{})
		require.ErrorIs(t, err, ErrNotFound)
		order.Status = StatusDelivered
		_, err = service.CreateReturnDeliveryOrder(ctx, CreateReturnRequest{CompanyID: companyID, OriginalDeliveryOrderID: 10}, 3)
		require.ErrorIs(t, err, ErrNotFound)
		require.Empty(t, stock.restocked)
	}
}

// TestReturnRefusesRestockWarehouseOfAnotherCompany: creating or confirming
// a return whose restock warehouse is not the company's is refused and
// moves no stock.
func TestReturnRefusesRestockWarehouseOfAnotherCompany(t *testing.T) {
	foreign := int64(500)
	repo := &returnRepoFake{delivery: &DeliveryOrder{ID: 10, CompanyID: 1, CustomerID: 2, Status: StatusDelivered, WarehouseID: 4, Lines: []Line{{ID: 7, ProductID: 9, QuantityDelivered: 5}}}}
	service := NewService(repo)
	_, err := service.CreateReturnDeliveryOrder(context.Background(), CreateReturnRequest{CompanyID: 1, OriginalDeliveryOrderID: 10, ReturnDate: time.Now(),
		Lines: []CreateReturnLineReq{{DeliveryOrderLineID: 7, ProductID: 9, QuantityReturned: 1, RestockWarehouseID: &foreign}}}, 3)
	require.ErrorIs(t, err, ErrRestockWarehouse)

	repo.returned = &ReturnDeliveryOrder{ID: 1, CompanyID: 1, Number: "RDO-5", WarehouseID: 4, Status: ReturnStatusDraft, Lines: []ReturnLine{{ProductID: 9, QuantityReturned: 1, RestockWarehouseID: &foreign}}}
	stock := &inventoryFake{}
	service.SetInventory(stock)
	_, err = service.ConfirmReturnDeliveryOrder(context.Background(), 1, 1, 3)
	require.ErrorIs(t, err, ErrRestockWarehouse)
	require.Empty(t, stock.restocked)
}

// TestReturnRefusesFallbackWarehouseOfAnotherCompany: a return without a
// restock warehouse falls back to the delivery's warehouse; when that is not
// a warehouse of the return's company, create and confirm are refused and
// no stock moves.
func TestReturnRefusesFallbackWarehouseOfAnotherCompany(t *testing.T) {
	zero := int64(0)
	repo := &returnRepoFake{delivery: &DeliveryOrder{ID: 10, CompanyID: 1, CustomerID: 2, Status: StatusDelivered, WarehouseID: 500, Lines: []Line{{ID: 7, ProductID: 9, QuantityDelivered: 5}}}}
	service := NewService(repo)
	_, err := service.CreateReturnDeliveryOrder(context.Background(), CreateReturnRequest{CompanyID: 1, OriginalDeliveryOrderID: 10, ReturnDate: time.Now(),
		Lines: []CreateReturnLineReq{{DeliveryOrderLineID: 7, ProductID: 9, QuantityReturned: 1, RestockWarehouseID: &zero}}}, 3)
	require.ErrorIs(t, err, ErrRestockWarehouse)

	repo.returned = &ReturnDeliveryOrder{ID: 1, CompanyID: 1, Number: "RDO-6", WarehouseID: 500, Status: ReturnStatusDraft, Lines: []ReturnLine{{ProductID: 9, QuantityReturned: 1}}}
	stock := &inventoryFake{}
	service.SetInventory(stock)
	_, err = service.ConfirmReturnDeliveryOrder(context.Background(), 1, 1, 3)
	require.ErrorIs(t, err, ErrRestockWarehouse)
	require.Empty(t, stock.restocked)
}

// TestCreateRefusesWarehouseOutsideCompany: a delivery order may only ship
// from a warehouse of the order's company; a missing and a foreign warehouse
// get the same error.
func TestCreateRefusesWarehouseOutsideCompany(t *testing.T) {
	for _, warehouseID := range []int64{500, 0} {
		repo := &createCountingRepo{fulfillmentWarehouseRepoFake: fulfillmentWarehouseRepoFake{
			returnRepoFake: returnRepoFake{delivery: &DeliveryOrder{ID: 99, CompanyID: 1}},
			salesOrder:     &SalesOrderInfo{ID: 1, CompanyID: 1, CustomerID: 7, Status: "CONFIRMED"},
			lines:          []DeliverableSOLine{{SalesOrderLineID: 1, ProductID: 3, Quantity: 2, RemainingQuantity: 2, FulfillmentWarehouseID: &warehouseID}},
		}}
		_, err := NewService(repo).Create(context.Background(), CreateRequest{CompanyID: 1, SalesOrderID: 1, WarehouseID: warehouseID, DeliveryDate: time.Now(),
			Lines: []CreateLineReq{{SalesOrderLineID: 1, ProductID: 3, QuantityToDeliver: 1}}}, 7)
		require.ErrorIs(t, err, ErrWarehouseNotFound, "warehouse %d", warehouseID)
		require.Zero(t, repo.created, "warehouse %d", warehouseID)
	}
}
