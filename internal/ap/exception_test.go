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

func (m *mockExceptionRepo) UpdateAPExceptionStatusForCompany(ctx context.Context, companyID, id int64, status string, resolvedBy *int64) error {
	args := m.Called(ctx, companyID, id, status, resolvedBy)
	return args.Error(0)
}

func (m *mockExceptionRepo) ResolveAPException(ctx context.Context, id int64, status string, resolvedBy int64, comment string) error {
	args := m.Called(ctx, id, status, resolvedBy, comment)
	return args.Error(0)
}

func (m *mockExceptionRepo) ResolveAPExceptionForCompany(ctx context.Context, companyID, id int64, status string, resolvedBy int64, comment string) error {
	args := m.Called(ctx, companyID, id, status, resolvedBy, comment)
	return args.Error(0)
}

func (m *mockExceptionRepo) ListAPExceptionResolutionEventsForCompany(ctx context.Context, companyID, exceptionID int64) ([]APExceptionResolutionEvent, error) {
	args := m.Called(ctx, companyID, exceptionID)
	return args.Get(0).([]APExceptionResolutionEvent), args.Error(1)
}

func (m *mockExceptionRepo) GetAPException(ctx context.Context, id int64) (APException, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(APException), args.Error(1)
}

func (m *mockExceptionRepo) GetAPExceptionForCompany(ctx context.Context, companyID, id int64) (APException, error) {
	args := m.Called(ctx, companyID, id)
	return args.Get(0).(APException), args.Error(1)
}

func (m *mockExceptionRepo) ListAPExceptions(ctx context.Context, status string, ownerID, invoiceID int64, limit, offset int) ([]APException, error) {
	args := m.Called(ctx, status, ownerID, invoiceID, limit, offset)
	return args.Get(0).([]APException), args.Error(1)
}

func (m *mockExceptionRepo) ListAPExceptionsForCompany(ctx context.Context, companyID int64, status string, ownerID, invoiceID int64, limit, offset int) ([]APException, error) {
	args := m.Called(ctx, companyID, status, ownerID, invoiceID, limit, offset)
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
	repo.On("ResolveAPException", ctx, int64(100), "RESOLVED", resolvedBy, "").Return(nil)

	err = svc.ResolveException(ctx, 100, resolvedBy, "RESOLVED")
	require.NoError(t, err)

	repo.AssertExpectations(t)
}

func TestExceptionService_ResolveForCompanyUsesScopedRepositoryMethods(t *testing.T) {
	repo := new(mockExceptionRepo)
	svc := NewExceptionService(repo)
	ctx := context.Background()
	resolvedBy := int64(2)
	existing := APException{ID: 100, APInvoiceID: 11, Status: "OPEN"}

	repo.On("GetAPExceptionForCompany", ctx, int64(7), int64(100)).Return(existing, nil)
	repo.On("ResolveAPExceptionForCompany", ctx, int64(7), int64(100), "REJECTED", resolvedBy, "").Return(nil)

	require.NoError(t, svc.ResolveExceptionForCompany(ctx, 7, 100, resolvedBy, "rejected"))
	repo.AssertExpectations(t)
}

func TestExceptionService_ResolveWithCommentAppendsImmutableEvent(t *testing.T) {
	repo := new(mockExceptionRepo)
	svc := NewExceptionService(repo)
	ctx := context.Background()
	existing := APException{ID: 100, Status: "IN_REVIEW"}
	resolvedBy := int64(2)

	repo.On("GetAPException", ctx, int64(100)).Return(existing, nil)
	repo.On("ResolveAPException", ctx, int64(100), "RESOLVED", resolvedBy, "operator confirmed quantity").Return(nil)

	require.NoError(t, svc.ResolveExceptionWithComment(ctx, 100, resolvedBy, "resolved", "  operator confirmed quantity  "))
	repo.AssertExpectations(t)
}

func TestExceptionService_ResolveForCompanyWithCommentScopesAndAppendsEvent(t *testing.T) {
	repo := new(mockExceptionRepo)
	svc := NewExceptionService(repo)
	ctx := context.Background()
	resolvedBy := int64(2)
	existing := APException{ID: 100, Status: "OPEN"}

	repo.On("GetAPExceptionForCompany", ctx, int64(7), int64(100)).Return(existing, nil)
	repo.On("ResolveAPExceptionForCompany", ctx, int64(7), int64(100), "REJECTED", resolvedBy, "supplier declined correction").Return(nil)

	require.NoError(t, svc.ResolveExceptionForCompanyWithComment(ctx, 7, 100, resolvedBy, "rejected", " supplier declined correction "))
	repo.AssertExpectations(t)
}

func TestExceptionService_RejectsResolverlessResolution(t *testing.T) {
	svc := NewExceptionService(new(mockExceptionRepo))
	require.ErrorIs(t, svc.ResolveException(context.Background(), 100, 0, "RESOLVED"), ErrExceptionResolverRequired)
}

func TestExceptionService_RejectsAlreadyClosedException(t *testing.T) {
	repo := new(mockExceptionRepo)
	svc := NewExceptionService(repo)
	ctx := context.Background()
	repo.On("GetAPException", ctx, int64(100)).Return(APException{ID: 100, Status: "RESOLVED"}, nil)

	require.ErrorIs(t, svc.ResolveException(ctx, 100, 2, "REJECTED"), ErrExceptionAlreadyClosed)
	repo.AssertNotCalled(t, "ResolveAPException", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestExceptionService_ListsScopedResolutionEvents(t *testing.T) {
	repo := new(mockExceptionRepo)
	svc := NewExceptionService(repo)
	ctx := context.Background()
	want := []APExceptionResolutionEvent{{ID: 501, APExceptionID: 100, CompanyID: 7, ToStatus: "RESOLVED", Comment: "done"}}
	repo.On("ListAPExceptionResolutionEventsForCompany", ctx, int64(7), int64(100)).Return(want, nil)

	events, err := svc.ListResolutionEventsForCompany(ctx, 7, 100)
	require.NoError(t, err)
	require.Equal(t, want, events)
	repo.AssertExpectations(t)
}

func TestExceptionService_RequiresCompanyForScopedReads(t *testing.T) {
	svc := NewExceptionService(new(mockExceptionRepo))

	_, err := svc.GetExceptionForCompany(context.Background(), 0, 100)
	require.ErrorIs(t, err, ErrExceptionCompanyScopeRequired)
	_, err = svc.ListExceptionsForCompany(context.Background(), 0, "", 0, 0, 100, 0)
	require.ErrorIs(t, err, ErrExceptionCompanyScopeRequired)
	require.ErrorIs(t, svc.ResolveExceptionForCompany(context.Background(), 0, 100, 2, "RESOLVED"), ErrExceptionCompanyScopeRequired)
}
