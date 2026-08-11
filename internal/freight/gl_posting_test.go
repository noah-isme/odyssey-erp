package freight

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
	accountingshared "github.com/odyssey-erp/odyssey-erp/internal/accounting/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/fx"
	"github.com/stretchr/testify/require"
)

type journalPosterFake struct {
	inputs      []journals.PostingInput
	entry       journals.JournalEntry
	err         error
	duplicateOn int
}

func (f *journalPosterFake) PostJournal(_ context.Context, input journals.PostingInput) (journals.JournalEntry, error) {
	f.inputs = append(f.inputs, input)
	if f.duplicateOn > 0 && len(f.inputs) == f.duplicateOn {
		return journals.JournalEntry{}, accountingshared.ErrSourceAlreadyLinked
	}
	if f.err != nil {
		return journals.JournalEntry{}, f.err
	}
	entry := f.entry
	if entry.SourceID == uuid.Nil {
		entry.SourceID = input.SourceID
	}
	if entry.SourceModule == "" {
		entry.SourceModule = input.SourceModule
	}
	return entry, nil
}

func freightTestCharge(repo *MockRepository, chargeID, companyID int64) {
	amount := accountingmoney.Must("123.4500", 4)
	repo.Charges[chargeID] = &FreightCharge{
		ID:           chargeID,
		CompanyID:    companyID,
		FreightTotal: amount,
		CreatedBy:    17,
	}
}

func freightTestInput() PostFreightToGLInput {
	return PostFreightToGLInput{
		CompanyID:       7,
		FreightChargeID: 11,
		PeriodID:        3,
		CostCenterID:    29,
		GLAccount:       "6100",
		FreightAmount:   accountingmoney.Must("123.4500", 4),
		Description:     "Carrier freight",
		PostedBy:        99,
	}
}

func TestPostFreightToGLPostsBalancedExactJournalWithDimensions(t *testing.T) {
	repo := NewMockRepository()
	freightTestCharge(repo, 11, 7)
	poster := &journalPosterFake{entry: journals.JournalEntry{ID: 42}}
	service := NewGLPostingService(repo, poster)

	postingID, err := service.PostFreightToGL(context.Background(), freightTestInput())

	require.NoError(t, err)
	require.Equal(t, int64(42), postingID)
	require.Len(t, poster.inputs, 1)
	posting := poster.inputs[0]
	require.Equal(t, int64(3), posting.PeriodID)
	require.Equal(t, int64(99), posting.PostedBy)
	require.Equal(t, "FREIGHT.CHARGE", posting.SourceModule)
	require.NotEqual(t, uuid.Nil, posting.SourceID)
	require.Len(t, posting.Lines, 2)

	debit, credit := posting.Lines[0], posting.Lines[1]
	require.Equal(t, int64(6100), debit.AccountID)
	require.Equal(t, int64(2100), credit.AccountID)
	require.Equal(t, int64(7), *debit.CompanyID)
	require.Equal(t, int64(7), *credit.CompanyID)
	require.Equal(t, int64(29), *debit.CostCenterID)
	require.Nil(t, credit.CostCenterID)
	require.Equal(t, 0.0, debit.Debit)
	require.Equal(t, 0.0, credit.Credit)
	require.Zero(t, debit.DebitDecimal.Cmp(fx.MustDecimal("123.4500")))
	require.Zero(t, credit.CreditDecimal.Cmp(fx.MustDecimal("123.4500")))
	require.Zero(t, debit.CreditDecimal.Cmp(fx.MustDecimal("0")))
	require.Zero(t, credit.DebitDecimal.Cmp(fx.MustDecimal("0")))
}

func TestPostFreightToGLFailsClosedWithoutAccounting(t *testing.T) {
	repo := NewMockRepository()
	freightTestCharge(repo, 11, 7)
	service := NewGLPostingService(repo)

	postingID, err := service.PostFreightToGL(context.Background(), freightTestInput())

	require.ErrorIs(t, err, ErrAccountingNotConfigured)
	require.Zero(t, postingID)
	require.Nil(t, repo.Charges[11].GLPostingID)
	require.Empty(t, repo.AuditLogs[11])
}

func TestPostFreightToGLIsIdempotentForExistingSourceLink(t *testing.T) {
	repo := NewMockRepository()
	freightTestCharge(repo, 11, 7)
	poster := &journalPosterFake{entry: journals.JournalEntry{ID: 42}, duplicateOn: 2}
	service := NewGLPostingService(repo, poster)
	input := freightTestInput()

	firstID, err := service.PostFreightToGL(context.Background(), input)
	require.NoError(t, err)
	secondID, err := service.PostFreightToGL(context.Background(), input)

	require.NoError(t, err)
	require.Equal(t, firstID, secondID)
	require.Equal(t, int64(42), secondID)
	require.Len(t, poster.inputs, 1, "the service cache must not repost a successful freight charge")
	require.Len(t, repo.AuditLogs[11], 1)
}

func TestPostFreightToGLRejectsMismatchedExistingPostingIdentity(t *testing.T) {
	repo := NewMockRepository()
	freightTestCharge(repo, 11, 7)
	existingID := int64(41)
	repo.Charges[11].GLPostingID = &existingID
	poster := &journalPosterFake{entry: journals.JournalEntry{ID: 42}}
	service := NewGLPostingService(repo, poster)

	postingID, err := service.PostFreightToGL(context.Background(), freightTestInput())

	require.ErrorIs(t, err, ErrGLPostingIdentityMismatch)
	require.Zero(t, postingID)
	require.Equal(t, int64(41), *repo.Charges[11].GLPostingID)
	require.Empty(t, repo.AuditLogs[11])
}

func TestPostFreightToGLUpdatesChargeAndAuditsActor(t *testing.T) {
	repo := NewMockRepository()
	freightTestCharge(repo, 11, 7)
	poster := &journalPosterFake{entry: journals.JournalEntry{ID: 42}}
	service := NewGLPostingService(repo, poster)
	input := freightTestInput()

	_, err := service.PostFreightToGL(context.Background(), input)

	require.NoError(t, err)
	require.NotNil(t, repo.Charges[11].GLPostingID)
	require.Equal(t, int64(42), *repo.Charges[11].GLPostingID)
	require.Len(t, repo.AuditLogs[11], 1)
	audit := repo.AuditLogs[11][0]
	require.Equal(t, AuditTypePosted, audit.AuditType)
	require.Equal(t, int64(99), audit.UserID)
	require.Equal(t, int64(7), audit.CompanyID)
	require.Equal(t, int64(11), audit.FreightChargeID)
	require.Zero(t, audit.NewValue.Cmp(accountingmoney.Must("123.4500", 4)))
}

func TestPostFreightToGLPropagatesAccountingError(t *testing.T) {
	repo := NewMockRepository()
	freightTestCharge(repo, 11, 7)
	poster := &journalPosterFake{err: errors.New("accounting unavailable")}
	service := NewGLPostingService(repo, poster)

	_, err := service.PostFreightToGL(context.Background(), freightTestInput())

	require.ErrorContains(t, err, "failed to post freight journal")
	require.Nil(t, repo.Charges[11].GLPostingID)
}
