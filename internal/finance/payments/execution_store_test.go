package payments

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestPostgresStoreRoundTripsCompleteExecution(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	store := NewPostgresStore(db)

	want := paymentExecutionFixture()
	createdAt := want.CreatedAt
	updatedAt := want.UpdatedAt
	stored := want
	stored.Version = 1
	payload, err := json.Marshal(stored)
	require.NoError(t, err)

	db.ExpectQuery("WITH candidate").WithArgs(
		int64(7), int64(11), "bank-test", "payment_instruction", "instruction-1",
		"SETTLED", int64(0), pgxmock.AnyArg(), createdAt, updatedAt,
	).WillReturnRows(pgxmock.NewRows([]string{"version"}).AddRow(int64(1)))
	require.NoError(t, store.Save(context.Background(), want))

	db.ExpectQuery("SELECT payload").WithArgs(
		int64(7), int64(11), "bank-test", "payment_instruction", "instruction-1",
	).WillReturnRows(pgxmock.NewRows([]string{"payload", "state", "version", "created_at", "updated_at"}).
		AddRow(payload, "SETTLED", int64(1), createdAt, updatedAt))

	got, err := store.Get(context.Background(), want.Instruction.Reference)
	require.NoError(t, err)
	require.Equal(t, stored, got)
	require.Equal(t, "125.50", got.Instruction.Amount.Amount.String())
	require.Equal(t, "123.45", got.Settlement.SettledAmount.Amount.String())
	require.Equal(t, "sha256:file-1", got.ExportArtifact.Checksum)
	require.Len(t, got.Transitions, 3)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestPostgresStoreSaveRejectsStaleVersionWithoutOverwriting(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	store := NewPostgresStore(db)

	execution := paymentExecutionFixture()
	execution.Version = 4
	db.ExpectQuery("WITH candidate").WithArgs(
		int64(7), int64(11), "bank-test", "payment_instruction", "instruction-1",
		"SETTLED", int64(4), pgxmock.AnyArg(), execution.CreatedAt, execution.UpdatedAt,
	).WillReturnRows(pgxmock.NewRows([]string{"version"}).AddRow(int64(5)))
	require.NoError(t, store.Save(context.Background(), execution))

	db.ExpectQuery("WITH candidate").WithArgs(
		int64(7), int64(11), "bank-test", "payment_instruction", "instruction-1",
		"SETTLED", int64(4), pgxmock.AnyArg(), execution.CreatedAt, execution.UpdatedAt,
	).WillReturnError(pgx.ErrNoRows)
	require.ErrorIs(t, store.Save(context.Background(), execution), ErrExecutionConflict)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestPostgresStoreRejectsCrossCompanyProviderReferencesBeforeQuery(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	store := NewPostgresStore(db)

	execution := paymentExecutionFixture()
	execution.Submission.Reference.Connection.CompanyID = 8
	err = store.Save(context.Background(), execution)
	require.ErrorIs(t, err, ErrCrossCompanyReference)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestPostgresStoreMapsMissingExecution(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	store := NewPostgresStore(db)
	reference := paymentInstruction().Reference

	db.ExpectQuery("SELECT payload").WithArgs(
		int64(7), int64(11), "bank-test", "payment_instruction", "instruction-1",
	).WillReturnError(pgx.ErrNoRows)
	_, err = store.Get(context.Background(), reference)
	require.True(t, errors.Is(err, ErrExecutionNotFound))
	require.NoError(t, db.ExpectationsWereMet())
}

func paymentExecutionFixture() PaymentExecution {
	instruction := paymentInstruction()
	provider := providerReference()
	return PaymentExecution{
		Instruction: instruction,
		State:       StateSettled,
		Submission: &Submission{
			Reference:  provider,
			Status:     SubmissionStatusSubmitted,
			OccurredAt: time.Date(2026, time.August, 12, 9, 1, 0, 0, time.UTC),
		},
		Settlement: &Settlement{
			Reference:         provider,
			Instruction:       instruction.Reference,
			Status:            SettlementStatusSettled,
			SettledAmount:     automation.MustParseExact("123.45"),
			SettledAt:         time.Date(2026, time.August, 12, 9, 5, 0, 0, time.UTC),
			ProviderFee:       automation.MustParseExact("1.25"),
			EndToEndReference: instruction.EndToEndReference,
		},
		ExportArtifact: &ExportArtifact{
			Reference: automation.ExternalReference{
				Connection: instruction.Reference.Connection,
				ObjectType: "bank_file",
				ObjectID:   "file-1",
			},
			Checksum:  "sha256:file-1",
			CreatedAt: time.Date(2026, time.August, 12, 9, 2, 0, 0, time.UTC),
		},
		ProposedBy: 101,
		ApprovedBy: 202,
		ExecutorID: 303,
		Transitions: []StateTransition{
			{From: StateProposed, To: StateApproved, At: time.Date(2026, time.August, 12, 9, 0, 1, 0, time.UTC), Reason: "approved"},
			{From: StateApproved, To: StateSubmitted, At: time.Date(2026, time.August, 12, 9, 1, 0, 0, time.UTC), Reason: "submitted"},
			{From: StateSubmitted, To: StateSettled, At: time.Date(2026, time.August, 12, 9, 5, 0, 0, time.UTC), Reason: "settled"},
		},
		CreatedAt: time.Date(2026, time.August, 12, 9, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, time.August, 12, 9, 5, 0, 0, time.UTC),
	}
}
