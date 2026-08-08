package ap

import (
	"context"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/jackc/pgx/v5/pgtype"
)

type mockRepo struct {
	Repository
	TxRepository
	mock.Mock
}

func (m *mockRepo) GetAPInvoiceWithDetails(ctx context.Context, id int64) (APInvoiceWithDetails, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(APInvoiceWithDetails), args.Error(1)
}

func (m *mockRepo) GetActiveMatchingPolicy(ctx context.Context, companyID, supplierID, categoryID *int64) (*MatchingPolicy, error) {
	args := m.Called(ctx, companyID, supplierID, categoryID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*MatchingPolicy), args.Error(1)
}

func (m *mockRepo) GetPOLineProgressByPO(ctx context.Context, poID int64) (map[int64]*sqlc.PoLineProgress, error) {
	args := m.Called(ctx, poID)
	return args.Get(0).(map[int64]*sqlc.PoLineProgress), args.Error(1)
}

func (m *mockRepo) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	return fn(ctx, m)
}

func (m *mockRepo) CreateMatchingRun(ctx context.Context, run MatchingRun) (int64, error) {
	args := m.Called(ctx, run)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockRepo) CreateMatchingRunLine(ctx context.Context, line MatchingRunLine) error {
	args := m.Called(ctx, line)
	return args.Error(0)
}

func TestMatchingService_RunMatch_ExactMatch(t *testing.T) {
	repo := new(mockRepo)
	svc := NewMatchingService(repo)

	ctx := context.Background()
	poID := int64(100)
	poLineID := int64(200)

	invoice := APInvoiceWithDetails{
		APInvoice: APInvoice{
			ID:         1,
			SupplierID: 10,
			POID:       &poID,
			Total:      1000,
		},
		Lines: []APInvoiceLine{
			{
				ID:           1,
				POLineID:     &poLineID,
				Quantity:     10,
				UnitPrice:    100,
			},
		},
	}

	policy := &MatchingPolicy{
		ID:                1,
		QtyTolerancePct:   0,
		PriceTolerancePct: 0,
		TotalToleranceAmt: 0,
	}

	progress := map[int64]*sqlc.PoLineProgress{
		poLineID: {
			PoLineID:   poLineID,
		},
	}
	
	n10 := pgtype.Numeric{}
	n10.Scan("10")
	n100 := pgtype.Numeric{}
	n100.Scan("100")
	progress[poLineID].OrderedQty = n10
	progress[poLineID].UnitPrice = n100

	repo.On("GetAPInvoiceWithDetails", ctx, int64(1)).Return(invoice, nil)
	repo.On("GetActiveMatchingPolicy", ctx, mock.Anything, mock.Anything, mock.Anything).Return(policy, nil)
	repo.On("GetPOLineProgressByPO", ctx, poID).Return(progress, nil)
	repo.On("CreateMatchingRun", ctx, mock.Anything).Return(int64(1), nil)
	repo.On("CreateMatchingRunLine", ctx, mock.Anything).Return(nil)

	run, err := svc.RunMatch(ctx, 1, 1)
	require.NoError(t, err)

	assert.Equal(t, "MATCHED", run.Status)
	assert.Equal(t, "AUTO_POST", run.ActionRecommended)
	assert.Len(t, run.Lines, 1)
	assert.Equal(t, "MATCHED", run.Lines[0].Status)
	assert.Equal(t, float64(10), *run.Lines[0].POQty)
}
