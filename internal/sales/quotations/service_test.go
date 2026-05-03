package quotations

import (
	"context"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/sales/customers"
	"github.com/stretchr/testify/require"
)

type memoryRepo struct {
	quotations map[int64]*Quotation
	nextID     int64
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		quotations: make(map[int64]*Quotation),
		nextID:     1,
	}
}

func (m *memoryRepo) WithTx(ctx context.Context, fn func(context.Context, Repository) error) error {
	return fn(ctx, m)
}

func (m *memoryRepo) Get(ctx context.Context, id int64) (*Quotation, error) {
	if q, ok := m.quotations[id]; ok {
		return q, nil
	}
	return nil, ErrNotFound
}

func (m *memoryRepo) GetByDocNumber(ctx context.Context, docNumber string) (*Quotation, error) {
	for _, q := range m.quotations {
		if q.DocNumber == docNumber {
			return q, nil
		}
	}
	return nil, ErrNotFound
}

func (m *memoryRepo) List(ctx context.Context, req ListQuotationsRequest) ([]QuotationWithDetails, int, error) {
	return nil, 0, nil
}

func (m *memoryRepo) Create(ctx context.Context, quotation Quotation) (int64, error) {
	id := m.nextID
	m.nextID++
	quotation.ID = id
	m.quotations[id] = &quotation
	return id, nil
}

func (m *memoryRepo) Update(ctx context.Context, id int64, updates map[string]interface{}) error {
	if _, ok := m.quotations[id]; !ok {
		return ErrNotFound
	}
	return nil
}

func (m *memoryRepo) InsertLine(ctx context.Context, line QuotationLine) (int64, error) {
	return 1, nil
}

func (m *memoryRepo) UpdateStatus(ctx context.Context, id int64, status QuotationStatus, userID int64, reason *string) error {
	if q, ok := m.quotations[id]; ok {
		q.Status = status
		return nil
	}
	return ErrNotFound
}

func (m *memoryRepo) DeleteLines(ctx context.Context, quotationID int64) error {
	return nil
}

func (m *memoryRepo) GenerateNumber(ctx context.Context, companyID int64, date time.Time) (string, error) {
	return "QT-001", nil
}

type mockCustomerRepo struct {
	customers.Repository
}

func (m *mockCustomerRepo) Get(ctx context.Context, id int64) (*customers.Customer, error) {
	return &customers.Customer{ID: id, Name: "Test Customer"}, nil
}

func TestCreateQuotation(t *testing.T) {
	repo := newMemoryRepo()
	custRepo := &mockCustomerRepo{}
	svc := NewService(repo, custRepo)

	req := CreateQuotationRequest{
		CompanyID:  1,
		CustomerID: 1,
		QuoteDate:  time.Now(),
		ValidUntil: time.Now().AddDate(0, 1, 0),
		Lines: []CreateQuotationLineReq{
			{ProductID: 1, Quantity: 10, UnitPrice: 100, UOM: "PCS"},
		},
	}

	qt, err := svc.Create(context.Background(), req, 1)
	require.NoError(t, err)
	require.NotNil(t, qt)
	require.Equal(t, "QT-001", qt.DocNumber)
	require.Equal(t, QuotationStatusDraft, qt.Status)
}

func TestSubmitQuotation(t *testing.T) {
	repo := newMemoryRepo()
	custRepo := &mockCustomerRepo{}
	svc := NewService(repo, custRepo)

	repo.quotations[1] = &Quotation{
		ID:        1,
		DocNumber: "QT-001",
		Status:    QuotationStatusDraft,
	}

	qt, err := svc.Submit(context.Background(), 1, 1)
	require.NoError(t, err)

	updated, _ := repo.Get(context.Background(), 1)
	require.Equal(t, QuotationStatusSubmitted, updated.Status)
	require.Equal(t, qt.Status, updated.Status)
}
