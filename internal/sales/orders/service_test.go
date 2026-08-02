package orders

import (
	"context"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/sales/customers"
	"github.com/odyssey-erp/odyssey-erp/internal/sales/quotations"
	"github.com/stretchr/testify/require"
)

type memoryRepo struct {
	orders map[int64]*SalesOrder
	nextID int64
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		orders: make(map[int64]*SalesOrder),
		nextID: 1,
	}
}

func (m *memoryRepo) WithTx(ctx context.Context, fn func(context.Context, Repository) error) error {
	return fn(ctx, m)
}

func (m *memoryRepo) Get(ctx context.Context, id int64) (*SalesOrder, error) {
	if order, ok := m.orders[id]; ok {
		return order, nil
	}
	return nil, ErrNotFound
}

func (m *memoryRepo) GetByDocNumber(ctx context.Context, docNumber string) (*SalesOrder, error) {
	for _, o := range m.orders {
		if o.DocNumber == docNumber {
			return o, nil
		}
	}
	return nil, ErrNotFound
}

func (m *memoryRepo) List(ctx context.Context, req ListSalesOrdersRequest) ([]SalesOrderWithDetails, int, error) {
	return nil, 0, nil
}

func (m *memoryRepo) Create(ctx context.Context, order SalesOrder) (int64, error) {
	id := m.nextID
	m.nextID++
	order.ID = id
	m.orders[id] = &order
	return id, nil
}

func (m *memoryRepo) Update(ctx context.Context, id int64, updates map[string]interface{}) error {
	if _, ok := m.orders[id]; !ok {
		return ErrNotFound
	}
	return nil
}

func (m *memoryRepo) InsertLine(ctx context.Context, line SalesOrderLine) (int64, error) {
	return 1, nil
}

func (m *memoryRepo) UpdateStatus(ctx context.Context, id int64, status SalesOrderStatus, userID int64, reason *string) error {
	if o, ok := m.orders[id]; ok {
		o.Status = status
		return nil
	}
	return ErrNotFound
}

func (m *memoryRepo) UpdateQuotationStatus(ctx context.Context, quotationID int64, status quotations.QuotationStatus) error {
	return nil
}

func (m *memoryRepo) DeleteLines(ctx context.Context, orderID int64) error {
	return nil
}

func (m *memoryRepo) GenerateNumber(ctx context.Context, companyID int64, date time.Time) (string, error) {
	return "SO-001", nil
}

type mockCustomerRepo struct {
	customers.Repository
}

func (m *mockCustomerRepo) Get(ctx context.Context, id int64) (*customers.Customer, error) {
	return &customers.Customer{ID: id, Name: "Test Customer"}, nil
}

type mockQuoteRepo struct {
	quotations.Repository
}

func TestCreateSalesOrder(t *testing.T) {
	repo := newMemoryRepo()
	custRepo := &mockCustomerRepo{}
	quoteRepo := &mockQuoteRepo{}
	svc := NewService(repo, custRepo, quoteRepo)

	req := CreateSalesOrderRequest{
		CompanyID:  1,
		CustomerID: 1,
		OrderDate:  time.Now(),
		Lines: []CreateSalesOrderLineReq{
			{ProductID: 1, FulfillmentWarehouseID: 2, Quantity: 10, UnitPrice: 100, UOM: "PCS"},
		},
	}

	order, err := svc.Create(context.Background(), req, 1)
	require.NoError(t, err)
	require.NotNil(t, order)
	require.Equal(t, "SO-001", order.DocNumber)
	require.Equal(t, SalesOrderStatusDraft, order.Status)
}

func TestCreateSalesOrderRequiresLineFulfillmentWarehouse(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(repo, &mockCustomerRepo{}, &mockQuoteRepo{})

	_, err := svc.Create(context.Background(), CreateSalesOrderRequest{
		CompanyID: 1, CustomerID: 1, OrderDate: time.Now(),
		Lines: []CreateSalesOrderLineReq{{ProductID: 1, Quantity: 1, UnitPrice: 10, UOM: "PCS"}},
	}, 1)
	require.ErrorContains(t, err, "fulfillment warehouse")
}

func TestConfirmSalesOrder(t *testing.T) {
	repo := newMemoryRepo()
	custRepo := &mockCustomerRepo{}
	quoteRepo := &mockQuoteRepo{}
	svc := NewService(repo, custRepo, quoteRepo)

	repo.orders[1] = &SalesOrder{
		ID:        1,
		DocNumber: "SO-001",
		Status:    SalesOrderStatusDraft,
		Lines: []SalesOrderLine{
			{ID: 1, ProductID: 1, Quantity: 10, UnitPrice: 100},
		},
	}

	order, err := svc.Confirm(context.Background(), 1, 1)
	require.NoError(t, err)

	updated, _ := repo.Get(context.Background(), 1)
	require.Equal(t, SalesOrderStatusConfirmed, updated.Status)
	require.Equal(t, order.Status, updated.Status)
}
