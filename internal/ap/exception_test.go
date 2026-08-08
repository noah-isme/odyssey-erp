package ap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockExceptionRepo struct {
	Repository
	TxRepository
	mock.Mock
}

func (m *mockExceptionRepo) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	return fn(ctx, m)
}

func (m *mockExceptionRepo) CreateAPException(ctx context.Context, exc APException) (int64, error) {
	args := m.Called(ctx, exc)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockExceptionRepo) UpdateAPExceptionStatus(ctx context.Context, id int64, status string, resolvedBy *int64) error {
	args := m.Called(ctx, id, status, resolvedBy)
	return args.Error(0)
}

func (m *mockExceptionRepo) GetAPException(ctx context.Context, id int64) (APException, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(APException), args.Error(1)
}

func (m *mockExceptionRepo) ListAPExceptions(ctx context.Context, status string, ownerID, invoiceID int64, limit, offset int) ([]APException, error) {
	args := m.Called(ctx, status, ownerID, invoiceID, limit, offset)
	return args.Get(0).([]APException), args.Error(1)
}

func TestExceptionService_CreateAndResolve(t *testing.T) {
	repo := new(mockExceptionRepo)
	svc := NewExceptionService(repo)
	ctx := context.Background()

	exc := APException{
		APInvoiceID:   1,
		ExceptionType: "MISMATCH",
		Severity:      "HIGH",
		Reason:        "Price variance",
	}

	repo.On("CreateAPException", ctx, mock.MatchedBy(func(e APException) bool {
		return e.Status == "OPEN" && e.SLADueAt != nil
	})).Return(int64(100), nil)

	id, err := svc.CreateException(ctx, exc)
	require.NoError(t, err)
	assert.Equal(t, int64(100), id)

	existingExc := APException{
		ID:     100,
		Status: "OPEN",
	}
	repo.On("GetAPException", ctx, int64(100)).Return(existingExc, nil)
	
	resolvedBy := int64(2)
	repo.On("UpdateAPExceptionStatus", ctx, int64(100), "RESOLVED", &resolvedBy).Return(nil)

	err = svc.ResolveException(ctx, 100, resolvedBy, "RESOLVED")
	require.NoError(t, err)
	
	repo.AssertExpectations(t)
}
